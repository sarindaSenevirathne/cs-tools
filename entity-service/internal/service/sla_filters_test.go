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
	"strings"
	"testing"
	"time"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
)

func TestParseSLAFilter_ErrorMessagesMatchAcrossWorkItemTypes(t *testing.T) {
	tests := []struct {
		name      string
		field, op string
		values    []string
		wantErr   string
	}{
		{"percent bad op", slaFilterFieldPercent, "eq", []string{"5"}, `filters: field "taskSLABusinessElapsedPercent" does not support op "eq"`},
		{"percent no values", slaFilterFieldPercent, "gte", nil, `filters: field "taskSLABusinessElapsedPercent" op "gte" requires a non-empty values array`},
		{"percent negative", slaFilterFieldPercent, "gte", []string{"-1"}, `filters: field "taskSLABusinessElapsedPercent" op "gte" value "-1" must be a non-negative integer`},
		{"breached bad op", slaFilterFieldBreached, "in", []string{"true"}, `filters: field "slaBreached" does not support op "in"`},
		{"breached two values", slaFilterFieldBreached, "eq", []string{"true", "false"}, "filters: slaBreached eq requires exactly one value"},
		{"breached not bool", slaFilterFieldBreached, "eq", []string{"TRUE"}, `filters: field "slaBreached" op "eq" value "TRUE" must be a boolean`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sla *domain.TaskSLAFilter
			var breached *bool
			err := parseSLAFilter(tt.field, tt.op, tt.values, &sla, &breached)
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("err = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestIncidentSLAFilters(t *testing.T) {
	tr, fl := true, false
	t.Run("percent range accumulates on one filter", func(t *testing.T) {
		_, sla, err := incidentSLAFilters([]domain.IncidentFieldFilter{
			{Field: "taskSLABusinessElapsedPercent", Op: "gte", Values: []string{"75"}},
			{Field: "taskSLABusinessElapsedPercent", Op: "lte", Values: []string{"100"}},
		}, nil)
		if err != nil || sla == nil || *sla.MinBusinessElapsedPercent != 75 || *sla.MaxBusinessElapsedPercent != 100 {
			t.Fatalf("got %+v, %v", sla, err)
		}
	})
	t.Run("slaBreached folds into slaViolated", func(t *testing.T) {
		b, _, err := incidentSLAFilters([]domain.IncidentFieldFilter{{Field: "slaBreached", Op: "eq", Values: []string{"false"}}}, nil)
		if err != nil || b == nil || *b {
			t.Fatalf("got %v, %v", b, err)
		}
	})
	t.Run("slaViolated alone passes through", func(t *testing.T) {
		b, _, err := incidentSLAFilters(nil, &tr)
		if err != nil || b == nil || !*b {
			t.Fatalf("got %v, %v", b, err)
		}
	})
	t.Run("disagreeing slaViolated and slaBreached is a validation error", func(t *testing.T) {
		_, _, err := incidentSLAFilters([]domain.IncidentFieldFilter{{Field: "slaBreached", Op: "eq", Values: []string{"true"}}}, &fl)
		if _, ok := err.(*apierror.ValidationError); !ok {
			t.Fatalf("expected ValidationError, got %T: %v", err, err)
		}
	})
	t.Run("ParseIncidentFieldFilters carries both", func(t *testing.T) {
		p, err := ParseIncidentFieldFilters([]domain.IncidentFieldFilter{
			{Field: "slaBreached", Op: "eq", Values: []string{"true"}},
			{Field: "taskSLABusinessElapsedPercent", Op: "lte", Values: []string{"0"}},
		}, time.Now())
		if err != nil || p.SlaViolated == nil || !*p.SlaViolated || p.TaskSLA == nil || *p.TaskSLA.MaxBusinessElapsedPercent != 0 {
			t.Fatalf("got %+v, %v", p, err)
		}
	})
}

// fakeIncidentSLADelegate records whether a search/aggregate reached it.
type fakeIncidentSLADelegate struct {
	IncidentService
	searched, aggregated bool
}

func (f *fakeIncidentSLADelegate) SearchIncidents(context.Context, domain.SearchIncidentsRequest) (domain.SearchIncidentsResponse, error) {
	f.searched = true
	return domain.SearchIncidentsResponse{Total: 99}, nil
}

func (f *fakeIncidentSLADelegate) AggregateIncidents(context.Context, domain.AggregateIncidentsRequest) (domain.AggregateResponse, error) {
	f.aggregated = true
	return domain.AggregateResponse{TotalRecords: 99}, nil
}

// TestIncidentService_SLADelegate mirrors the case service's delegate test:
// an SLA-shaped filter (slaBreached, slaViolated or the percent range) is
// forwarded to the delegate for both search and aggregate; no SLA filter, or
// no delegate wired, stays on the Postgres repository.
func TestIncidentService_SLADelegate(t *testing.T) {
	breached := domain.SearchIncidentsFilters{Filters: []domain.IncidentFieldFilter{{Field: "slaBreached", Op: "eq", Values: []string{"true"}}}}
	violated := domain.SearchIncidentsFilters{Filters: []domain.IncidentFieldFilter{{Field: "slaViolated", Op: "eq", Values: []string{"true"}}}}
	percent := domain.SearchIncidentsFilters{Filters: []domain.IncidentFieldFilter{{Field: "taskSLABusinessElapsedPercent", Op: "gte", Values: []string{"75"}}}}
	plain := domain.SearchIncidentsFilters{Filters: []domain.IncidentFieldFilter{{Field: "madeSla", Op: "eq", Values: []string{"true"}}}}

	tests := []struct {
		name         string
		filters      domain.SearchIncidentsFilters
		withDelegate bool
		wantDelegate bool
	}{
		{"slaBreached delegates", breached, true, true},
		{"slaViolated delegates", violated, true, true},
		{"percent delegates", percent, true, true},
		{"no SLA filter does not delegate", plain, true, false},
		{"no delegate wired stays on postgres", breached, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &capturingIncidentSearchRepo{stubIncidentRepo: &stubIncidentRepo{}}
			delegate := &fakeIncidentSLADelegate{}
			svc := NewIncidentService(repo, nil)
			if tt.withDelegate {
				svc = WithSLASearchDelegate(svc, delegate)
			}

			resp, err := svc.SearchIncidents(context.Background(), domain.SearchIncidentsRequest{Filters: tt.filters})
			if err != nil {
				t.Fatalf("SearchIncidents: %v", err)
			}
			if delegate.searched != tt.wantDelegate || (tt.wantDelegate && resp.Total != 99) {
				t.Errorf("search delegated = %v (total %d), want %v", delegate.searched, resp.Total, tt.wantDelegate)
			}
			agg, err := svc.AggregateIncidents(context.Background(), domain.AggregateIncidentsRequest{Filters: tt.filters, GroupBy: "state"})
			if err != nil {
				t.Fatalf("AggregateIncidents: %v", err)
			}
			if delegate.aggregated != tt.wantDelegate || (tt.wantDelegate && agg.TotalRecords != 99) {
				t.Errorf("aggregate delegated = %v, want %v", delegate.aggregated, tt.wantDelegate)
			}
			wantRepoCalls := 2
			if tt.wantDelegate {
				wantRepoCalls = 0
			}
			if repo.calls != wantRepoCalls {
				t.Errorf("repository called %d times, want %d", repo.calls, wantRepoCalls)
			}
		})
	}
}

// TestIncidentService_PostgresPercentFilterReachesRepository proves the
// percent range is handed to the repository on the request (ParsedTaskSLA).
func TestIncidentService_PostgresPercentFilterReachesRepository(t *testing.T) {
	var got *domain.TaskSLAFilter
	repo := &recordingTaskSLARepo{stubIncidentRepo: &stubIncidentRepo{}, got: &got}
	svc := NewIncidentService(repo, nil)
	filters := domain.SearchIncidentsFilters{Filters: []domain.IncidentFieldFilter{{Field: "taskSLABusinessElapsedPercent", Op: "gte", Values: []string{"75"}}}}
	if _, err := svc.SearchIncidents(context.Background(), domain.SearchIncidentsRequest{Filters: filters}); err != nil {
		t.Fatal(err)
	}
	if got == nil || *got.MinBusinessElapsedPercent != 75 {
		t.Fatalf("search: repository saw %+v", got)
	}
	got = nil
	if _, err := svc.AggregateIncidents(context.Background(), domain.AggregateIncidentsRequest{Filters: filters, GroupBy: "state"}); err != nil {
		t.Fatal(err)
	}
	if got == nil || *got.MinBusinessElapsedPercent != 75 {
		t.Fatalf("aggregate: repository saw %+v", got)
	}
}

type recordingTaskSLARepo struct {
	*stubIncidentRepo
	got **domain.TaskSLAFilter
}

func (r *recordingTaskSLARepo) SearchIncidents(_ context.Context, req domain.SearchIncidentsRequest, _, _, _, _ []string, _, _ *bool, _, _ *time.Time) ([]domain.SearchIncidentView, int, error) {
	*r.got = req.Filters.ParsedTaskSLA
	return nil, 0, nil
}

func (r *recordingTaskSLARepo) AggregateIncidents(_ context.Context, req domain.SearchIncidentsRequest, _, _, _, _ []string, _, _ *bool, _, _ *time.Time, _ string, _ int) (domain.AggregateResponse, error) {
	*r.got = req.Filters.ParsedTaskSLA
	return domain.AggregateResponse{}, nil
}

// The ServiceNow-backed incident service cannot express a percent range
// through its existing endpoint, so it refuses rather than forwarding a
// filter that would be silently ignored. client is nil: the refusal precedes
// any call.
func TestSNIncidentService_PercentFilterRejected_BreachedAccepted(t *testing.T) {
	svc := NewServiceNowIncidentService(nil, nil)
	pct := domain.SearchIncidentsFilters{Filters: []domain.IncidentFieldFilter{{Field: "taskSLABusinessElapsedPercent", Op: "gte", Values: []string{"75"}}}}
	_, err := svc.SearchIncidents(contextWithUserIDToken("token"), domain.SearchIncidentsRequest{Filters: pct})
	ve, ok := err.(*apierror.ValidationError)
	if !ok || !strings.Contains(ve.Msg, "taskSLABusinessElapsedPercent") {
		t.Fatalf("search: expected ValidationError naming the field, got %T: %v", err, err)
	}
	_, err = svc.AggregateIncidents(contextWithUserIDToken("token"), domain.AggregateIncidentsRequest{Filters: pct, GroupBy: "state"})
	if _, ok := err.(*apierror.ValidationError); !ok {
		t.Fatalf("aggregate: expected ValidationError, got %T: %v", err, err)
	}
}
