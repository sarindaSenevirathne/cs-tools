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
	"errors"
	"testing"
	"time"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/service"
)

type fakeMigrationCaseReportRepo struct {
	rows          []domain.MigrationCaseReportRow
	emails        []string
	gotCutoff     time.Time
	gotHold       string
	gotAccounts   string
	gotTeamKeys   []string
	listErr       error
	recipientsErr error
}

func (f *fakeMigrationCaseReportRepo) ListLongRunningMigrations(_ context.Context, createdBefore time.Time, holdTag, accountsTag string) ([]domain.MigrationCaseReportRow, error) {
	f.gotCutoff, f.gotHold, f.gotAccounts = createdBefore, holdTag, accountsTag
	return f.rows, f.listErr
}

func (f *fakeMigrationCaseReportRepo) TeamGroupEmails(_ context.Context, teamKeys []string) ([]string, error) {
	f.gotTeamKeys = teamKeys
	return f.emails, f.recipientsErr
}

func migrationRow(number string, onHold, waiting bool) domain.MigrationCaseReportRow {
	return domain.MigrationCaseReportRow{ID: "id-" + number, Number: number, OnHold: onHold, WaitingOnAccounts: waiting}
}

func numbers(rows []domain.MigrationCaseReportRow) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Number)
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestBucketMigrationCases(t *testing.T) {
	rows := []domain.MigrationCaseReportRow{
		migrationRow("ENG0001", true, false),
		migrationRow("ENG0002", false, true),
		migrationRow("ENG0003", true, true),
		migrationRow("ENG0004", false, false),
		migrationRow("ENG0005", false, false),
	}
	got := service.BucketMigrationCases(rows)

	if want := []string{"ENG0001", "ENG0003"}; !equalStrings(numbers(got.OnHold), want) {
		t.Errorf("OnHold = %v, want %v", numbers(got.OnHold), want)
	}
	if want := []string{"ENG0002", "ENG0003"}; !equalStrings(numbers(got.WaitingOnAccounts), want) {
		t.Errorf("WaitingOnAccounts = %v, want %v", numbers(got.WaitingOnAccounts), want)
	}
	if want := []string{"ENG0004", "ENG0005"}; !equalStrings(numbers(got.InProgress), want) {
		t.Errorf("InProgress = %v, want %v: a row with either tag must not reach the third table", numbers(got.InProgress), want)
	}
}

func TestBucketMigrationCasesEmptyTablesAreLists(t *testing.T) {
	got := service.BucketMigrationCases(nil)
	if got.OnHold == nil || got.WaitingOnAccounts == nil || got.InProgress == nil || got.Recipients == nil {
		t.Fatalf("empty tables must be non-nil so they serialise as [], got %+v", got)
	}
}

func TestMigrationCaseCutoff(t *testing.T) {
	loc, err := time.LoadLocation(service.MigrationCaseReportTimezone)
	if err != nil {
		t.Fatalf("load %s: %v", service.MigrationCaseReportTimezone, err)
	}
	// Sunday 2026-10-18 22:30 UTC is already Monday 2026-10-19 04:00 in
	// Colombo. 180 days before Monday 2026-10-19 is 2026-04-22; the cut-off
	// is the start of that day in Colombo.
	run := time.Date(2026, 10, 18, 22, 30, 0, 0, time.UTC)
	got := service.MigrationCaseCutoff(run, 180, loc)
	want := time.Date(2026, 4, 22, 0, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Errorf("cutoff = %s, want %s", got, want)
	}
	if got.Location().String() != service.MigrationCaseReportTimezone {
		t.Errorf("cutoff zone = %s, want %s", got.Location(), service.MigrationCaseReportTimezone)
	}
}

func TestMigrationCaseReportBuildReport(t *testing.T) {
	repo := &fakeMigrationCaseReportRepo{
		rows:   []domain.MigrationCaseReportRow{migrationRow("ENG0001", true, false), migrationRow("ENG0002", false, false)},
		emails: []string{"team-a@example.com", "team-b@example.com"},
	}
	svc := service.NewMigrationCaseReportService(repo, service.MigrationCaseReportConfig{
		HoldTag:           "hold-tag",
		AccountsTag:       "accounts-tag",
		RecipientTeamKeys: []string{"team-a", "team-b"},
	})
	now := time.Date(2026, 10, 18, 22, 30, 0, 0, time.UTC)

	got, err := svc.BuildReport(context.Background(), domain.MigrationCaseReportRequest{Now: now})
	if err != nil {
		t.Fatalf("BuildReport: %v", err)
	}
	if got.OlderThanDays != service.MigrationCaseReportDefaultOlderThanDays {
		t.Errorf("OlderThanDays = %d, want the default %d", got.OlderThanDays, service.MigrationCaseReportDefaultOlderThanDays)
	}
	if !got.CreatedBefore.Equal(repo.gotCutoff) {
		t.Errorf("CreatedBefore %s differs from the cut-off the repository was asked for, %s", got.CreatedBefore, repo.gotCutoff)
	}
	if repo.gotHold != "hold-tag" || repo.gotAccounts != "accounts-tag" {
		t.Errorf("tags passed = %q/%q, want the configured ones", repo.gotHold, repo.gotAccounts)
	}
	if !equalStrings(repo.gotTeamKeys, []string{"team-a", "team-b"}) {
		t.Errorf("team keys passed = %v", repo.gotTeamKeys)
	}
	if !equalStrings(got.Recipients, repo.emails) {
		t.Errorf("Recipients = %v, want %v", got.Recipients, repo.emails)
	}
	if !got.GeneratedAt.Equal(now) {
		t.Errorf("GeneratedAt = %s, want the requested instant %s", got.GeneratedAt, now)
	}
	if len(got.OnHold) != 1 || len(got.InProgress) != 1 || len(got.WaitingOnAccounts) != 0 {
		t.Errorf("tables = %d/%d/%d, want 1/0/1", len(got.OnHold), len(got.WaitingOnAccounts), len(got.InProgress))
	}
}

func TestMigrationCaseReportRejectsBadAgeAndMissingTags(t *testing.T) {
	svc := service.NewMigrationCaseReportService(&fakeMigrationCaseReportRepo{}, service.MigrationCaseReportConfig{
		HoldTag: "hold-tag", AccountsTag: "accounts-tag"})
	for _, days := range []int{-1, service.MigrationCaseReportMaxOlderThanDays + 1} {
		_, err := svc.BuildReport(context.Background(), domain.MigrationCaseReportRequest{OlderThanDays: days})
		var ve *apierror.ValidationError
		if !errors.As(err, &ve) {
			t.Errorf("olderThanDays=%d: err = %v, want a validation error", days, err)
		}
	}

	unconfigured := service.NewMigrationCaseReportService(&fakeMigrationCaseReportRepo{}, service.MigrationCaseReportConfig{})
	_, err := unconfigured.BuildReport(context.Background(), domain.MigrationCaseReportRequest{})
	var sue *apierror.ServiceUnavailableError
	if !errors.As(err, &sue) {
		t.Errorf("no tags configured: err = %v, want service unavailable", err)
	}
}

func TestMigrationCaseReportPropagatesRepositoryErrors(t *testing.T) {
	boom := errors.New("boom")
	svc := service.NewMigrationCaseReportService(&fakeMigrationCaseReportRepo{listErr: boom}, service.MigrationCaseReportConfig{
		HoldTag: "hold-tag", AccountsTag: "accounts-tag"})
	if _, err := svc.BuildReport(context.Background(), domain.MigrationCaseReportRequest{}); !errors.Is(err, boom) {
		t.Errorf("err = %v, want %v", err, boom)
	}
}
