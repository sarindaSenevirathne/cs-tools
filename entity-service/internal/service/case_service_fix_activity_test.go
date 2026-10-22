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
	"testing"
	"time"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/auth"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
)

type fixActivityFixture struct {
	repo     *stubCaseRepo
	activity []recordedActivity
}

// newFixActivityFixture answers the prior-workaround-stamp lookup and
// records every activity row the service writes.
func newFixActivityFixture(prevWorkaround *time.Time) *fixActivityFixture {
	f := &fixActivityFixture{}
	f.repo = &stubCaseRepo{
		getCaseWorkaroundProvidedOn: func(context.Context, string) (*time.Time, error) {
			return prevWorkaround, nil
		},
		updateCaseFields: func(context.Context, domain.UpdateCaseRequest, string, string) (time.Time, error) {
			return time.Date(2026, 10, 19, 9, 30, 0, 0, time.UTC), nil
		},
		recordCaseFieldChangeActivity: func(_ context.Context, _, field, oldValue, newValue, actor string) error {
			f.activity = append(f.activity, recordedActivity{field, oldValue, newValue, actor})
			return nil
		},
	}
	return f
}

func fixActivityUserCtx(t *testing.T) context.Context {
	return contextWithUserIDToken(fakeJWTWithEmail(t, "jane.doe@example.com"))
}

func fixActivityService(repo *stubCaseRepo) CaseService {
	userRepo := stubUserRepo{getUserByEmail: func(_ context.Context, email string) (domain.User, error) {
		return domain.User{ID: "00000000-0000-0000-0000-000000000000", Email: email}, nil
	}}
	return NewCaseService(repo, userRepo, nil, alwaysUnrestrictedAccess{}, nil)
}

func TestCaseService_UpdateCase_WorkaroundSetLogsActivity(t *testing.T) {
	f := newFixActivityFixture(nil)
	provided := true
	if _, err := fixActivityService(f.repo).UpdateCase(fixActivityUserCtx(t), domain.UpdateCaseRequest{ID: testDeploymentUUID, WorkaroundProvided: &provided}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := recordedActivity{"workaround_provided_on", "", "2026-10-19T09:30:00Z", "jane.doe@example.com"}
	if len(f.activity) != 1 || f.activity[0] != want {
		t.Errorf("activity = %+v, want [%+v]", f.activity, want)
	}
}

func TestCaseService_UpdateCase_WorkaroundSetAfterPreviousStampLogsOldValue(t *testing.T) {
	was := time.Date(2026, 10, 18, 8, 0, 0, 0, time.UTC)
	f := newFixActivityFixture(&was)
	provided := true
	if _, err := fixActivityService(f.repo).UpdateCase(fixActivityUserCtx(t), domain.UpdateCaseRequest{ID: testDeploymentUUID, WorkaroundProvided: &provided}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := recordedActivity{"workaround_provided_on", "2026-10-18T08:00:00Z", "2026-10-19T09:30:00Z", "jane.doe@example.com"}
	if len(f.activity) != 1 || f.activity[0] != want {
		t.Errorf("activity = %+v, want [%+v]", f.activity, want)
	}
}

func TestCaseService_UpdateCase_WorkaroundPriorStampReadFailureStillUpdates(t *testing.T) {
	f := newFixActivityFixture(nil)
	f.repo.getCaseWorkaroundProvidedOn = func(context.Context, string) (*time.Time, error) { return nil, context.DeadlineExceeded }
	provided := false
	if _, err := fixActivityService(f.repo).UpdateCase(fixActivityUserCtx(t), domain.UpdateCaseRequest{ID: testDeploymentUUID, WorkaroundProvided: &provided}); err != nil {
		t.Fatalf("update must not fail on a prior-stamp read error: %v", err)
	}
	if len(f.activity) != 0 {
		t.Errorf("activity = %+v, want none", f.activity)
	}
}

func TestCaseService_UpdateCase_WorkaroundRecallLogsActivity(t *testing.T) {
	was := time.Date(2026, 10, 18, 8, 0, 0, 0, time.UTC)
	f := newFixActivityFixture(&was)
	provided := false
	if _, err := fixActivityService(f.repo).UpdateCase(fixActivityUserCtx(t), domain.UpdateCaseRequest{ID: testDeploymentUUID, WorkaroundProvided: &provided}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := recordedActivity{"workaround_provided_on", "2026-10-18T08:00:00Z", "", "jane.doe@example.com"}
	if len(f.activity) != 1 || f.activity[0] != want {
		t.Errorf("activity = %+v, want [%+v]", f.activity, want)
	}
}

func TestCaseService_UpdateCase_WorkaroundRecallWhenUnsetLogsNothing(t *testing.T) {
	f := newFixActivityFixture(nil)
	provided := false
	if _, err := fixActivityService(f.repo).UpdateCase(fixActivityUserCtx(t), domain.UpdateCaseRequest{ID: testDeploymentUUID, WorkaroundProvided: &provided}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.activity) != 0 {
		t.Errorf("activity = %+v, want none", f.activity)
	}
}

func TestCaseService_UpdateCase_MarkFixIssuedLogsActivity(t *testing.T) {
	issued := time.Date(2026, 10, 19, 10, 15, 0, 0, time.UTC)
	tests := []struct {
		name       string
		alreadySet bool
		ctx        context.Context
		want       []recordedActivity
	}{
		{"first write by user", false, fixActivityUserCtx(t), []recordedActivity{{"fix_issued_on", "", "2026-10-19T10:15:00Z", "jane.doe@example.com"}}},
		{"first write by M2M caller", false, auth.WithIdentity(context.Background(), auth.Identity{ClientID: "integration-client"}), []recordedActivity{{"fix_issued_on", "", "2026-10-19T10:15:00Z", "integration-client"}}},
		{"already set", true, fixActivityUserCtx(t), nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixActivityFixture(nil)
			f.repo.markCaseFixIssued = func(context.Context, string) (time.Time, bool, error) { return issued, tc.alreadySet, nil }
			mark := true
			if _, err := fixActivityService(f.repo).UpdateCase(tc.ctx, domain.UpdateCaseRequest{ID: testDeploymentUUID, MarkFixIssued: &mark}); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(f.activity) != len(tc.want) {
				t.Fatalf("activity = %+v, want %+v", f.activity, tc.want)
			}
			for i := range tc.want {
				if f.activity[i] != tc.want[i] {
					t.Errorf("activity[%d] = %+v, want %+v", i, f.activity[i], tc.want[i])
				}
			}
		})
	}
}
