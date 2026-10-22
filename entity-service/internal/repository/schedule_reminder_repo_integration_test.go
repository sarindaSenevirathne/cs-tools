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

package repository

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
)

// The reminder record: nothing for a period yet, the first send creates it,
// a later one counts itself and replaces the invites. Needs migration 0242;
// skipped unless ENTITY_TEST_DATABASE_URL is set.
func TestScheduleReminderIntegration_RecordAndRead(t *testing.T) {
	dsn := os.Getenv("ENTITY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ENTITY_TEST_DATABASE_URL is not set; skipping the live-database tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	const period = "2031-01-04"
	clean := func() {
		if _, err := pool.Exec(ctx, `DELETE FROM team_schedule_reminder_sent WHERE family = 'CRE' AND kind = 'weekend_notice' AND period_start = $1::date`, period); err != nil {
			t.Fatalf("clean: %v", err)
		}
	}
	clean()
	t.Cleanup(clean)

	repo := NewScheduleReminderRepository(pool)
	if rec, err := repo.Get(ctx, "CRE", "weekend_notice", period); err != nil || rec != nil {
		t.Fatalf("before any send: %+v, %v", rec, err)
	}

	first, err := repo.Record(ctx, "CRE", "weekend_notice", period, domain.RecordScheduleReminderRequest{
		SentBy: "rota_weekend_notice",
		Invites: []domain.ScheduleReminderInvite{
			{UID: "cre-rota-2031-01-04-cre_weekend@csm-team-schedule", Sequence: 1, Attendees: []string{"ann@example.com", "ben@example.com"}},
		},
	})
	if err != nil {
		t.Fatalf("first record: %v", err)
	}
	if first.SendCount != 1 || first.PeriodStart != period || len(first.Invites) != 1 || len(first.Invites[0].Attendees) != 2 {
		t.Fatalf("first record = %+v", first)
	}

	second, err := repo.Record(ctx, "CRE", "weekend_notice", period, domain.RecordScheduleReminderRequest{
		SentBy: "rota_weekend_reminder",
		Invites: []domain.ScheduleReminderInvite{
			{UID: "cre-rota-2031-01-04-cre_weekend@csm-team-schedule", Sequence: 2, Attendees: []string{"ann@example.com"}},
		},
	})
	if err != nil {
		t.Fatalf("second record: %v", err)
	}
	if second.SendCount != 2 || second.SentBy != "rota_weekend_reminder" || second.Invites[0].Sequence != 2 ||
		len(second.Invites[0].Attendees) != 1 || !second.FirstSentOn.Equal(first.FirstSentOn) || second.LastSentOn.Before(first.LastSentOn) {
		t.Fatalf("second record = %+v (first %+v)", second, first)
	}

	got, err := repo.Get(ctx, "CRE", "weekend_notice", period)
	if err != nil || got == nil || got.SendCount != 2 || got.Invites[0].Attendees[0] != "ann@example.com" {
		t.Fatalf("read back = %+v, %v", got, err)
	}
}
