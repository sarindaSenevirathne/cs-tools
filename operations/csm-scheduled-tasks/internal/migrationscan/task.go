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

// Package migrationscan is the weekly migration case scan sub-cron: it
// fetches the long-running migration engagements from entity-service and
// mails them, in three tables, to the CS teams' mail groups. The third
// report-style sub-cron in this component, built the way its own CLAUDE.md
// ("Per-task report emails") describes: its own template, its own render
// function, and its recipients wired through cmd/server/main.go.
package migrationscan

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/wso2-open-operations/cs-tools/operations/csm-scheduled-tasks/internal/migrationreport"
	"github.com/wso2-open-operations/cs-tools/operations/csm-scheduled-tasks/internal/notify"
)

// ReportFetcher is the subset of *migrationreport.Client this package
// depends on.
type ReportFetcher interface {
	Report(ctx context.Context, olderThanDays int) (migrationreport.Report, error)
}

// EmailSender is the subset of *notify.Client this package depends on.
type EmailSender interface {
	SendEmail(ctx context.Context, to, cc []string, subject, htmlBody string) error
}

// Subject is the scan's mail subject.
const Subject = "[Report] Weekly Migration Case Scan"

// SendReport returns a registry.Task.Handler that fetches the scan for
// migrations older than olderThanDays and mails it.
//
// The audience is the union of two sources: the mail groups entity-service
// derives from the teams it is configured with (the scan's `recipients`)
// and this task's own SUB_CRON_RECIPIENTS entry (to, cc). Teams own their
// mail groups, so the team table is the source of truth for who reads the
// scan; the configured lists exist for the readers who are not a team --
// leadership and management lists. The scan is always sent, empty tables
// included, as it always was.
//
// emailsEnabled is cmd/server/main.go's ALERTS_ENABLED. When false the
// handler succeeds without contacting entity-service. Unlike the stale and
// open case reports, an empty audience is an error, not a quiet skip: a
// scan nobody receives looks exactly like a quiet week, and the whole point
// of moving the recipients into the team table is that an empty mail group
// should be noticed.
func SendReport(reports ReportFetcher, email EmailSender, olderThanDays int, to, cc []string, caseLinkBase string, emailsEnabled bool) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		if !emailsEnabled {
			return nil
		}

		report, err := reports.Report(ctx, olderThanDays)
		if err != nil {
			return fmt.Errorf("migrationscan: fetch scan: %w", err)
		}

		audienceTo, audienceCc := Audience(to, cc, report.Recipients)
		if len(audienceTo) == 0 {
			return fmt.Errorf("migrationscan: no recipients: no team mail group was derived and the task has no configured \"to\"")
		}
		slog.Info("sending the migration case scan",
			"olderThanDays", report.OlderThanDays,
			"onHold", len(report.OnHold),
			"waitingOnAccounts", len(report.WaitingOnAccounts),
			"inProgress", len(report.InProgress),
			"to", len(audienceTo), "cc", len(audienceCc),
			"derived", len(report.Recipients))

		body := notify.RenderMigrationCaseReport(report, caseLinkBase)
		if err := email.SendEmail(ctx, audienceTo, audienceCc, Subject, body); err != nil {
			return fmt.Errorf("migrationscan: send scan: %w", err)
		}
		return nil
	}
}

// Audience merges the configured lists with the derived mail groups: To is
// the configured "to" followed by the derived groups, Cc the configured
// "cc". Addresses are trimmed, blanks dropped, and duplicates removed
// ignoring case, with To winning over Cc so nobody is addressed twice.
func Audience(to, cc, derived []string) (audienceTo, audienceCc []string) {
	seen := map[string]bool{}
	add := func(dst []string, src []string) []string {
		for _, raw := range src {
			addr := strings.TrimSpace(raw)
			if addr == "" {
				continue
			}
			key := strings.ToLower(addr)
			if seen[key] {
				continue
			}
			seen[key] = true
			dst = append(dst, addr)
		}
		return dst
	}
	audienceTo = add(nil, to)
	audienceTo = add(audienceTo, derived)
	audienceCc = add(nil, cc)
	return audienceTo, audienceCc
}
