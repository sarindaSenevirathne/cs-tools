// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

// Package gmail is a minimal Gmail REST client: list unprocessed messages,
// read their headers, and label them once handled. The label is the
// durable record of "already handled", so no other state store is needed for
// correctness.
package gmail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/jwt"

	"github.com/wso2-open-operations/cs-tools/integrations/csm-mail-paging-service/internal/rules"
)

const (
	defaultAPIBase = "https://gmail.googleapis.com/gmail/v1"
	scopeModify    = "https://www.googleapis.com/auth/gmail.modify"
	maxBodyBytes   = 4 << 20
	pageSize       = 50
)

// Config configures a Client.
type Config struct {
	// CredentialsJSON is a Google service-account key (JSON) with
	// domain-wide delegation granted for gmail.modify.
	CredentialsJSON string
	// DelegatedUser is the mailbox the service account acts as; it must
	// receive the group's mail.
	DelegatedUser string
	// ProcessedLabel is applied to every handled message.
	ProcessedLabel string
	// APIBaseURL overrides the Gmail API base (tests).
	APIBaseURL string
	// HTTPClient overrides the authenticated client (tests).
	HTTPClient *http.Client
}

// Client talks to the Gmail API.
type Client struct {
	http    *http.Client
	base    string
	label   string
	labelID string
}

// New builds a Client. It makes no network call.
func New(ctx context.Context, cfg Config) (*Client, error) {
	c := &Client{base: cfg.APIBaseURL, label: cfg.ProcessedLabel, http: cfg.HTTPClient}
	if c.base == "" {
		c.base = defaultAPIBase
	}
	if c.label == "" {
		return nil, errors.New("gmail: processed label is required")
	}
	if c.http == nil {
		var key struct {
			ClientEmail string `json:"client_email"`
			PrivateKey  string `json:"private_key"`
			TokenURI    string `json:"token_uri"`
		}
		if err := json.Unmarshal([]byte(cfg.CredentialsJSON), &key); err != nil {
			return nil, fmt.Errorf("gmail: parse credentials: %w", err)
		}
		if key.ClientEmail == "" || key.PrivateKey == "" || cfg.DelegatedUser == "" {
			return nil, errors.New("gmail: credentials need client_email, private_key and a delegated user")
		}
		jc := &jwt.Config{
			Email:      key.ClientEmail,
			PrivateKey: []byte(key.PrivateKey),
			Scopes:     []string{scopeModify},
			TokenURL:   key.TokenURI,
			Subject:    cfg.DelegatedUser,
		}
		// The same timeout-bound context serves the token exchange and the
		// API calls, so a stalled token endpoint cannot hang the poll loop.
		tokenCtx := context.WithValue(ctx, oauth2.HTTPClient, &http.Client{Timeout: 30 * time.Second})
		c.http = oauth2.NewClient(tokenCtx, jc.TokenSource(tokenCtx))
		c.http.Timeout = 30 * time.Second
	}
	return c, nil
}

func (c *Client) do(ctx context.Context, method, path string, q url.Values, body, out any) error {
	u := c.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("gmail: %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return fmt.Errorf("gmail: read %s: %w", path, err)
	}
	if resp.StatusCode/100 != 2 {
		// The body is not logged: it can echo message content.
		return fmt.Errorf("gmail: %s %s: status %d", method, path, resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

// EnsureLabel finds or creates the processed label. Call once at startup.
func (c *Client) EnsureLabel(ctx context.Context) error {
	var list struct {
		Labels []struct{ ID, Name string } `json:"labels"`
	}
	if err := c.do(ctx, http.MethodGet, "/users/me/labels", nil, nil, &list); err != nil {
		return err
	}
	for _, l := range list.Labels {
		if l.Name == c.label {
			c.labelID = l.ID
			return nil
		}
	}
	var created struct {
		ID string `json:"id"`
	}
	err := c.do(ctx, http.MethodPost, "/users/me/labels", nil,
		map[string]string{"name": c.label, "labelListVisibility": "labelHide", "messageListVisibility": "hide"}, &created)
	if err != nil {
		return err
	}
	c.labelID = created.ID
	return nil
}

// ListUnprocessed returns ids of messages matching searchQuery that do not
// carry the processed label, oldest first. Gmail lists newest first, so every
// page is collected (ids only, bounded by MAIL_LOOKBACK) and the result
// reversed: a backlog drains in arrival order and a run of failing recent
// messages cannot starve older ones.
func (c *Client) ListUnprocessed(ctx context.Context, searchQuery string) ([]string, error) {
	var ids []string
	seen := map[string]bool{}
	pageToken := ""
	for {
		q := url.Values{"q": {fmt.Sprintf("%s -label:%q", searchQuery, c.label)}, "maxResults": {fmt.Sprint(pageSize)}}
		if pageToken != "" {
			q.Set("pageToken", pageToken)
		}
		var out struct {
			Messages      []struct{ ID string } `json:"messages"`
			NextPageToken string                `json:"nextPageToken"`
		}
		if err := c.do(ctx, http.MethodGet, "/users/me/messages", q, nil, &out); err != nil {
			return nil, err
		}
		for _, m := range out.Messages {
			ids = append(ids, m.ID)
		}
		// An empty or repeated token ends the scan, so a misbehaving API
		// cannot loop it forever.
		if out.NextPageToken == "" || seen[out.NextPageToken] {
			break
		}
		seen[out.NextPageToken] = true
		pageToken = out.NextPageToken
	}
	for i, j := 0, len(ids)-1; i < j; i, j = i+1, j-1 {
		ids[i], ids[j] = ids[j], ids[i]
	}
	return ids, nil
}

// Get reads a message's headers (never its body).
func (c *Client) Get(ctx context.Context, id string) (rules.Message, error) {
	q := url.Values{"format": {"metadata"}}
	for _, h := range []string{"From", "To", "Cc", "Delivered-To", "Subject", "Message-ID"} {
		q.Add("metadataHeaders", h)
	}
	var out struct {
		InternalDate string `json:"internalDate"`
		Payload      struct {
			Headers []struct{ Name, Value string } `json:"headers"`
		} `json:"payload"`
	}
	if err := c.do(ctx, http.MethodGet, "/users/me/messages/"+url.PathEscape(id), q, nil, &out); err != nil {
		return rules.Message{}, err
	}
	m := rules.Message{ID: id}
	for _, h := range out.Payload.Headers {
		switch http.CanonicalHeaderKey(h.Name) {
		case "From":
			if a := rules.Addresses(h.Value); len(a) > 0 {
				m.From = a[0]
			}
		case "To", "Cc", "Delivered-To":
			m.Recipients = append(m.Recipients, rules.Addresses(h.Value)...)
		case "Subject":
			m.Subject = h.Value
		case "Message-Id":
			m.ID = h.Value
		}
	}
	var ms int64
	if _, err := fmt.Sscan(out.InternalDate, &ms); err == nil {
		m.ReceivedAt = time.UnixMilli(ms).UTC().Format(time.RFC3339)
	}
	return m, nil
}

// MarkProcessed labels a message so it is never listed again.
func (c *Client) MarkProcessed(ctx context.Context, id string) error {
	if c.labelID == "" {
		return errors.New("gmail: EnsureLabel was not called")
	}
	return c.do(ctx, http.MethodPost, "/users/me/messages/"+url.PathEscape(id)+"/modify", nil,
		map[string][]string{"addLabelIds": {c.labelID}}, nil)
}
