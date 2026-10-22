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
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
)

// ScheduleReminderRepository stores what the Team Schedule's reminders last
// sent (team_schedule_reminder_sent, migration 0242).
type ScheduleReminderRepository interface {
	// Get is the record for one period, or nil when nothing was sent yet.
	Get(ctx context.Context, family, kind, periodStart string) (*domain.ScheduleReminderSent, error)
	// Record stores a send: the first creates the row, a later one counts
	// itself and replaces the invites.
	Record(ctx context.Context, family, kind, periodStart string, req domain.RecordScheduleReminderRequest) (domain.ScheduleReminderSent, error)
}

type scheduleReminderRepository struct{ db *pgxpool.Pool }

// NewScheduleReminderRepository constructs a ScheduleReminderRepository.
func NewScheduleReminderRepository(db *pgxpool.Pool) ScheduleReminderRepository {
	return &scheduleReminderRepository{db: db}
}

const scheduleReminderColumns = `family, kind, period_start::text, first_sent_on, last_sent_on, send_count, invites, sent_by`

func scanScheduleReminder(row pgx.Row) (domain.ScheduleReminderSent, error) {
	var out domain.ScheduleReminderSent
	var invites []byte
	if err := row.Scan(&out.Family, &out.Kind, &out.PeriodStart, &out.FirstSentOn, &out.LastSentOn, &out.SendCount, &invites, &out.SentBy); err != nil {
		return domain.ScheduleReminderSent{}, err
	}
	out.Invites = []domain.ScheduleReminderInvite{}
	if err := json.Unmarshal(invites, &out.Invites); err != nil {
		return domain.ScheduleReminderSent{}, fmt.Errorf("decode reminder invites: %w", err)
	}
	return out, nil
}

func (r *scheduleReminderRepository) Get(ctx context.Context, family, kind, periodStart string) (*domain.ScheduleReminderSent, error) {
	rec, err := scanScheduleReminder(r.db.QueryRow(ctx, `SELECT `+scheduleReminderColumns+`
		  FROM team_schedule_reminder_sent
		 WHERE family = $1 AND kind = $2 AND period_start = $3::date`, family, kind, periodStart))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get reminder record: %w", err)
	}
	return &rec, nil
}

func (r *scheduleReminderRepository) Record(ctx context.Context, family, kind, periodStart string, req domain.RecordScheduleReminderRequest) (domain.ScheduleReminderSent, error) {
	invites := req.Invites
	if invites == nil {
		invites = []domain.ScheduleReminderInvite{}
	}
	raw, err := json.Marshal(invites)
	if err != nil {
		return domain.ScheduleReminderSent{}, fmt.Errorf("encode reminder invites: %w", err)
	}
	rec, err := scanScheduleReminder(r.db.QueryRow(ctx, `
		INSERT INTO team_schedule_reminder_sent (family, kind, period_start, invites, sent_by)
		VALUES ($1, $2, $3::date, $4::jsonb, $5)
		ON CONFLICT (family, kind, period_start) DO UPDATE
		   SET last_sent_on = now(),
		       send_count   = team_schedule_reminder_sent.send_count + 1,
		       invites      = EXCLUDED.invites,
		       sent_by      = EXCLUDED.sent_by
		RETURNING `+scheduleReminderColumns, family, kind, periodStart, string(raw), req.SentBy))
	if err != nil {
		return domain.ScheduleReminderSent{}, fmt.Errorf("record reminder: %w", err)
	}
	return rec, nil
}
