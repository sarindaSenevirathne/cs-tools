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

// Package migrationreport is the read-only client for entity-service's
// weekly migration case scan (GET /reports/migration-cases) and the types
// it returns. internal/migrationscan turns the result into the mail;
// internal/notify renders it. The types live here, apart from the task, so
// notify can depend on them without the task depending on notify in a
// cycle.
package migrationreport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/wso2-open-operations/cs-tools/operations/csm-scheduled-tasks/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/operations/csm-scheduled-tasks/internal/httpsec"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

var tokenFetchTimeout = 10 * time.Second

// maxResponseBytes bounds the scan's body. A few hundred rows of a dozen
// short fields is well under a megabyte; eight is a generous ceiling.
const maxResponseBytes = 8 << 20

// Report is the scan as entity-service returns it.
type Report struct {
	GeneratedAt   time.Time `json:"generatedAt"`
	OlderThanDays int       `json:"olderThanDays"`
	CreatedBefore time.Time `json:"createdBefore"`
	// OnHold are the migrations the customer asked to pause.
	OnHold []Row `json:"onHold"`
	// WaitingOnAccounts are the migrations blocked on the accounts team.
	WaitingOnAccounts []Row `json:"waitingOnAccounts"`
	// InProgress are the remaining long-running migrations.
	InProgress []Row `json:"inProgress"`
	// Recipients are the mail groups of the teams entity-service is
	// configured to send the scan to.
	Recipients []string `json:"recipients"`
}

// Row is one migration engagement in the scan.
type Row struct {
	ID                    string     `json:"id"`
	Number                string     `json:"number"`
	IssueKey              string     `json:"issueKey"`
	CreatedOn             time.Time  `json:"createdOn"`
	LastCustomerCommentOn *time.Time `json:"lastCustomerCommentOn"`
	AssignedTo            string     `json:"assignedTo"`
	Product               string     `json:"product"`
	Account               string     `json:"account"`
	CsTeam                string     `json:"csTeam"`
	SalesRegion           string     `json:"salesRegion"`
	AccountManager        string     `json:"accountManager"`
	TechnicalOwner        string     `json:"technicalOwner"`
}

// Config is the entity-service connection: the same OAuth client
// credentials every other entity-service client in this component uses.
type Config struct {
	BaseURL      string
	TokenURL     string
	ClientID     string
	ClientSecret string
	Scopes       []string
}

// Client fetches the scan.
type Client struct {
	http    *http.Client
	baseURL string
}

// NewClient constructs a Client. Both URLs must be https, as everywhere
// else in this component.
func NewClient(cfg Config) (*Client, error) {
	if err := httpsec.RequireSecureURL(cfg.TokenURL); err != nil {
		return nil, fmt.Errorf("migrationreport: token URL: %w", err)
	}
	if err := httpsec.RequireSecureURL(cfg.BaseURL); err != nil {
		return nil, fmt.Errorf("migrationreport: base URL: %w", err)
	}

	cc := clientcredentials.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		TokenURL:     cfg.TokenURL,
		Scopes:       cfg.Scopes,
	}

	tokenHTTPClient := &http.Client{Timeout: tokenFetchTimeout}
	httpsec.RejectInsecureRedirects(tokenHTTPClient)
	tokenCtx := context.WithValue(context.Background(), oauth2.HTTPClient, tokenHTTPClient)
	httpClient := cc.Client(tokenCtx)
	httpClient.Timeout = 60 * time.Second
	httpsec.RejectInsecureRedirects(httpClient)

	return &Client{http: httpClient, baseURL: strings.TrimRight(cfg.BaseURL, "/")}, nil
}

// Report fetches the scan for migrations older than olderThanDays.
func (c *Client) Report(ctx context.Context, olderThanDays int) (Report, error) {
	q := url.Values{}
	q.Set("olderThanDays", strconv.Itoa(olderThanDays))

	endpoint := c.baseURL + "/reports/migration-cases?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Report{}, fmt.Errorf("migrationreport: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return Report{}, fmt.Errorf("migrationreport: fetch scan: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return Report{}, fmt.Errorf("migrationreport: read scan: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return Report{}, &apierror.Error{StatusCode: resp.StatusCode, Body: string(body)}
	}

	var report Report
	if err := json.Unmarshal(body, &report); err != nil {
		return Report{}, fmt.Errorf("migrationreport: decode scan: %w", err)
	}
	return report, nil
}
