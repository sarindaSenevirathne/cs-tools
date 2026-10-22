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

package notify_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/wso2-open-operations/cs-tools/operations/csm-scheduled-tasks/internal/notify"
	"github.com/wso2-open-operations/cs-tools/operations/csm-scheduled-tasks/internal/worklogreport"
)

// End-to-end send harness for the CS Worklogs report mail.
//
// It renders a report that entity-service actually produced (see
// WORKLOG_REPORT_JSON_OUT in its worklog_report_integration_test.go) and
// mails it through the real notification service, so the whole chain --
// Postgres, aggregation, template, delivery -- is exercised once rather
// than each half being trusted separately.
//
// Skipped unless WORKLOG_E2E_TO is set, and it must be set to a real
// address the runner owns. This never derives recipients from the report.
//
//	WORKLOG_E2E_TO=you@example.com \
//	WORKLOG_REPORT_JSON=/path/to/report.json \
//	OAUTH2_CLIENT_ID=... OAUTH2_CLIENT_SECRET=... OAUTH2_TOKEN_URL=... \
//	EMAIL_BASE_URL=... EMAIL_FROM_ADDRESS=... EMAIL_SCOPES=... \
//	  go test -run TestWorklogsReportE2ESend ./internal/notify/
func TestWorklogsReportE2ESend(t *testing.T) {
	to := os.Getenv("WORKLOG_E2E_TO")
	if to == "" {
		t.Skip("WORKLOG_E2E_TO is not set; skipping the live send")
	}
	path := os.Getenv("WORKLOG_REPORT_JSON")
	if path == "" {
		t.Fatal("WORKLOG_REPORT_JSON must point at a report built by entity-service")
	}

	blob, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var report worklogreport.Report
	if err := json.Unmarshal(blob, &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}

	// Issue keys link into the CSM portal, as they linked into the agent
	// workspace in ServiceNow. Empty renders them as plain text.
	caseLinkBase := os.Getenv("CSM_PORTAL_WEB_BASE_URL")

	body := notify.RenderWorklogsReport(report, caseLinkBase)
	subject := notify.WorklogsReportSubject(report)

	// A template placeholder that survived rendering would reach the
	// recipient as a literal HTML comment, so fail before sending.
	if strings.Contains(body, "<!-- [") {
		t.Fatalf("unsubstituted placeholder in rendered body: %s", body[strings.Index(body, "<!-- ["):strings.Index(body, "<!-- [")+40])
	}

	// WORKLOG_RENDER_OUT writes the rendered mail to disk and sends
	// nothing, for eyeballing the two cadences side by side.
	if out := os.Getenv("WORKLOG_RENDER_OUT"); out != "" {
		if err := os.WriteFile(out, []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", out, err)
		}
		t.Logf("rendered %q to %s (%d bytes); not sent", subject, out, len(body))
		return
	}

	client, err := notify.NewClient(notify.Config{
		BaseURL:      os.Getenv("EMAIL_BASE_URL"),
		TokenURL:     os.Getenv("OAUTH2_TOKEN_URL"),
		ClientID:     os.Getenv("OAUTH2_CLIENT_ID"),
		ClientSecret: os.Getenv("OAUTH2_CLIENT_SECRET"),
		Scopes:       strings.Fields(os.Getenv("EMAIL_SCOPES")),
		FromAddress:  os.Getenv("EMAIL_FROM_ADDRESS"),
	})
	if err != nil {
		t.Fatalf("notify client: %v", err)
	}

	// The real audience is derived from the team registry; WORKLOG_E2E_TO
	// overrides it outright so a test run cannot reach it.
	audience := worklogreport.Resolve(report.Recipients, worklogreport.Audience{},
		worklogreport.SourceConfigured, []string{to})
	t.Logf("derived audience (NOT used): to=%v cc=%v", report.Recipients.To, report.Recipients.Cc)
	if len(audience.Cc) != 0 || len(audience.To) != 1 || audience.To[0] != to {
		t.Fatalf("override did not take: %+v", audience)
	}

	if err := client.SendEmail(context.Background(), audience.To, audience.Cc, subject, body); err != nil {
		t.Fatalf("send: %v", err)
	}
	t.Logf("sent %q to %s (%d bytes)", subject, to, len(body))
}
