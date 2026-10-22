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
	"reflect"
	"testing"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
)

// TestIncidentFieldChanges: only fields the request named are compared, a re-sent value is no
// change, the derived fields follow what drives them, and values read as a person would read them.
func TestIncidentFieldChanges(t *testing.T) {
	before := domain.IncidentView{
		State:           strPtr("NEW"),
		Priority:        strPtr("LOW"),
		Impact:          strPtr("LOW"),
		Urgency:         strPtr("MEDIUM"),
		Category:        strPtr("SERVICE_INTERRUPTION"),
		Subject:         strPtr("test"),
		AssignmentGroup: &domain.EntityRef{ID: "g1", Name: "MS/PC SRE Group"},
	}
	after := before
	after.State = strPtr("IN_PROGRESS")
	after.Priority = strPtr("MODERATE")
	after.Impact = strPtr("HIGH")
	after.Category = strPtr("SECURITY")                              // changed, but not by this request
	after.AssignedTo = &domain.EntityRef{ID: "u1", Name: "Jane Doe"} // set from none
	after.AssignmentGroup = &domain.EntityRef{ID: "g2"}              // a group with no name reads as its id

	state := domain.IncidentStateInProgress
	impact := domain.IncidentImpactHigh
	urgency := domain.IncidentUrgencyMedium // re-sent with the value it already had
	subject := "test"                       // likewise
	user, group := "u1", "g2"
	req := domain.UpdateIncidentRequest{
		State: &state, Impact: &impact, Urgency: &urgency, Subject: &subject,
		AssignedEngineerID: &user, AssignmentGroupID: &group,
	}

	got := incidentFieldChanges(req, before, after)
	want := []repository.IncidentFieldChange{
		{Field: "state", OldValue: "New", NewValue: "In Progress"},
		{Field: "priority", OldValue: "Low", NewValue: "Moderate"},
		{Field: "impact", OldValue: "Low", NewValue: "High"},
		{Field: "assigned_to_id", OldValue: "", NewValue: "Jane Doe"},
		{Field: "assignment_group_id", OldValue: "MS/PC SRE Group", NewValue: "g2"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("incidentFieldChanges =\n  %+v\nwant\n  %+v", got, want)
	}
}

// TestIncidentFieldChanges_PriorityUnmoved: an impact or urgency change that leaves the computed
// priority where it was records no priority row.
func TestIncidentFieldChanges_PriorityUnmoved(t *testing.T) {
	before := domain.IncidentView{Priority: strPtr("LOW"), Impact: strPtr("LOW"), Urgency: strPtr("LOW")}
	after := before
	after.Urgency = strPtr("MEDIUM")
	urgency := domain.IncidentUrgencyMedium
	got := incidentFieldChanges(domain.UpdateIncidentRequest{Urgency: &urgency}, before, after)
	want := []repository.IncidentFieldChange{
		{Field: "urgency", OldValue: "Low", NewValue: "Medium"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("incidentFieldChanges = %+v, want %+v", got, want)
	}
}

// TestIncidentFieldChanges_ClearedReference: unassigning reads as the old name and an empty new value.
func TestIncidentFieldChanges_ClearedReference(t *testing.T) {
	before := domain.IncidentView{AssignedTo: &domain.EntityRef{ID: "u1", Name: "Jane Doe"}}
	empty := ""
	got := incidentFieldChanges(domain.UpdateIncidentRequest{AssignedEngineerID: &empty}, before, domain.IncidentView{})
	want := []repository.IncidentFieldChange{{Field: "assigned_to_id", OldValue: "Jane Doe", NewValue: ""}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("incidentFieldChanges = %+v, want %+v", got, want)
	}
}

// updateRecordingRepo answers GetIncidentByID with before on the first read and after on every
// later one: UpdateIncident reads the incident once before its write and again after it.
func updateRecordingRepo(before, after domain.IncidentView) *stubIncidentRepo {
	reads := 0
	return &stubIncidentRepo{
		updateIncidentLifecycle: func(context.Context, string, repository.IncidentLifecycleUpdate, string) error { return nil },
		getIncidentByID: func(context.Context, string) (domain.IncidentView, error) {
			reads++
			if reads == 1 {
				return before, nil
			}
			return after, nil
		},
	}
}

// TestIncidentService_UpdateIncident_RecordsFieldChangeActivity: a field change lands in the
// activity feed, attributed to the caller, once the update has been written.
func TestIncidentService_UpdateIncident_RecordsFieldChangeActivity(t *testing.T) {
	before := newTestIncidentView(testDeploymentUUID)
	before.Impact, before.Urgency, before.Priority = strPtr("LOW"), strPtr("MEDIUM"), strPtr("LOW")
	after := before
	after.Impact, after.Priority = strPtr("HIGH"), strPtr("MODERATE")
	repo := updateRecordingRepo(before, after)
	svc := NewIncidentServiceWithPublisher(repo, stubUpdateIncidentUserRepo{email: "jane.doe@example.com"}, nil)

	impact := domain.IncidentImpactHigh
	if _, err := svc.UpdateIncident(contextWithUserIDToken(fakeJWTWithEmail(t, "jane.doe@example.com")),
		domain.UpdateIncidentRequest{ID: testDeploymentUUID, Impact: &impact}); err != nil {
		t.Fatalf("UpdateIncident: %v", err)
	}
	if len(repo.recordedChanges) != 1 {
		t.Fatalf("RecordIncidentFieldChanges calls = %d, want 1", len(repo.recordedChanges))
	}
	rec := repo.recordedChanges[0]
	if rec.incidentID != testDeploymentUUID || rec.actorEmail != "jane.doe@example.com" {
		t.Errorf("recorded for %q by %q, want %q by jane.doe@example.com", rec.incidentID, rec.actorEmail, testDeploymentUUID)
	}
	want := []repository.IncidentFieldChange{
		{Field: "priority", OldValue: "Low", NewValue: "Moderate"},
		{Field: "impact", OldValue: "Low", NewValue: "High"},
	}
	if !reflect.DeepEqual(rec.changes, want) {
		t.Errorf("changes = %+v, want %+v", rec.changes, want)
	}
}

// TestIncidentService_UpdateIncident_ActivityFailureDoesNotFailTheUpdate: the update has already
// committed, so a failed activity write is logged, not returned.
func TestIncidentService_UpdateIncident_ActivityFailureDoesNotFailTheUpdate(t *testing.T) {
	before := newTestIncidentView(testDeploymentUUID)
	before.Impact = strPtr("LOW")
	after := before
	after.Impact = strPtr("HIGH")
	repo := updateRecordingRepo(before, after)
	repo.recordChangesErr = errors.New("insert refused")
	svc := NewIncidentServiceWithPublisher(repo, stubUpdateIncidentUserRepo{email: "jane.doe@example.com"}, nil)

	impact := domain.IncidentImpactHigh
	if _, err := svc.UpdateIncident(contextWithUserIDToken(fakeJWTWithEmail(t, "jane.doe@example.com")),
		domain.UpdateIncidentRequest{ID: testDeploymentUUID, Impact: &impact}); err != nil {
		t.Fatalf("UpdateIncident returned the activity failure: %v", err)
	}
	if len(repo.recordedChanges) != 1 {
		t.Errorf("RecordIncidentFieldChanges calls = %d, want 1", len(repo.recordedChanges))
	}
}

// TestIncidentService_UpdateIncident_NoActivityWithoutABaseline: when the incident could not be read
// before the change there is nothing to compare against, so nothing is recorded.
func TestIncidentService_UpdateIncident_NoActivityWithoutABaseline(t *testing.T) {
	reads := 0
	repo := &stubIncidentRepo{
		updateIncidentLifecycle: func(context.Context, string, repository.IncidentLifecycleUpdate, string) error { return nil },
		getIncidentByID: func(_ context.Context, id string) (domain.IncidentView, error) {
			reads++
			if reads == 1 {
				return domain.IncidentView{}, errors.New("read failed")
			}
			return newTestIncidentView(id), nil
		},
	}
	svc := NewIncidentServiceWithPublisher(repo, stubUpdateIncidentUserRepo{email: "jane.doe@example.com"}, nil)

	impact := domain.IncidentImpactHigh
	if _, err := svc.UpdateIncident(contextWithUserIDToken(fakeJWTWithEmail(t, "jane.doe@example.com")),
		domain.UpdateIncidentRequest{ID: testDeploymentUUID, Impact: &impact}); err != nil {
		t.Fatalf("UpdateIncident: %v", err)
	}
	if len(repo.recordedChanges) != 0 {
		t.Errorf("recorded %+v with no baseline, want nothing", repo.recordedChanges)
	}
}

// TestIncidentService_UpdateIncident_NotesOnlyRecordNoFieldChange: a work note is a comment in the
// feed already; it neither reads the incident beforehand nor writes a field change.
func TestIncidentService_UpdateIncident_NotesOnlyRecordNoFieldChange(t *testing.T) {
	repo := &stubIncidentRepo{
		createIncidentComment: func(_ context.Context, _ string, _ domain.CommentType, content, _ string) (domain.CaseComment, error) {
			return domain.CaseComment{Content: content}, nil
		},
		getIncidentByID: func(_ context.Context, id string) (domain.IncidentView, error) {
			return newTestIncidentView(id), nil
		},
	}
	svc := NewIncidentServiceWithPublisher(repo, stubUpdateIncidentUserRepo{email: "jane.doe@example.com"}, nil)

	note := "checked the gateway logs"
	if _, err := svc.UpdateIncident(contextWithUserIDToken(fakeJWTWithEmail(t, "jane.doe@example.com")),
		domain.UpdateIncidentRequest{ID: testDeploymentUUID, WorkNotes: &note}); err != nil {
		t.Fatalf("UpdateIncident(work note): %v", err)
	}
	if len(repo.recordedChanges) != 0 {
		t.Errorf("a notes-only update recorded %+v, want nothing", repo.recordedChanges)
	}
}

// TestIncidentService_SearchIncidentActivities_HasMore: hasMore says whether rows lie past this
// page, so a client paging until it is false reads the whole feed.
func TestIncidentService_SearchIncidentActivities_HasMore(t *testing.T) {
	for _, tt := range []struct {
		name          string
		offset, total int
		rows          int
		want          bool
	}{
		{"first of several pages", 0, 120, 50, true},
		{"last full page", 50, 100, 50, false},
		{"short last page", 100, 120, 20, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := &stubIncidentRepo{
				searchActivities: func(context.Context, domain.SearchIncidentActivitiesRequest) ([]domain.CaseActivity, int, error) {
					return make([]domain.CaseActivity, tt.rows), tt.total, nil
				},
			}
			svc := NewIncidentServiceWithPublisher(repo, stubUpdateIncidentUserRepo{}, nil)
			resp, err := svc.SearchIncidentActivities(context.Background(), domain.SearchIncidentActivitiesRequest{
				IncidentID: testDeploymentUUID,
				Pagination: domain.Pagination{Offset: tt.offset, Limit: 50},
			})
			if err != nil {
				t.Fatalf("SearchIncidentActivities: %v", err)
			}
			if resp.HasMore != tt.want {
				t.Errorf("HasMore = %v, want %v", resp.HasMore, tt.want)
			}
		})
	}
}

// TestHandOffIncidentToSpecialist_RecordsFieldChangeActivity: a handoff's group move and cleared
// assignee land in the activity feed, attributed to the caller, after the handoff has committed.
func TestHandOffIncidentToSpecialist_RecordsFieldChangeActivity(t *testing.T) {
	id := hoIncidentID
	before := domain.IncidentView{
		ID:              &id,
		AssignedTo:      &domain.EntityRef{ID: "33333333-3333-4333-8333-333333333333", Name: "Jane Doe"},
		AssignmentGroup: &domain.EntityRef{ID: "44444444-4444-4444-8444-444444444444", Name: "Choreo Operations"},
	}
	after := domain.IncidentView{ID: &id, AssignmentGroup: &domain.EntityRef{ID: hoChoreoSpecial, Name: "Choreo Special Ops"}}
	reads := 0
	repo := &stubIncidentRepo{
		getIncidentByID: func(context.Context, string) (domain.IncidentView, error) {
			reads++
			if reads == 1 {
				return before, nil
			}
			return after, nil
		},
		applySpecialistHandoff: func(_ context.Context, _, _ string, plan func(repository.SpecialistHandoffSnapshot) (repository.SpecialistHandoffPlan, error)) (repository.SpecialistHandoffWritten, error) {
			snap := hoSnapshot(hoChoreoService, "44444444-4444-4444-8444-444444444444", "IN_PROGRESS")
			if _, err := plan(snap); err != nil {
				return repository.SpecialistHandoffWritten{}, err
			}
			return repository.SpecialistHandoffWritten{Before: snap, GroupName: "Choreo Special Ops"}, nil
		},
		createIncidentComment: func(context.Context, string, domain.CommentType, string, string) (domain.CaseComment, error) {
			return domain.CaseComment{}, nil
		},
	}
	svc := WithSpecialistHandoffConfig(NewIncidentService(repo, nil), hoConfig(t))
	noIssue := false
	_, err := svc.HandOffIncidentToSpecialist(contextWithUserIDToken(fakeJWTWithEmail(t, "jane.doe@example.com")),
		domain.HandOffIncidentToSpecialistRequest{IncidentID: hoIncidentID, ReasonCode: domain.IncidentSpecialistHandoffReasonNoRunbook,
			EscalationTeam: hoTeam("choreo-special-ops"), CreateGithubIssue: &noIssue})
	if err != nil {
		t.Fatalf("handoff: %v", err)
	}
	if len(repo.recordedChanges) != 1 {
		t.Fatalf("RecordIncidentFieldChanges calls = %d, want 1", len(repo.recordedChanges))
	}
	rec := repo.recordedChanges[0]
	if rec.incidentID != hoIncidentID || rec.actorEmail != "jane.doe@example.com" {
		t.Errorf("recorded for %q by %q", rec.incidentID, rec.actorEmail)
	}
	want := []repository.IncidentFieldChange{
		{Field: "assigned_to_id", OldValue: "Jane Doe", NewValue: ""},
		{Field: "assignment_group_id", OldValue: "Choreo Operations", NewValue: "Choreo Special Ops"},
	}
	if !reflect.DeepEqual(rec.changes, want) {
		t.Errorf("changes = %+v, want %+v", rec.changes, want)
	}
}
