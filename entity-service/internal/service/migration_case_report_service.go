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

package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
)

// MigrationCaseReportService builds the weekly migration case scan.
type MigrationCaseReportService interface {
	BuildReport(ctx context.Context, req domain.MigrationCaseReportRequest) (domain.MigrationCaseReport, error)
}

// MigrationCaseReportTimezone is the zone the age cut-off is taken in. The
// scan runs on a Sunday evening in UTC, which is already Monday morning in
// Colombo, where the team reading it works; "created more than N days
// ago" is counted in their calendar.
const MigrationCaseReportTimezone = "Asia/Colombo"

// MigrationCaseReportDefaultOlderThanDays is the age a migration must reach
// before the scan lists it when the caller does not say: six months, the
// window the CS team asked for in place of the original three.
const MigrationCaseReportDefaultOlderThanDays = 180

// MigrationCaseReportMaxOlderThanDays bounds the age a caller may ask for.
const MigrationCaseReportMaxOlderThanDays = 3650

// MigrationCaseReportConfig names the tags and teams the scan is built
// from. Tags are compared ignoring case.
type MigrationCaseReportConfig struct {
	// HoldTag marks a migration the customer asked to pause.
	HoldTag string
	// AccountsTag marks a migration waiting on the accounts team.
	AccountsTag string
	// RecipientTeamKeys are the teams whose mail groups receive the scan.
	RecipientTeamKeys []string
}

type migrationCaseReportService struct {
	repo repository.MigrationCaseReportRepository
	cfg  MigrationCaseReportConfig
	loc  *time.Location
}

// NewMigrationCaseReportService constructs a MigrationCaseReportService.
func NewMigrationCaseReportService(repo repository.MigrationCaseReportRepository, cfg MigrationCaseReportConfig) MigrationCaseReportService {
	loc, err := time.LoadLocation(MigrationCaseReportTimezone)
	if err != nil {
		// The zone is a constant; a missing tz database is the only way
		// this fails, and UTC is the honest fallback.
		loc = time.UTC
	}
	return &migrationCaseReportService{repo: repo, cfg: cfg, loc: loc}
}

func (s *migrationCaseReportService) BuildReport(ctx context.Context, req domain.MigrationCaseReportRequest) (domain.MigrationCaseReport, error) {
	days := req.OlderThanDays
	if days == 0 {
		days = MigrationCaseReportDefaultOlderThanDays
	}
	if days < 1 || days > MigrationCaseReportMaxOlderThanDays {
		return domain.MigrationCaseReport{}, &apierror.ValidationError{
			Msg: fmt.Sprintf("olderThanDays must be between 1 and %d", MigrationCaseReportMaxOlderThanDays)}
	}
	if strings.TrimSpace(s.cfg.HoldTag) == "" || strings.TrimSpace(s.cfg.AccountsTag) == "" {
		return domain.MigrationCaseReport{}, &apierror.ServiceUnavailableError{
			Msg: "the migration case scan's tags are not configured"}
	}
	now := req.Now
	if now.IsZero() {
		now = time.Now()
	}
	createdBefore := MigrationCaseCutoff(now, days, s.loc)

	// The route is internal-only; the report reads under the system
	// identity so row-level security on comment does not hide the
	// customer's comments from a report that exists to surface them.
	ctx = repository.WithSystemIdentity(ctx)
	rows, err := s.repo.ListLongRunningMigrations(ctx, createdBefore, s.cfg.HoldTag, s.cfg.AccountsTag)
	if err != nil {
		return domain.MigrationCaseReport{}, err
	}
	recipients, err := s.repo.TeamGroupEmails(ctx, s.cfg.RecipientTeamKeys)
	if err != nil {
		return domain.MigrationCaseReport{}, err
	}

	report := BucketMigrationCases(rows)
	report.GeneratedAt = now
	report.OlderThanDays = days
	report.CreatedBefore = createdBefore
	report.Recipients = recipients
	return report, nil
}

// MigrationCaseCutoff is the newest creation instant a migration may have
// and still count as older than days: the start, in loc, of the day that
// many days before now. The query compares with <=, so a migration created
// at or before 00:00 of that day is listed and one created later that day
// is not -- it has not yet been open for the whole of those days. The
// cut-off is on a day boundary, rather than exactly days*24h before now,
// so a retried run later the same day lists the same migrations.
func MigrationCaseCutoff(now time.Time, days int, loc *time.Location) time.Time {
	d := now.In(loc).AddDate(0, 0, -days)
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, loc)
}

// BucketMigrationCases splits the scan's rows into its three tables. A row
// tagged on hold goes in OnHold; one tagged waiting on accounts goes in
// WaitingOnAccounts; a row with both tags goes in both. Only a row with
// neither tag reaches InProgress. The input order (oldest first) is kept
// within each table. Every table is non-nil so an empty one serialises as
// an empty list, not null.
func BucketMigrationCases(rows []domain.MigrationCaseReportRow) domain.MigrationCaseReport {
	report := domain.MigrationCaseReport{
		OnHold:            []domain.MigrationCaseReportRow{},
		WaitingOnAccounts: []domain.MigrationCaseReportRow{},
		InProgress:        []domain.MigrationCaseReportRow{},
		Recipients:        []string{},
	}
	for _, row := range rows {
		if row.OnHold {
			report.OnHold = append(report.OnHold, row)
		}
		if row.WaitingOnAccounts {
			report.WaitingOnAccounts = append(report.WaitingOnAccounts, row)
		}
		if !row.OnHold && !row.WaitingOnAccounts {
			report.InProgress = append(report.InProgress, row)
		}
	}
	return report
}
