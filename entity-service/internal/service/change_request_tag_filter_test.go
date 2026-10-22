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
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
)

func TestParseChangeRequestFieldFilters_Tag(t *testing.T) {
	p, err := ParseChangeRequestFieldFilters([]domain.ChangeRequestFieldFilter{
		{Field: "tag", Op: "in", Values: []string{"Hotfix"}},
		{Field: "tag", Op: "notIn", Values: []string{"patch", "draft"}},
	}, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Tags) != 1 || p.Tags[0] != "Hotfix" {
		t.Errorf("Tags = %v", p.Tags)
	}
	if len(p.ExcludeTags) != 2 {
		t.Errorf("ExcludeTags = %v", p.ExcludeTags)
	}
}

func TestParseChangeRequestFieldFilters_TagRejectsOtherOpsAndEmptyValues(t *testing.T) {
	for _, f := range []domain.ChangeRequestFieldFilter{
		{Field: "tag", Op: "eq", Values: []string{"x"}},
		{Field: "tag", Op: "in"},
		{Field: "assignedUserId", Op: "notIn", Values: []string{"11111111-1111-1111-1111-111111111111"}},
	} {
		if _, err := ParseChangeRequestFieldFilters([]domain.ChangeRequestFieldFilter{f}, time.Now()); err == nil {
			t.Errorf("%+v: expected a validation error", f)
		}
	}
}

type tagRecordingCRRepo struct {
	stubChangeRequestRepo
	got *domain.SearchChangeRequestsFilters
}

func (r *tagRecordingCRRepo) SearchChangeRequests(_ context.Context, req domain.SearchChangeRequestsRequest, _, _ *time.Time, _ *string, _ []string) ([]domain.SearchChangeRequestView, int, error) {
	f := req.Filters
	r.got = &f
	return nil, 0, nil
}

func tagSearch() domain.SearchChangeRequestsRequest {
	return domain.SearchChangeRequestsRequest{Filters: domain.SearchChangeRequestsFilters{
		Filters: []domain.ChangeRequestFieldFilter{{Field: "tag", Op: "in", Values: []string{"hotfix"}}},
	}}
}

func TestChangeRequestService_SearchByTag_StaffReachesTheRepository(t *testing.T) {
	repo := &tagRecordingCRRepo{}
	svc := NewChangeRequestService(repo, stubUserRepo{})
	ctx := repository.WithCallerIdentity(context.Background(), repository.SearchScope{Unrestricted: true})
	if _, err := svc.SearchChangeRequests(ctx, tagSearch()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.got == nil || len(repo.got.Tags) != 1 || repo.got.Tags[0] != "hotfix" {
		t.Fatalf("repository did not receive the tag filter: %+v", repo.got)
	}
}

func TestChangeRequestService_SearchByTag_RefusedForRestrictedCallers(t *testing.T) {
	for name, ctx := range map[string]context.Context{
		"customer":    repository.WithCallerIdentity(context.Background(), repository.SearchScope{ViewerEmail: "c@example.com"}),
		"no identity": context.Background(),
	} {
		t.Run(name, func(t *testing.T) {
			repo := &tagRecordingCRRepo{}
			svc := NewChangeRequestService(repo, stubUserRepo{})
			_, err := svc.SearchChangeRequests(ctx, tagSearch())
			var forbidden *apierror.ForbiddenError
			if !errors.As(err, &forbidden) {
				t.Fatalf("err = %v, want ForbiddenError", err)
			}
			if repo.got != nil {
				t.Fatal("repository was called")
			}
		})
	}
}

func TestParseChangeRequestFieldFilters_ListColumns(t *testing.T) {
	p, err := ParseChangeRequestFieldFilters([]domain.ChangeRequestFieldFilter{
		{Field: "updatedOn", Op: "gte", Values: []string{"2026-01-01"}},
		{Field: "plannedStartOn", Op: "lte", Values: []string{"2026-02-01"}},
		{Field: "plannedEndOn", Op: "gte", Values: []string{"2026-01-15"}},
		{Field: "type", Op: "in", Values: []string{"Normal", " emergency "}},
	}, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.UpdatedStart == nil || p.PlannedStartTo == nil || p.PlannedEndFrom == nil {
		t.Errorf("bounds not parsed: %+v", p)
	}
	if len(p.Types) != 2 || p.Types[0] != "normal" || p.Types[1] != "emergency" {
		t.Errorf("Types = %v", p.Types)
	}
	if !changeRequestPostgresOnlyFilters(p) {
		t.Error("the ServiceNow service must refuse these filters")
	}
}

func TestChangeRequestService_ListFiltersReachTheRepository(t *testing.T) {
	repo := &tagRecordingCRRepo{}
	svc := NewChangeRequestService(repo, stubUserRepo{})
	req := domain.SearchChangeRequestsRequest{Filters: domain.SearchChangeRequestsFilters{Filters: []domain.ChangeRequestFieldFilter{
		{Field: "assignmentGroupId", Op: "in", Values: []string{"11111111-1111-1111-1111-111111111111"}},
		{Field: "assignedUserId", Op: "in", Values: []string{"22222222-2222-2222-2222-222222222222"}},
		{Field: "type", Op: "in", Values: []string{"standard"}},
	}}}
	if _, err := svc.SearchChangeRequests(context.Background(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.got.AssignmentGroupIDs) != 1 || len(repo.got.AssignedUserIDs) != 1 || len(repo.got.Types) != 1 {
		t.Fatalf("repository got %+v", repo.got)
	}

	bad := domain.SearchChangeRequestsRequest{Filters: domain.SearchChangeRequestsFilters{Filters: []domain.ChangeRequestFieldFilter{
		{Field: "type", Op: "in", Values: []string{"not-a-type"}},
	}}}
	if _, err := svc.SearchChangeRequests(context.Background(), bad); err == nil {
		t.Fatal("an unknown type must be refused, not ignored")
	}
}

func TestChangeRequestService_SortsByThePlannedWindow(t *testing.T) {
	for _, field := range []domain.ChangeRequestSortField{
		domain.ChangeRequestSortFieldPlannedStartOn, domain.ChangeRequestSortFieldPlannedEndOn,
	} {
		repo := &tagRecordingCRRepo{}
		svc := NewChangeRequestService(repo, stubUserRepo{})
		req := domain.SearchChangeRequestsRequest{SortBy: domain.ChangeRequestSort{Field: field, Order: domain.ChangeRequestSortOrderAsc}}
		if _, err := svc.SearchChangeRequests(context.Background(), req); err != nil {
			t.Fatalf("%s: unexpected error: %v", field, err)
		}
	}
}
