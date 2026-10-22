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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/service"
)

// Builds the migration case scan against a real deployment, seeding
// nothing and writing nothing: every statement it causes is a SELECT.
// Fixtures prove the bucketing; this proves the tags exist under the
// configured names, the accounts carry a CS team and owners, and the
// customer comments resolve.
//
//	MIGRATION_LIVE_DB_URL="postgres://..." \
//	MIGRATION_LIVE_TEAM_KEYS=team-a,team-b MIGRATION_LIVE_OLDER_THAN_DAYS=180 \
//	MIGRATION_LIVE_JSON_OUT=/tmp/scan.json \
//	  go test -v -run TestMigrationCaseReportAgainstLiveDatabase ./internal/service/
//
// MIGRATION_LIVE_HOLD_TAG / MIGRATION_LIVE_ACCOUNTS_TAG override the
// default tag names. MIGRATION_LIVE_JSON_OUT writes the scan so
// csm-scheduled-tasks can render and send what came back.
func TestMigrationCaseReportAgainstLiveDatabase(t *testing.T) {
	dsn := os.Getenv("MIGRATION_LIVE_DB_URL")
	if dsn == "" {
		t.Skip("MIGRATION_LIVE_DB_URL is not set; skipping the live-deployment read")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	days := service.MigrationCaseReportDefaultOlderThanDays
	if raw := os.Getenv("MIGRATION_LIVE_OLDER_THAN_DAYS"); raw != "" {
		if days, err = strconv.Atoi(raw); err != nil {
			t.Fatalf("MIGRATION_LIVE_OLDER_THAN_DAYS: %v", err)
		}
	}
	cfg := service.MigrationCaseReportConfig{HoldTag: "s_mig_hold", AccountsTag: "s_acc"}
	if v := os.Getenv("MIGRATION_LIVE_HOLD_TAG"); v != "" {
		cfg.HoldTag = v
	}
	if v := os.Getenv("MIGRATION_LIVE_ACCOUNTS_TAG"); v != "" {
		cfg.AccountsTag = v
	}
	for _, k := range strings.Split(os.Getenv("MIGRATION_LIVE_TEAM_KEYS"), ",") {
		if k = strings.TrimSpace(k); k != "" {
			cfg.RecipientTeamKeys = append(cfg.RecipientTeamKeys, k)
		}
	}

	svc := service.NewMigrationCaseReportService(repository.NewMigrationCaseReportRepository(repository.NewScoped(pool)), cfg)
	report, err := svc.BuildReport(ctx, domain.MigrationCaseReportRequest{OlderThanDays: days, Now: time.Now()})
	if err != nil {
		t.Fatalf("BuildReport: %v", err)
	}

	t.Logf("older than %d days (created before %s): on hold %d, waiting on accounts %d, in progress %d; derived recipients %d",
		report.OlderThanDays, report.CreatedBefore.Format("2006-01-02"),
		len(report.OnHold), len(report.WaitingOnAccounts), len(report.InProgress), len(report.Recipients))
	// Whether the tags exist under the configured names is the first
	// thing a deployment gets wrong, and it is invisible in the counts
	// alone: a misspelt tag simply moves every row into the third table.
	var holdTags, accountsTags int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FILTER (WHERE LOWER(name) = LOWER($1)), COUNT(*) FILTER (WHERE LOWER(name) = LOWER($2)) FROM tag`,
		cfg.HoldTag, cfg.AccountsTag).Scan(&holdTags, &accountsTags); err != nil {
		t.Fatalf("count tags: %v", err)
	}
	t.Logf("tag rows named %q: %d, named %q: %d", cfg.HoldTag, holdTags, cfg.AccountsTag, accountsTags)
	var blankTeam, blankRegion, blankOwners, withComment int
	for _, rows := range [][]domain.MigrationCaseReportRow{report.OnHold, report.WaitingOnAccounts, report.InProgress} {
		for _, r := range rows {
			if r.CsTeam == "" {
				blankTeam++
			}
			if r.SalesRegion == "" {
				blankRegion++
			}
			if r.AccountManager == "" && r.TechnicalOwner == "" {
				blankOwners++
			}
			if r.LastCustomerCommentOn != nil {
				withComment++
			}
		}
	}
	t.Logf("rows without a CS team %d, without a sales region %d, without any owner %d, with a customer comment %d",
		blankTeam, blankRegion, blankOwners, withComment)

	if out := os.Getenv("MIGRATION_LIVE_JSON_OUT"); out != "" {
		blob, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		if err := os.WriteFile(out, blob, 0o600); err != nil {
			t.Fatalf("write %s: %v", out, err)
		}
		t.Logf("scan written to %s", out)
	}
}
