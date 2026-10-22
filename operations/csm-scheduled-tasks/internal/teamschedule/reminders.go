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

package teamschedule

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/wso2-open-operations/cs-tools/operations/csm-scheduled-tasks/internal/apierror"
)

// ReminderInvite is one calendar event a reminder sent: its identity, the
// sequence it was last sent with, and who it went to.
type ReminderInvite struct {
	UID       string   `json:"uid"`
	Sequence  int      `json:"sequence"`
	Attendees []string `json:"attendees"`
}

// ReminderRecord is what a reminder last sent for one rota period
// (entity-service's team_schedule_reminder_sent).
type ReminderRecord struct {
	FirstSentOn time.Time        `json:"firstSentOn"`
	LastSentOn  time.Time        `json:"lastSentOn"`
	SendCount   int              `json:"sendCount"`
	Invites     []ReminderInvite `json:"invites"`
	SentBy      string           `json:"sentBy"`
}

// RotaAdmin is one rota admin of a family.
type RotaAdmin struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// notFoundSaying is whether err is entity-service's own "nothing there"
// answer: a 404 whose JSON message is want. A 404 with any other body -- a
// route that does not exist yet, because this task was deployed before the
// entity-service that adds it -- is a real failure: reading it as "nothing
// sent" or "nothing saved" would re-send the weekend notice every evening and
// ignore reminders an admin switched off.
func notFoundSaying(err error, want string) bool {
	var apiErr *apierror.Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		return false
	}
	var body struct {
		Message string `json:"message"`
	}
	return json.Unmarshal([]byte(apiErr.Body), &body) == nil && body.Message == want
}

// The messages entity-service answers its 404s with on these routes.
const (
	nothingSentMessage    = "nothing was sent for this period"
	noSettingsMessage     = "no rota reminder settings saved for this family"
	nothingWaitingMessage = "no rota reminder request is waiting"
)

func reminderPath(family, kind, periodStart string) string {
	return "/team-schedule/reminders/" + url.PathEscape(family) + "/" + url.PathEscape(kind) + "/" + url.PathEscape(periodStart)
}

// GetReminder is what the reminder kind last sent for the period starting on
// periodStart (YYYY-MM-DD), or nil when nothing was sent yet.
func (c *Client) GetReminder(ctx context.Context, family, kind, periodStart string) (*ReminderRecord, error) {
	body, err := c.do(ctx, http.MethodGet, reminderPath(family, kind, periodStart), nil)
	if notFoundSaying(err, nothingSentMessage) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var rec ReminderRecord
	if err := json.Unmarshal(body, &rec); err != nil {
		return nil, fmt.Errorf("teamschedule: decode reminder record: %w", err)
	}
	return &rec, nil
}

// RecordReminder stores a send: the invites just sent replace the stored ones.
func (c *Client) RecordReminder(ctx context.Context, family, kind, periodStart string, invites []ReminderInvite, sentBy string) error {
	if invites == nil {
		invites = []ReminderInvite{}
	}
	body, err := json.Marshal(struct {
		Invites []ReminderInvite `json:"invites"`
		SentBy  string           `json:"sentBy"`
	}{invites, sentBy})
	if err != nil {
		return fmt.Errorf("teamschedule: encode reminder record: %w", err)
	}
	_, err = c.do(ctx, http.MethodPut, reminderPath(family, kind, periodStart), body)
	return err
}

// RotaAdmins is the family's active rota admins with an address.
func (c *Client) RotaAdmins(ctx context.Context, family string) ([]RotaAdmin, error) {
	body, err := c.do(ctx, http.MethodGet, "/team-schedule/rota-admins/"+url.PathEscape(family), nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Admins []RotaAdmin `json:"admins"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("teamschedule: decode rota admins: %w", err)
	}
	return resp.Admins, nil
}
