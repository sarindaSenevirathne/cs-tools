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
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/auth"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
)

// ScheduleReminderService is the Team Schedule reminders' bookkeeping, for
// csm-scheduled-tasks: what a reminder last sent for a rota period, recording
// a send, and the family's rota admins to ask when a rota is not complete.
//
// Service credentials only. Nothing here is for a person: the record holds
// the addresses every invite went to, and the admin list is only for the
// reminder that asks them to finish the rota.
type ScheduleReminderService interface {
	// Get is the record for one period, or nil when nothing was sent yet.
	Get(ctx context.Context, family, kind, periodStart string) (*domain.ScheduleReminderSent, error)
	Record(ctx context.Context, family, kind, periodStart string, req domain.RecordScheduleReminderRequest) (domain.ScheduleReminderSent, error)
	// RotaAdmins is the family's active rota admins with an address, by name.
	RotaAdmins(ctx context.Context, family string) ([]domain.RotaAdminContact, error)
}

type scheduleReminderService struct {
	repo   repository.ScheduleReminderRepository
	access repository.RotaAccessRepository
	scope  AccessService
}

// NewScheduleReminderService constructs a ScheduleReminderService.
func NewScheduleReminderService(repo repository.ScheduleReminderRepository, access repository.RotaAccessRepository, scope AccessService) ScheduleReminderService {
	return &scheduleReminderService{repo: repo, access: access, scope: scope}
}

// Limits on a recorded send: a weekend is four invites of a handful of
// people, so these only stop nonsense, never a real rota.
const (
	maxReminderInvites   = 50
	maxReminderAttendees = 200
	maxReminderText      = 255
)

var reminderKindRe = regexp.MustCompile(`^[a-z_]{1,32}$`)

// requireService admits an internal service credential and nobody else.
func (s *scheduleReminderService) requireService(ctx context.Context) error {
	if err := RequireInternalCaller(ctx, s.scope, "team schedule reminders are internal only"); err != nil {
		return err
	}
	if auth.IdentityFromContext(ctx).UserEmail != "" {
		return &apierror.ForbiddenError{Msg: "team schedule reminders are for the scheduled tasks' service credential, not a user"}
	}
	return nil
}

func validReminderFamily(family string) error {
	switch family {
	case familyCRE, familySRE, familySME:
		return nil
	}
	return &apierror.ValidationError{Msg: "family must be CRE, SRE or SME"}
}

func validReminderKey(family, kind, periodStart string) error {
	if err := validReminderFamily(family); err != nil {
		return err
	}
	if !reminderKindRe.MatchString(kind) {
		return &apierror.ValidationError{Msg: "kind must be 1 to 32 lower-case letters or underscores"}
	}
	if _, err := time.Parse("2006-01-02", periodStart); err != nil {
		return &apierror.ValidationError{Msg: "periodStart must be a date, YYYY-MM-DD"}
	}
	return nil
}

func validReminderText(field, v string) error {
	if strings.TrimSpace(v) == "" || len(v) > maxReminderText || strings.ContainsAny(v, "\r\n") {
		return &apierror.ValidationError{Msg: field + " must be 1 to 255 characters on one line"}
	}
	return nil
}

func (s *scheduleReminderService) Get(ctx context.Context, family, kind, periodStart string) (*domain.ScheduleReminderSent, error) {
	if err := s.requireService(ctx); err != nil {
		return nil, err
	}
	if err := validReminderKey(family, kind, periodStart); err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, family, kind, periodStart)
}

func (s *scheduleReminderService) Record(ctx context.Context, family, kind, periodStart string, req domain.RecordScheduleReminderRequest) (domain.ScheduleReminderSent, error) {
	if err := s.requireService(ctx); err != nil {
		return domain.ScheduleReminderSent{}, err
	}
	if err := validReminderKey(family, kind, periodStart); err != nil {
		return domain.ScheduleReminderSent{}, err
	}
	if err := validReminderText("sentBy", req.SentBy); err != nil {
		return domain.ScheduleReminderSent{}, err
	}
	if len(req.Invites) > maxReminderInvites {
		return domain.ScheduleReminderSent{}, &apierror.ValidationError{Msg: "at most 50 invites"}
	}
	for _, inv := range req.Invites {
		if err := validReminderText("invites[].uid", inv.UID); err != nil {
			return domain.ScheduleReminderSent{}, err
		}
		if inv.Sequence < 0 {
			return domain.ScheduleReminderSent{}, &apierror.ValidationError{Msg: "invites[].sequence must not be negative"}
		}
		if len(inv.Attendees) > maxReminderAttendees {
			return domain.ScheduleReminderSent{}, &apierror.ValidationError{Msg: "at most 200 attendees an invite"}
		}
		for _, a := range inv.Attendees {
			if err := validReminderText("invites[].attendees[]", a); err != nil || !strings.Contains(a, "@") || strings.ContainsAny(a, " \t") {
				return domain.ScheduleReminderSent{}, &apierror.ValidationError{Msg: "invites[].attendees[] must be email addresses"}
			}
		}
	}
	return s.repo.Record(ctx, family, kind, periodStart, req)
}

func (s *scheduleReminderService) RotaAdmins(ctx context.Context, family string) ([]domain.RotaAdminContact, error) {
	if err := s.requireService(ctx); err != nil {
		return nil, err
	}
	if err := validReminderFamily(family); err != nil {
		return nil, err
	}
	grants, err := s.access.ListRotaAdmins(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := []domain.RotaAdminContact{}
	for _, g := range grants {
		email := strings.TrimSpace(g.Email)
		if g.Family != family || !g.Active || email == "" || seen[strings.ToLower(email)] {
			continue
		}
		seen[strings.ToLower(email)] = true
		out = append(out, domain.RotaAdminContact{Name: g.Name, Email: email})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}
