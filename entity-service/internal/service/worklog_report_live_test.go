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

package service_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/service"
)

// Builds both reports against a real deployment, seeding nothing and
// writing nothing. Every statement it causes is a SELECT.
//
// This is the harness for answering "would this work against the actual
// database", which no amount of fixture testing settles: fixtures prove
// the arithmetic, this proves the queries run, the roles resolve and the
// roster is populated.
//
//	WORKLOG_LIVE_DB_URL="postgres://..." \
//	WORKLOG_LIVE_FROM=2026-10-12 WORKLOG_LIVE_TO=2026-10-18 \
//	  go test -v -run TestWorklogReportAgainstLiveDatabase ./internal/service/
//
// WORKLOG_LIVE_JSON_OUT writes the weekly report, WORKLOG_LIVE_DAILY_OUT
// the daily one, so csm-scheduled-tasks can render what came back.
func TestWorklogReportAgainstLiveDatabase(t *testing.T) {
	dsn := os.Getenv("WORKLOG_LIVE_DB_URL")
	if dsn == "" {
		t.Skip("WORKLOG_LIVE_DB_URL is not set; skipping the live-deployment read")
	}
	ctx := repository.WithSystemIdentity(context.Background())
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	from := mustLiveDate(t, "WORKLOG_LIVE_FROM")
	to := mustLiveDate(t, "WORKLOG_LIVE_TO")

	svc := service.NewWorklogReportService(
		repository.NewWorklogReportRepository(repository.NewScoped(pool)),
		[]string{"cre-abt", "cre"}, nil)

	for _, tc := range []struct {
		cadence domain.WorklogReportCadence
		from    time.Time
		out     string
	}{
		{domain.WorklogReportCadenceWeekly, from, "WORKLOG_LIVE_JSON_OUT"},
		{domain.WorklogReportCadenceDaily, to, "WORKLOG_LIVE_DAILY_OUT"},
	} {
		report, err := svc.BuildReport(ctx, domain.WorklogReportRequest{
			Cadence: tc.cadence, From: tc.from, To: to})
		if err != nil {
			t.Fatalf("%s BuildReport: %v", tc.cadence, err)
		}

		t.Logf("%s %s..%s: %d members, %d issues, %.2f hours across %d teams",
			tc.cadence, report.From, report.To, report.Totals.Members,
			report.Totals.Issues, report.Totals.TotalHours, len(report.Teams))
		for _, tr := range report.Teams {
			t.Logf("    %-28s members=%-3d forAvg=%-4v issues=%-4d total=%.2f",
				tr.TeamName, tr.Members, tr.MembersForAverage, tr.Issues, tr.TotalHours)
		}
		t.Logf("    derived To: %v", report.Recipients.To)
		t.Logf("    derived Cc: %v", report.Recipients.Cc)

		// A report with no teams means the roster read came back empty,
		// which is a configuration problem rather than a quiet zero --
		// every deployment has teams even when nobody logged time.
		if len(report.Teams) == 0 {
			t.Errorf("%s: no teams at all; the roster or the team-type scope is wrong", tc.cadence)
		}

		if out := os.Getenv(tc.out); out != "" {
			blob, err := json.MarshalIndent(report, "", "  ")
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if err := os.WriteFile(out, blob, 0o600); err != nil {
				t.Fatalf("write %s: %v", out, err)
			}
		}
	}
}

func mustLiveDate(t *testing.T, env string) time.Time {
	t.Helper()
	v := os.Getenv(env)
	if v == "" {
		t.Fatalf("%s must be set, as YYYY-MM-DD", env)
	}
	d, err := time.Parse("2006-01-02", v)
	if err != nil {
		t.Fatalf("%s: %v", env, err)
	}
	return d
}
