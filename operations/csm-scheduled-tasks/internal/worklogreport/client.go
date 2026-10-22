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

package worklogreport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/wso2-open-operations/cs-tools/operations/csm-scheduled-tasks/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/operations/csm-scheduled-tasks/internal/httpsec"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// tokenFetchTimeout is the HTTP client timeout for token-endpoint requests.
var tokenFetchTimeout = 10 * time.Second

// maxResponseBytes caps the report body. A weekly report covering eleven
// teams and eighty people runs to a few hundred kilobytes; anything past
// eight megabytes is a bug at the other end, and reading it into memory
// unbounded would be this process's problem rather than theirs.
const maxResponseBytes = 8 << 20

// Config holds this client's configuration. Mirrors internal/entitycases.
type Config struct {
	BaseURL      string
	TokenURL     string
	ClientID     string
	ClientSecret string
	Scopes       []string
}

// Client is a narrow, read-only client for entity-service's
// GET /reports/worklogs.
type Client struct {
	http    *http.Client
	baseURL string
}

// NewClient constructs a Client authenticated via the OAuth2 client
// credentials grant.
func NewClient(cfg Config) (*Client, error) {
	if err := httpsec.RequireSecureURL(cfg.TokenURL); err != nil {
		return nil, fmt.Errorf("worklogreport: token URL: %w", err)
	}
	if err := httpsec.RequireSecureURL(cfg.BaseURL); err != nil {
		return nil, fmt.Errorf("worklogreport: base URL: %w", err)
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
	// A weekly report reads every time card in a seven-day window across
	// every delivery team, so it is allowed noticeably longer than the
	// other sub-crons' calls.
	httpClient.Timeout = 90 * time.Second
	httpsec.RejectInsecureRedirects(httpClient)

	return &Client{http: httpClient, baseURL: strings.TrimRight(cfg.BaseURL, "/")}, nil
}

// Report fetches one report. from and to are inclusive calendar dates in
// the report's own timezone; the caller decides which days those are,
// because entity-service holds work_date as a bare DATE and has no way to
// know which zone the caller means.
func (c *Client) Report(ctx context.Context, cadence Cadence, from, to time.Time) (Report, error) {
	q := url.Values{}
	q.Set("cadence", string(cadence))
	q.Set("from", from.Format("2006-01-02"))
	q.Set("to", to.Format("2006-01-02"))

	endpoint := c.baseURL + "/reports/worklogs?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Report{}, fmt.Errorf("worklogreport: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return Report{}, fmt.Errorf("worklogreport: %s report: %w", cadence, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return Report{}, fmt.Errorf("worklogreport: read %s report: %w", cadence, err)
	}
	if resp.StatusCode != http.StatusOK {
		return Report{}, &apierror.Error{StatusCode: resp.StatusCode, Body: string(body)}
	}

	var report Report
	if err := json.Unmarshal(body, &report); err != nil {
		return Report{}, fmt.Errorf("worklogreport: decode %s report: %w", cadence, err)
	}
	return report, nil
}
