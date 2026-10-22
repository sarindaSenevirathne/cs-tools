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

// The create form's "Risk and impact analysis" and "Backout plan" must show on the detail
// page's Plan view straight after create. Create used to write them only to
// risk_impact_analysis / backout_plan, while the detail read, the PATCH and the portal use
// impact_description / rollback_process, so the Rollback plan section stayed empty until the
// plan was typed again in Edit. Both create paths now write both columns, and the read falls
// back to the older pair for change requests created before the fix. Skipped without
// CHANGE_REQUEST_TEST_DSN; reuses changeRequestCreatePool's cleanup (subject / SN id).
//
//	CHANGE_REQUEST_TEST_DSN=postgres://... go test ./internal/repository/ -run ChangeRequestPlanFieldsIntegration

package repository_test

import (
	"context"
	"testing"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
)

const (
	crPlanRollback = "<p>1. Restore the previous image tag.</p><p>2. Re-run the smoke tests.</p>"
	crPlanRisk     = "<p>Gateway restarts for about 30 seconds.</p>"
)

func strOf(s string) *string { return &s }

// assertPlanFields reads the change request back the way the detail page does and checks the
// two plan fields, plus the columns the read and the PATCH use.
func assertPlanFields(t *testing.T, scoped *repository.Scoped, repo repository.ChangeRequestRepository, id, wantRollback, wantRisk string) {
	t.Helper()
	sys := repository.WithSystemIdentity(context.Background())
	cr, err := repo.GetChangeRequestByID(sys, id)
	if err != nil {
		t.Fatalf("GetChangeRequestByID: %v", err)
	}
	if cr.RollbackPlan == nil || *cr.RollbackPlan != wantRollback {
		t.Fatalf("RollbackPlan = %v, want %q", cr.RollbackPlan, wantRollback)
	}
	if cr.ImpactDescription == nil || *cr.ImpactDescription != wantRisk {
		t.Fatalf("ImpactDescription = %v, want %q", cr.ImpactDescription, wantRisk)
	}
	var rollbackProcess, backout, impactDescription, riskAnalysis *string
	if err := scoped.QueryRow(sys,
		`SELECT rollback_process, backout_plan, impact_description, risk_impact_analysis FROM change_request WHERE id = $1`, id,
	).Scan(&rollbackProcess, &backout, &impactDescription, &riskAnalysis); err != nil {
		t.Fatalf("read back plan columns: %v", err)
	}
	for name, got := range map[string]*string{"rollback_process": rollbackProcess, "backout_plan": backout} {
		if got == nil || *got != wantRollback {
			t.Fatalf("%s = %v, want %q", name, got, wantRollback)
		}
	}
	for name, got := range map[string]*string{"impact_description": impactDescription, "risk_impact_analysis": riskAnalysis} {
		if got == nil || *got != wantRisk {
			t.Fatalf("%s = %v, want %q", name, got, wantRisk)
		}
	}
}

func planCreateRequest() domain.CreateChangeRequestRequest {
	return domain.CreateChangeRequestRequest{
		Subject:            crCreateSubject,
		Type:               crCreateType(domain.ChangeRequestTypeNormal),
		BackoutPlan:        strOf(crPlanRollback),
		RiskImpactAnalysis: strOf(crPlanRisk),
	}
}

func TestChangeRequestPlanFieldsIntegration_PortalCreateShowsThePlanAtOnce(t *testing.T) {
	scoped := changeRequestCreatePool(t)
	repo := repository.NewChangeRequestRepository(scoped)
	resp, err := repo.CreateChangeRequest(repository.WithSystemIdentity(context.Background()), planCreateRequest(), "cr-create-test@test.local")
	if err != nil {
		t.Fatalf("CreateChangeRequest: %v", err)
	}
	assertPlanFields(t, scoped, repo, resp.ChangeRequest.ID, crPlanRollback, crPlanRisk)
}

func TestChangeRequestPlanFieldsIntegration_ServiceNowFirstCreateShowsThePlanAtOnce(t *testing.T) {
	scoped := changeRequestCreatePool(t)
	repo := repository.NewChangeRequestRepository(scoped)
	if _, err := repo.CreateChangeRequestFromServiceNow(repository.WithSystemIdentity(context.Background()),
		planCreateRequest(), crCreateSNID, crCreateSNNumber, "cr-create-test@test.local"); err != nil {
		t.Fatalf("CreateChangeRequestFromServiceNow: %v", err)
	}
	assertPlanFields(t, scoped, repo, crCreateSNID, crPlanRollback, crPlanRisk)
}

// A change request created before the fix holds the plan only in backout_plan /
// risk_impact_analysis: the read falls back to them.
func TestChangeRequestPlanFieldsIntegration_OlderRowsShowThePlanFromTheOlderColumns(t *testing.T) {
	scoped := changeRequestCreatePool(t)
	repo := repository.NewChangeRequestRepository(scoped)
	sys := repository.WithSystemIdentity(context.Background())
	resp, err := repo.CreateChangeRequest(sys, planCreateRequest(), "cr-create-test@test.local")
	if err != nil {
		t.Fatalf("CreateChangeRequest: %v", err)
	}
	id := resp.ChangeRequest.ID
	if _, err := scoped.Exec(sys, `UPDATE change_request SET rollback_process = NULL, impact_description = NULL WHERE id = $1`, id); err != nil {
		t.Fatalf("make the row look like one created before the fix: %v", err)
	}
	cr, err := repo.GetChangeRequestByID(sys, id)
	if err != nil {
		t.Fatalf("GetChangeRequestByID: %v", err)
	}
	if cr.RollbackPlan == nil || *cr.RollbackPlan != crPlanRollback {
		t.Fatalf("RollbackPlan = %v, want the backout_plan fallback %q", cr.RollbackPlan, crPlanRollback)
	}
	if cr.ImpactDescription == nil || *cr.ImpactDescription != crPlanRisk {
		t.Fatalf("ImpactDescription = %v, want the risk_impact_analysis fallback %q", cr.ImpactDescription, crPlanRisk)
	}
}

// An edit of the plan replaces what create wrote, and clearing it in Edit stays cleared: the
// read does not fall back to the older column once the field has been set to "".
func TestChangeRequestPlanFieldsIntegration_EditReplacesAndClearsThePlan(t *testing.T) {
	scoped := changeRequestCreatePool(t)
	repo := repository.NewChangeRequestRepository(scoped)
	sys := repository.WithSystemIdentity(context.Background())
	resp, err := repo.CreateChangeRequest(sys, planCreateRequest(), "cr-create-test@test.local")
	if err != nil {
		t.Fatalf("CreateChangeRequest: %v", err)
	}
	id := resp.ChangeRequest.ID

	edited := "<p>Roll back with the blue/green switch.</p>"
	if _, err := repo.PatchChangeRequest(sys, id, domain.PatchChangeRequestRequest{RollbackPlan: strOf(edited)}, "cr-create-test@test.local"); err != nil {
		t.Fatalf("PatchChangeRequest (edit): %v", err)
	}
	cr, err := repo.GetChangeRequestByID(sys, id)
	if err != nil {
		t.Fatalf("GetChangeRequestByID: %v", err)
	}
	if cr.RollbackPlan == nil || *cr.RollbackPlan != edited {
		t.Fatalf("RollbackPlan after edit = %v, want %q", cr.RollbackPlan, edited)
	}

	if _, err := repo.PatchChangeRequest(sys, id, domain.PatchChangeRequestRequest{RollbackPlan: strOf("")}, "cr-create-test@test.local"); err != nil {
		t.Fatalf("PatchChangeRequest (clear): %v", err)
	}
	cr, err = repo.GetChangeRequestByID(sys, id)
	if err != nil {
		t.Fatalf("GetChangeRequestByID: %v", err)
	}
	if cr.RollbackPlan != nil && *cr.RollbackPlan != "" {
		t.Fatalf("RollbackPlan after clearing = %q, want empty", *cr.RollbackPlan)
	}
}
