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

	"github.com/wso2-open-operations/cs-tools/operations/csm-scheduled-tasks/internal/migrationreport"
	"github.com/wso2-open-operations/cs-tools/operations/csm-scheduled-tasks/internal/notify"
)

// End-to-end send harness for the migration case scan mail: renders a scan
// entity-service actually produced (MIGRATION_LIVE_JSON_OUT in its
// migration_case_report_live_test.go) and mails it through the real
// notification service, so the whole chain is exercised once.
//
// Skipped unless MIGRATION_E2E_TO is set, and it must be a real address
// the runner owns. The scan's own derived recipients are never used.
//
//	MIGRATION_E2E_TO=you@example.com \
//	MIGRATION_SCAN_JSON=/path/to/scan.json \
//	MIGRATION_CASE_REPORT_LINK_BASE_URL=https://.../cases \
//	OAUTH2_CLIENT_ID=... OAUTH2_CLIENT_SECRET=... OAUTH2_TOKEN_URL=... \
//	EMAIL_BASE_URL=... EMAIL_FROM_ADDRESS=... EMAIL_SCOPES=... \
//	  go test -run TestMigrationCaseReportE2ESend ./internal/notify/
func TestMigrationCaseReportE2ESend(t *testing.T) {
	to := os.Getenv("MIGRATION_E2E_TO")
	if to == "" {
		t.Skip("MIGRATION_E2E_TO is not set; skipping the live send")
	}
	path := os.Getenv("MIGRATION_SCAN_JSON")
	if path == "" {
		t.Fatal("MIGRATION_SCAN_JSON must point at a scan built by entity-service")
	}
	blob, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var report migrationreport.Report
	if err := json.Unmarshal(blob, &report); err != nil {
		t.Fatalf("decode scan: %v", err)
	}

	body := notify.RenderMigrationCaseReport(report, os.Getenv("MIGRATION_CASE_REPORT_LINK_BASE_URL"))
	if out := os.Getenv("MIGRATION_E2E_HTML_OUT"); out != "" {
		if err := os.WriteFile(out, []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", out, err)
		}
	}

	client, err := notify.NewClient(notify.Config{
		BaseURL:      os.Getenv("EMAIL_BASE_URL"),
		TokenURL:     os.Getenv("OAUTH2_TOKEN_URL"),
		ClientID:     os.Getenv("OAUTH2_CLIENT_ID"),
		ClientSecret: os.Getenv("OAUTH2_CLIENT_SECRET"),
		Scopes:       splitScopes(os.Getenv("EMAIL_SCOPES")),
		FromAddress:  os.Getenv("EMAIL_FROM_ADDRESS"),
	})
	if err != nil {
		t.Fatalf("notify client: %v", err)
	}
	subject := "[Test copy] [Report] Weekly Migration Case Scan"
	if err := client.SendEmail(context.Background(), []string{to}, nil, subject, body); err != nil {
		t.Fatalf("send: %v", err)
	}
	t.Logf("sent %q to %s: on hold %d, waiting on accounts %d, in progress %d",
		subject, to, len(report.OnHold), len(report.WaitingOnAccounts), len(report.InProgress))
}

func splitScopes(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
