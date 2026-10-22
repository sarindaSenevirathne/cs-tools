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

// Integration test: PATCH of a project's onboarding status and owner on the
// Postgres data source, read back through GetProjectByID. Runs against a real
// Postgres, skipped without CASE_STATS_TEST_DSN.
//
//	CASE_STATS_TEST_DSN=postgres://... go test ./internal/repository/ -run ProjectOnboardingUpdateIntegration

package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
)

const (
	pouInternal = "92000000-0000-0000-0000-000000000001"
	pouExternal = "92000000-0000-0000-0000-000000000002"
	pouInactive = "92000000-0000-0000-0000-000000000003"
	pouProject  = "92000000-0000-0000-0000-0000000000a1"
)

func TestProjectOnboardingUpdateIntegration(t *testing.T) {
	pool := caseStatsPool(t)
	ctx := repository.WithSystemIdentity(context.Background())

	cleanup := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM project WHERE id = $1`, pouProject)
		_, _ = pool.Exec(ctx, `DELETE FROM "user" WHERE id IN ($1,$2,$3)`, pouInternal, pouExternal, pouInactive)
	}
	cleanup()
	t.Cleanup(cleanup)

	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed (%.80s): %v", sql, err)
		}
	}
	// user_type is trigger-derived from role membership, so it is set directly
	// after the insert.
	mkUser := func(id, name, userType string, active bool) {
		mustExec(`INSERT INTO "user" (id, created_on, updated_on, user_name, first_name, last_name, email, is_active)
			VALUES ($1, now(), now(), $2::text, 'Jane', 'Doe', $2::text||'@example.com', $3)`, id, name, active)
		mustExec(`UPDATE "user" SET user_type = $2::user_type_enum WHERE id = $1`, id, userType)
	}
	mkUser(pouInternal, "pou-internal", "INTERNAL", true)
	mkUser(pouExternal, "pou-external", "EXTERNAL", true)
	mkUser(pouInactive, "pou-inactive", "INTERNAL", false)
	mustExec(`INSERT INTO project (id, created_on, updated_on, created_by, updated_by, key, name, sf_id, onboarding_status)
		VALUES ($1, now(), now(), 'pou-test', 'pou-test', 'POUPROJ', 'POUPROJ', 'sf-POUPROJ', 'NOT_STARTED')`, pouProject)

	repo := repository.NewProjectRepository(repository.NewScoped(pool))
	scope := repository.SearchScope{Unrestricted: true}
	str := func(s string) *string { return &s }
	patch := func(req domain.ProjectUpdateRequest) (domain.ProjectUpdateResult, error) {
		return repo.UpdateProject(ctx, pouProject, req, "jane.doe@example.com")
	}

	res, err := patch(domain.ProjectUpdateRequest{OnboardingStatus: str("IN_PROGRESS"), OnboardingOwner: str(pouInternal)})
	if err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}
	if res.OnboardingStatus == nil || *res.OnboardingStatus != "In-Progress" || res.OnboardingOwner == nil || res.OnboardingOwner.ID != pouInternal {
		t.Errorf("result onboarding = %v / %+v", res.OnboardingStatus, res.OnboardingOwner)
	}
	got, err := repo.GetProjectByID(ctx, pouProject, scope)
	if err != nil {
		t.Fatalf("GetProjectByID: %v", err)
	}
	if got.OnboardingStatus == nil || *got.OnboardingStatus != "In-Progress" || got.OnboardingOwner == nil || got.OnboardingOwner.ID != pouInternal || got.OnboardingOwner.Name != "Jane Doe" {
		t.Errorf("read back status=%v owner=%+v", got.OnboardingStatus, got.OnboardingOwner)
	}

	// Status-only patch leaves the owner; owner-only patch leaves the status.
	if _, err := patch(domain.ProjectUpdateRequest{OnboardingStatus: str("on-hold")}); err != nil {
		t.Fatalf("status-only: %v", err)
	}
	got, _ = repo.GetProjectByID(ctx, pouProject, scope)
	if got.OnboardingStatus == nil || *got.OnboardingStatus != "On-Hold" || got.OnboardingOwner == nil || got.OnboardingOwner.ID != pouInternal {
		t.Errorf("after status-only status=%v owner=%+v", got.OnboardingStatus, got.OnboardingOwner)
	}

	// Rejections leave the row untouched.
	for name, req := range map[string]domain.ProjectUpdateRequest{
		"bad status":    {OnboardingStatus: str("Paused")},
		"external user": {OnboardingOwner: str(pouExternal)},
		"inactive user": {OnboardingOwner: str(pouInactive)},
		"unknown user":  {OnboardingOwner: str("00000000-0000-0000-0000-000000000000")},
		"not a uuid":    {OnboardingOwner: str("jane")},
	} {
		_, err := patch(req)
		var ve *apierror.ValidationError
		if !errors.As(err, &ve) {
			t.Errorf("%s: err = %v, want ValidationError", name, err)
		}
	}
	got, _ = repo.GetProjectByID(ctx, pouProject, scope)
	if got.OnboardingStatus == nil || *got.OnboardingStatus != "On-Hold" || got.OnboardingOwner == nil || got.OnboardingOwner.ID != pouInternal {
		t.Errorf("after rejections status=%v owner=%+v, want unchanged", got.OnboardingStatus, got.OnboardingOwner)
	}
}
