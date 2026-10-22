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

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
)

func TestParseIncidentAnyOfPostgres(t *testing.T) {
	f := domain.SearchIncidentsFilters{AnyOf: []domain.IncidentFilterBranch{
		{Filters: []domain.IncidentFieldFilter{
			{Field: "state", Op: "in", Values: []string{"NEW"}},
			{Field: "priority", Op: "in", Values: []string{"CRITICAL"}},
		}},
		{Filters: []domain.IncidentFieldFilter{
			{Field: "category", Op: "in", Values: []string{"network"}},
			{Field: "assignmentGroupId", Op: "in", Values: []string{"11111111-1111-1111-1111-111111111111"}},
		}},
	}}
	if err := parseIncidentAnyOfPostgres(&f); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.OrGroups) != 2 {
		t.Fatalf("OrGroups = %+v", f.OrGroups)
	}
	if f.OrGroups[0].States[0] != "NEW" || f.OrGroups[0].Priorities[0] != "CRITICAL" {
		t.Errorf("branch 0 = %+v", f.OrGroups[0])
	}
	if f.OrGroups[1].Categories[0] != "NETWORK" || len(f.OrGroups[1].AssignmentGroupIDs) != 1 {
		t.Errorf("branch 1 = %+v", f.OrGroups[1])
	}
}

func TestParseIncidentAnyOfPostgres_Refusals(t *testing.T) {
	for name, branches := range map[string][]domain.IncidentFilterBranch{
		"empty branch":     {{}},
		"unknown field":    {{Filters: []domain.IncidentFieldFilter{{Field: "createdOn", Op: "in", Values: []string{"x"}}}}},
		"not op in":        {{Filters: []domain.IncidentFieldFilter{{Field: "state", Op: "eq", Values: []string{"NEW"}}}}},
		"no values":        {{Filters: []domain.IncidentFieldFilter{{Field: "state", Op: "in"}}}},
		"bad state":        {{Filters: []domain.IncidentFieldFilter{{Field: "state", Op: "in", Values: []string{"nope"}}}}},
		"group not a uuid": {{Filters: []domain.IncidentFieldFilter{{Field: "assignmentGroupId", Op: "in", Values: []string{"x"}}}}},
	} {
		t.Run(name, func(t *testing.T) {
			f := domain.SearchIncidentsFilters{AnyOf: branches}
			if err := parseIncidentAnyOfPostgres(&f); err == nil {
				t.Fatal("expected a validation error")
			}
		})
	}
	many := make([]domain.IncidentFilterBranch, maxListFilterBranches+1)
	for i := range many {
		many[i] = domain.IncidentFilterBranch{Filters: []domain.IncidentFieldFilter{{Field: "state", Op: "in", Values: []string{"NEW"}}}}
	}
	if err := parseIncidentAnyOfPostgres(&domain.SearchIncidentsFilters{AnyOf: many}); err == nil {
		t.Fatal("too many branches must be refused")
	}
}

func TestParseChangeRequestAnyOfPostgres(t *testing.T) {
	staff := repository.WithCallerIdentity(context.Background(), repository.SearchScope{Unrestricted: true})
	f := domain.SearchChangeRequestsFilters{AnyOf: []domain.ChangeRequestFilterBranch{
		{Filters: []domain.ChangeRequestFieldFilter{
			{Field: "state", Op: "in", Values: []string{"new"}},
			{Field: "type", Op: "in", Values: []string{"Normal"}},
		}},
		{Filters: []domain.ChangeRequestFieldFilter{
			{Field: "impact", Op: "in", Values: []string{"high"}},
			{Field: "tag", Op: "in", Values: []string{"hotfix"}},
		}},
	}}
	if err := parseChangeRequestAnyOfPostgres(staff, &f); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.OrGroups[0].States[0] != "NEW" || f.OrGroups[0].ChangeModels[0] != "NORMAL" {
		t.Errorf("branch 0 = %+v", f.OrGroups[0])
	}
	if f.OrGroups[1].Impacts[0] != "HIGH" || f.OrGroups[1].Tags[0] != "hotfix" {
		t.Errorf("branch 1 = %+v", f.OrGroups[1])
	}

	customer := repository.WithCallerIdentity(context.Background(), repository.SearchScope{ViewerEmail: "c@example.com"})
	tagOnly := domain.SearchChangeRequestsFilters{AnyOf: []domain.ChangeRequestFilterBranch{
		{Filters: []domain.ChangeRequestFieldFilter{{Field: "tag", Op: "in", Values: []string{"hotfix"}}}},
	}}
	var forbidden *apierror.ForbiddenError
	if err := parseChangeRequestAnyOfPostgres(customer, &tagOnly); !errors.As(err, &forbidden) {
		t.Fatalf("a customer's tag branch: err = %v, want ForbiddenError", err)
	}
	badType := domain.SearchChangeRequestsFilters{AnyOf: []domain.ChangeRequestFilterBranch{
		{Filters: []domain.ChangeRequestFieldFilter{{Field: "type", Op: "in", Values: []string{"model"}}}},
	}}
	if err := parseChangeRequestAnyOfPostgres(staff, &badType); err == nil {
		t.Fatal("a type with no change_model must be refused")
	}
}

func TestChangeRequestService_AnyOfReachesTheRepository(t *testing.T) {
	repo := &tagRecordingCRRepo{}
	svc := NewChangeRequestService(repo, stubUserRepo{})
	req := domain.SearchChangeRequestsRequest{Filters: domain.SearchChangeRequestsFilters{AnyOf: []domain.ChangeRequestFilterBranch{
		{Filters: []domain.ChangeRequestFieldFilter{{Field: "state", Op: "in", Values: []string{"new"}}}},
		{Filters: []domain.ChangeRequestFieldFilter{{Field: "impact", Op: "in", Values: []string{"low"}}}},
	}}}
	if _, err := svc.SearchChangeRequests(context.Background(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.got.OrGroups) != 2 {
		t.Fatalf("repository got OrGroups = %+v", repo.got.OrGroups)
	}
}
