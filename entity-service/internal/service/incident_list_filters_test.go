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
	"testing"
	"time"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
)

func TestParseIncidentListFiltersPostgres(t *testing.T) {
	f := domain.SearchIncidentsFilters{Filters: []domain.IncidentFieldFilter{
		{Field: "assignmentGroupId", Op: "in", Values: []string{"11111111-1111-1111-1111-111111111111"}},
		{Field: "category", Op: "in", Values: []string{" software ", "Network"}},
		{Field: "updatedOn", Op: "gte", Values: []string{"2026-01-01"}},
		{Field: "updatedOn", Op: "lte", Values: []string{"2026-01-31"}},
	}}
	if err := parseIncidentListFiltersPostgres(&f, time.Now()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.AssignmentGroupIDs) != 1 {
		t.Errorf("AssignmentGroupIDs = %v", f.AssignmentGroupIDs)
	}
	if len(f.Categories) != 2 || f.Categories[0] != "SOFTWARE" || f.Categories[1] != "NETWORK" {
		t.Errorf("Categories = %v", f.Categories)
	}
	if f.UpdatedStart == nil || f.UpdatedEnd == nil || !f.UpdatedEnd.After(*f.UpdatedStart) {
		t.Errorf("updated window = %v .. %v", f.UpdatedStart, f.UpdatedEnd)
	}
	// The main parser accepts the same request (it leaves these fields to this one).
	if _, _, _, _, _, _, _, _, err := parseIncidentFieldFiltersPostgres(f, time.Now()); err != nil {
		t.Fatalf("parseIncidentFieldFiltersPostgres: %v", err)
	}
}

func TestParseIncidentListFiltersPostgres_Refusals(t *testing.T) {
	for name, ff := range map[string][]domain.IncidentFieldFilter{
		"group not a uuid":   {{Field: "assignmentGroupId", Op: "in", Values: []string{"team-x"}}},
		"category wrong op":  {{Field: "category", Op: "eq", Values: []string{"SOFTWARE"}}},
		"category no values": {{Field: "category", Op: "in"}},
		"updated wrong op":   {{Field: "updatedOn", Op: "in", Values: []string{"2026-01-01"}}},
		"updated end before start": {
			{Field: "updatedOn", Op: "gte", Values: []string{"2026-02-01"}},
			{Field: "updatedOn", Op: "lte", Values: []string{"2026-01-01"}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := domain.SearchIncidentsFilters{Filters: ff}
			if err := parseIncidentListFiltersPostgres(&f, time.Now()); err == nil {
				t.Fatal("expected a validation error")
			}
		})
	}
}

func TestParseIncidentFieldFilters_ServiceNowRefusesPostgresOnlyFields(t *testing.T) {
	for _, field := range []string{"category", "updatedOn"} {
		_, err := ParseIncidentFieldFilters([]domain.IncidentFieldFilter{{Field: field, Op: "in", Values: []string{"x"}}}, time.Now())
		if err == nil {
			t.Errorf("%s: expected the ServiceNow parser to refuse it", field)
		}
	}
}
