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
	"errors"
	"strings"
	"testing"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/auth"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
)

type fakeReminderRepo struct {
	recorded *domain.RecordScheduleReminderRequest
	got      string
}

func (f *fakeReminderRepo) Get(_ context.Context, family, kind, period string) (*domain.ScheduleReminderSent, error) {
	f.got = family + "/" + kind + "/" + period
	return nil, nil
}

func (f *fakeReminderRepo) Record(_ context.Context, family, kind, period string, req domain.RecordScheduleReminderRequest) (domain.ScheduleReminderSent, error) {
	f.recorded = &req
	return domain.ScheduleReminderSent{Family: family, Kind: kind, PeriodStart: period, SendCount: 1, Invites: req.Invites, SentBy: req.SentBy}, nil
}

// adminsRepo lists rota admins of several families, one of them inactive.
type adminsRepo struct{ *fakeRotaAccessRepo }

func (adminsRepo) ListRotaAdmins(context.Context) ([]domain.RotaAdminGrant, error) {
	return []domain.RotaAdminGrant{
		{Family: "CRE", Name: "Zoe", Email: "zoe@example.com", Active: true},
		{Family: "CRE", Name: "Amal", Email: "amal@example.com", Active: true},
		{Family: "CRE", Name: "Amal again", Email: "AMAL@example.com", Active: true},
		{Family: "CRE", Name: "Gone", Email: "gone@example.com", Active: false},
		{Family: "CRE", Name: "No address", Active: true},
		{Family: "SRE", Name: "Sam", Email: "sam@example.com", Active: true},
	}, nil
}

func serviceCtx() context.Context {
	return auth.WithIdentity(context.Background(), auth.Identity{Validated: true, ClientID: "m2m"})
}

func newReminders(repo *fakeReminderRepo) ScheduleReminderService {
	return NewScheduleReminderService(repo, adminsRepo{&fakeRotaAccessRepo{}}, alwaysUnrestrictedAccess{})
}

// A person, even an admin, is refused: the record holds every invite's
// addresses and the list is only for the reminder that asks admins to act.
func TestScheduleReminder_RefusesAUser(t *testing.T) {
	svc := newReminders(&fakeReminderRepo{})
	user := auth.WithIdentity(context.Background(), auth.Identity{Validated: true, UserEmail: "ann@example.com", Roles: []string{"cs_admin"}})
	for name, call := range map[string]func() error{
		"get": func() error { _, err := svc.Get(user, "CRE", "weekend_notice", "2026-10-24"); return err },
		"record": func() error {
			_, err := svc.Record(user, "CRE", "weekend_notice", "2026-10-24", domain.RecordScheduleReminderRequest{SentBy: "x"})
			return err
		},
		"admins": func() error { _, err := svc.RotaAdmins(user, "CRE"); return err },
	} {
		var forbidden *apierror.ForbiddenError
		if err := call(); !errors.As(err, &forbidden) {
			t.Errorf("%s: err = %v, want forbidden", name, err)
		}
	}
}

func TestScheduleReminder_ChecksTheKeyAndTheBody(t *testing.T) {
	repo := &fakeReminderRepo{}
	svc := newReminders(repo)
	ok := domain.RecordScheduleReminderRequest{SentBy: "rota_weekend_notice", Invites: []domain.ScheduleReminderInvite{
		{UID: "cre-rota-2026-10-24-cre_weekend@csm-team-schedule", Sequence: 2, Attendees: []string{"ann@example.com"}},
	}}
	for _, c := range []struct {
		name, family, kind, period string
		req                        domain.RecordScheduleReminderRequest
		want                       string
	}{
		{"unknown family", "ABC", "weekend_notice", "2026-10-24", ok, "family"},
		{"bad kind", "CRE", "Weekend-Notice", "2026-10-24", ok, "kind"},
		{"bad date", "CRE", "weekend_notice", "24/10/2026", ok, "periodStart"},
		{"no sender", "CRE", "weekend_notice", "2026-10-24", domain.RecordScheduleReminderRequest{}, "sentBy"},
		{"not an address", "CRE", "weekend_notice", "2026-10-24", domain.RecordScheduleReminderRequest{SentBy: "x", Invites: []domain.ScheduleReminderInvite{{UID: "u", Attendees: []string{"ann"}}}}, "email addresses"},
		{"negative sequence", "CRE", "weekend_notice", "2026-10-24", domain.RecordScheduleReminderRequest{SentBy: "x", Invites: []domain.ScheduleReminderInvite{{UID: "u", Sequence: -1}}}, "sequence"},
	} {
		_, err := svc.Record(serviceCtx(), c.family, c.kind, c.period, c.req)
		var invalid *apierror.ValidationError
		if !errors.As(err, &invalid) || !strings.Contains(invalid.Msg, c.want) {
			t.Errorf("%s: err = %v, want a validation error about %s", c.name, err, c.want)
		}
	}
	if repo.recorded != nil {
		t.Fatalf("an invalid request reached the repository: %+v", repo.recorded)
	}
	if _, err := svc.Record(serviceCtx(), "CRE", "weekend_notice", "2026-10-24", ok); err != nil || repo.recorded == nil {
		t.Fatalf("valid record: err = %v, recorded = %v", err, repo.recorded)
	}
	if rec, err := svc.Get(serviceCtx(), "CRE", "weekend_notice", "2026-10-24"); err != nil || rec != nil || repo.got != "CRE/weekend_notice/2026-10-24" {
		t.Fatalf("get = %v, %v (asked %q)", rec, err, repo.got)
	}
}

// The family's active admins with an address, each once, by name.
func TestScheduleReminder_RotaAdminsOfOneFamily(t *testing.T) {
	admins, err := newReminders(&fakeReminderRepo{}).RotaAdmins(serviceCtx(), "CRE")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, a := range admins {
		got = append(got, a.Name+" <"+a.Email+">")
	}
	if strings.Join(got, ", ") != "Amal <amal@example.com>, Zoe <zoe@example.com>" {
		t.Fatalf("admins = %v", got)
	}
}
