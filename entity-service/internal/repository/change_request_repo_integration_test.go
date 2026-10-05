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

package repository_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
)

// Skipped without CHANGE_REQUEST_TEST_DSN, the same convention every other
// repository integration test in this package uses.
//
//	CHANGE_REQUEST_TEST_DSN=postgres://... go test ./internal/repository/ -run ChangeRequestIntegration

const (
	changeRequestApprovalTestID          = "36666666-0000-0000-0000-000000000001"
	changeRequestApprovalApproverUserID  = "36666666-0000-0000-0000-000000000003"
	changeRequestApprovalApproverUserID2 = "36666666-0000-0000-0000-000000000004"
	changeRequestApprovalApproverUserID3 = "36666666-0000-0000-0000-000000000005"

	// changeRequestAssignedTeamTestID is its own id, distinct from
	// changeRequestApprovalTestID above, so the AssignedTeamID tests below
	// never race the approval tests' seed/cleanup of the same work_item row.
	changeRequestAssignedTeamTestID = "36666666-0000-0000-0000-000000000006"

	// seededGroupID is scripts/csm-compose/seed-entity-service.sql's one
	// "group" row ("Example Corp ABT", id 901) -- reused here rather than
	// inserting a fresh "group" row for this test alone, to avoid growing
	// that seed file for something it already covers.
	seededGroupID = "00000000-0000-0000-0000-000000000901"

	// unknownGroupID is a well-formed UUID that is not the id of any "group"
	// row -- used to exercise assignment_group_id's FK violation path.
	unknownGroupID = "36666666-aaaa-0000-0000-000000000000"

	// changeRequestAssessGateTestID is its own id, distinct from every other
	// test's change request above, so the Assess-gate/approver-provisioning
	// tests below never race any of them over the same work_item row.
	changeRequestAssessGateTestID = "36666666-0000-0000-0000-000000000007"

	// changeRequestAssessGateMemberUserID{,2} are seeded as team_member rows
	// against changeRequestAssessGateGroupID (distinct from every other
	// test's own user ids) to exercise Assess's auto-provisioned approvers.
	changeRequestAssessGateMemberUserID  = "36666666-0000-0000-0000-000000000008"
	changeRequestAssessGateMemberUserID2 = "36666666-0000-0000-0000-000000000009"

	// changeRequestAssessGateGroupID is its own dedicated "group" row,
	// deliberately NOT seededGroupID -- the membership-provisioning tests
	// below assert the *exact, exhaustive* set of a group's members, which
	// is unsafe to share with seededGroupID: that fixture is reused by other
	// tests (and by hand during local manual verification) for its mere FK
	// validity, with no guarantee nothing else ever attaches a team_member
	// row to it. A real collision of exactly this kind was hit once already
	// (a manual verification session left two real users' team_member rows
	// pointed at seededGroupID, inflating this test's own membership count
	// until that residue was cleaned up) -- a dedicated, test-owned group
	// makes that structurally impossible instead of merely unlikely.
	changeRequestAssessGateGroupID = "36666666-0000-0000-0000-00000000000a"

	// changeRequestReviewGateTestID is its own id, distinct from every other
	// test's change request above (including the Assess- and Authorize-gate
	// tests), so the Review-gate/approver-provisioning tests below never race
	// any of them over the same work_item row.
	changeRequestReviewGateTestID = "36666666-0000-0000-0000-00000000000f"

	// changeRequestReviewGateMemberUserID{,2} are seeded as team_member rows
	// against changeRequestReviewGateGroupID (distinct from every other
	// test's own user ids) to exercise Review's auto-provisioned approvers.
	changeRequestReviewGateMemberUserID  = "36666666-0000-0000-0000-000000000010"
	changeRequestReviewGateMemberUserID2 = "36666666-0000-0000-0000-000000000011"

	// changeRequestReviewGateGroupID is its own dedicated "group" row,
	// deliberately not shared with changeRequestAssessGateGroupID,
	// changeRequestAuthorizeGateGroupID or seededGroupID -- same isolation
	// reasoning as changeRequestAssessGateGroupID's own doc comment.
	changeRequestReviewGateGroupID = "36666666-0000-0000-0000-000000000012"

	// changeRequestOnHoldTestID is its own id, distinct from every other
	// test's change request above, so the on-hold gate tests below never
	// race any of them over the same work_item row.
	changeRequestOnHoldTestID = "36666666-0000-0000-0000-000000000013"

	// Customer-approval/customer-review authorization fixtures (change_request.
	// is_customer_approved/is_customer_reviewed -- see
	// authorizeChangeRequestCustomerFlagWrite's own doc comment in
	// change_request_repo.go for the full rule). A distinct id prefix
	// ("37777777...") from every fixture above, own account/projects/
	// contacts, so these tests never race any other test in this file over
	// a shared row.
	crCustomerFlagAccountID        = "37777777-0000-0000-0000-000000000001"
	crCustomerFlagAccountContactID = "37777777-0000-0000-0000-000000000002"

	// crCustomerFlagProjectID is the change request's OWN project in every
	// test below except the "different project" one -- it gets a REGISTERED
	// PORTAL_USER contact (crCustomerFlagPortalUserEmail) and a REGISTERED,
	// but NOT PORTAL_USER-holding, contact's own membership is never added
	// to it (see crCustomerFlagNoRoleProjectID for that case instead).
	crCustomerFlagProjectID = "37777777-0000-0000-0000-000000000003"
	// crCustomerFlagOtherProjectID is a DIFFERENT project, with its own
	// REGISTERED PORTAL_USER contact (crCustomerFlagOtherProjectEmail) --
	// exercises "a registered contact's own authorization does not transfer
	// to a change request on a different project."
	crCustomerFlagOtherProjectID = "37777777-0000-0000-0000-000000000004"
	// crCustomerFlagNoRoleProjectID has exactly one contact
	// (crCustomerFlagWrongRoleEmail), REGISTERED but holding only
	// SECURITY_CONTACT -- a literal "project with no qualifying contact" per
	// this feature's own product decision (point 3 of its design): nobody
	// external can ever flip a flag on a change request linked to this
	// project, which must have zero bearing on that change request's own
	// state transitions.
	crCustomerFlagNoRoleProjectID = "37777777-0000-0000-0000-000000000005"

	crCustomerFlagPortalUserEmail   = "cr-customer-flag-portal-user@example.test"
	crCustomerFlagOtherProjectEmail = "cr-customer-flag-other-project@example.test"
	crCustomerFlagWrongRoleEmail    = "cr-customer-flag-wrong-role@example.test"
	// crCustomerFlagInvitedEmail is INVITED (not yet REGISTERED) on
	// crCustomerFlagProjectID despite holding PORTAL_USER -- exercises
	// "registration state matters as much as role."
	crCustomerFlagInvitedEmail = "cr-customer-flag-invited@example.test"

	// One change request (work_item) id per test below, so none of them
	// ever race another over the same row.
	crCustomerFlagInternalTestID         = "37777777-0000-0000-0000-000000000010"
	crCustomerFlagQualifyingTestID       = "37777777-0000-0000-0000-000000000011"
	crCustomerFlagDifferentProjectTestID = "37777777-0000-0000-0000-000000000012"
	crCustomerFlagWrongRoleTestID        = "37777777-0000-0000-0000-000000000013"
	crCustomerFlagInvitedTestID          = "37777777-0000-0000-0000-000000000014"
	crCustomerFlagLockedTestID           = "37777777-0000-0000-0000-000000000015"
	crCustomerFlagNoOpTestID             = "37777777-0000-0000-0000-000000000016"
	crCustomerFlagIndependentTestID      = "37777777-0000-0000-0000-000000000017"
)

// seedApprovalUserForDecisionTest inserts one "user" row per given id --
// approval_stage_approver.approver_user_id's FK needs a real one -- as its
// own fresh rows (not relying on any of the local docker-compose stack's own
// incidental seed users) so this test only depends on migrations having run,
// the same assumption every other integration test in this package makes.
func seedApprovalUserForDecisionTest(t *testing.T, pool *pgxpool.Pool, userIDs ...string) {
	t.Helper()
	ctx := context.Background()
	if len(userIDs) == 0 {
		userIDs = []string{changeRequestApprovalApproverUserID}
	}

	for i, userID := range userIDs {
		id := userID
		cleanup := func() {
			_, _ = pool.Exec(ctx, `DELETE FROM "user" WHERE id = $1`, id)
		}
		cleanup()
		t.Cleanup(cleanup)

		email := fmt.Sprintf("cr-approval-test-%d@example.com", i+1)
		if _, err := pool.Exec(ctx,
			`INSERT INTO "user" (id, created_on, updated_on, created_by, updated_by, user_name, name, first_name, last_name, email, is_active, is_system_user)
			 VALUES ($1, now(), now(), 'cr-approval-test', 'cr-approval-test', $2, 'CR Approval Test', 'CR', 'Approval Test', $2, true, false)`,
			id, email); err != nil {
			t.Fatalf("seed approver user %s: %v", id, err)
		}
	}
}

// seedChangeRequestForApprovalTest inserts a minimal work_item/change_request
// pair in the given state -- enough for PatchChangeRequest's own read (via
// GetChangeRequestByID at the end of a successful patch) to resolve, since
// every other join in changeRequestFromJoins is a LEFT JOIN.
func seedChangeRequestForApprovalTest(t *testing.T, pool *repository.Scoped, state string) {
	t.Helper()
	// work_item and change_request are RLS-protected; seed/cleanup as internal.
	ctx := repository.WithSystemIdentity(context.Background())

	cleanup := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM work_item WHERE id = $1`, changeRequestApprovalTestID)
	}
	cleanup()
	t.Cleanup(cleanup)

	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed (%.60s): %v", sql, err)
		}
	}

	mustExec(`INSERT INTO work_item (id, created_on, updated_on, created_by, updated_by, number, subject, type)
	          VALUES ($1, now(), now(), 'cr-approval-test-creator', 'cr-approval-test', 'CRAPPRV01', 'approval guard test', 'CHANGE_REQUEST')`,
		changeRequestApprovalTestID)
	mustExec(`INSERT INTO change_request (id, state) VALUES ($1, $2::change_request_state_enum)`,
		changeRequestApprovalTestID, state)
}

// TestChangeRequestIntegration_RequestApprovalIsBookkeepingOnly confirms the
// corrected behavior: {requestApproval: true} always sets
// change_request.approval = 'REQUESTED' and never touches state, regardless
// of the change request's current state. This replaces two prior tests
// (TestChangeRequestIntegration_RequestApprovalRejectsNonNewState/
// TestChangeRequestIntegration_RequestApprovalAdvancesNewToAssess) that
// asserted the earlier, now-confirmed-wrong model -- PatchChangeRequest used
// to also force state=ASSESS for a change request in New (and reject the
// request with a ConflictError for any other state) modeling New->Assess as
// an approval-gated ceremony. Checked against the real ServiceNow instance:
// New->Assess is a plain, ungated state change like every other transition,
// unrelated to approval at all -- the one real approval-gated transition is
// Assess->Authorize, handled entirely by DecideChangeRequestApproval, which
// this test does not touch.
func TestChangeRequestIntegration_RequestApprovalIsBookkeepingOnly(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	scoped := repository.NewScoped(pool)
	sys := repository.WithSystemIdentity(context.Background())
	repo := repository.NewChangeRequestRepository(scoped)

	for _, seedState := range []string{"NEW", "REVIEW", "CLOSED"} {
		t.Run(seedState, func(t *testing.T) {
			seedChangeRequestForApprovalTest(t, scoped, seedState)

			yes := true
			_, err := repo.PatchChangeRequest(sys, changeRequestApprovalTestID,
				domain.PatchChangeRequestRequest{RequestApproval: &yes}, "cr-approval-test")
			if err != nil {
				t.Fatalf("PatchChangeRequest(requestApproval=true) on a %s-state change request: %v", seedState, err)
			}

			var gotState, gotApproval *string
			if scanErr := scoped.QueryRow(sys,
				`SELECT state::TEXT, approval::TEXT FROM change_request WHERE id = $1`, changeRequestApprovalTestID).
				Scan(&gotState, &gotApproval); scanErr != nil {
				t.Fatalf("read back state/approval: %v", scanErr)
			}
			if gotState == nil || *gotState != seedState {
				t.Fatalf("state after requestApproval=true = %v, want unchanged %q", gotState, seedState)
			}
			if gotApproval == nil || *gotApproval != "REQUESTED" {
				t.Fatalf("approval after requestApproval=true = %v, want \"REQUESTED\"", gotApproval)
			}
		})
	}
}

// seedApprovalStageForDecisionTest inserts one approval_stage plus one or
// more approval_stage_approver rows for changeRequestApprovalTestID -- the
// same shared seed changeRequestForApprovalTest/seedChangeRequestForApprovalTest
// use, extended with an actual approval stage so DecideChangeRequestApproval
// has something real to act on. Each entry in approverUserIDs gets its own
// requested approver row; the returned stage id lets a test seed additional
// rows (e.g. a second approver already rejected) directly.
func seedApprovalStageForDecisionTest(t *testing.T, pool *repository.Scoped, approverUserIDs ...string) string {
	t.Helper()
	// approval_stage and approval_stage_approver are RLS-protected; seed/cleanup as internal.
	ctx := repository.WithSystemIdentity(context.Background())

	stageID := "36666666-0000-0000-0000-000000000002"
	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed (%.60s): %v", sql, err)
		}
	}
	mustExec(`INSERT INTO approval_stage (id, created_on, updated_on, created_by, updated_by, work_item_id, raw_status)
	          VALUES ($1, now(), now(), 'cr-approval-test', 'cr-approval-test', $2, 'requested')`,
		stageID, changeRequestApprovalTestID)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM approval_stage WHERE id = $1`, stageID)
	})

	for i, userID := range approverUserIDs {
		mustExec(`INSERT INTO approval_stage_approver (id, created_on, updated_on, created_by, updated_by, stage_id, work_item_id, approver_user_id, status)
		          VALUES ($1, now(), now(), 'cr-approval-test', 'cr-approval-test', $2, $3, $4, 'requested')`,
			fmt.Sprintf("36666666-0000-0000-0000-00000000%04d", i+10), stageID, changeRequestApprovalTestID, userID)
	}
	return stageID
}

// TestChangeRequestIntegration_DecideApprovalCascadesAssessToAuthorize is the
// regression guard for a real, reported gap: DecideChangeRequestApproval used
// to only flip the one approval_stage_approver row and never touch
// change_request.state at all -- confirmed live against a real approval on
// this data source. Approving the sole (or last-standing) approver on a
// change request's Assess-stage approval, while it's actually sitting in
// Assess, must now advance it to Authorize.
func TestChangeRequestIntegration_DecideApprovalCascadesAssessToAuthorize(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	scoped := repository.NewScoped(pool)
	sys := repository.WithSystemIdentity(context.Background())
	repo := repository.NewChangeRequestRepository(scoped)
	seedApprovalUserForDecisionTest(t, pool)
	seedChangeRequestForApprovalTest(t, scoped, "ASSESS")
	seedApprovalGroupMembers(t, scoped, crCABGroupID, crCABMemberUserID1)
	seedApprovalStageForDecisionTest(t, scoped, changeRequestApprovalApproverUserID)

	if _, err := repo.DecideChangeRequestApproval(sys, changeRequestApprovalTestID,
		changeRequestApprovalApproverUserID, "approved", "cr-approval-test"); err != nil {
		t.Fatalf("DecideChangeRequestApproval(approved): %v", err)
	}

	var gotState string
	if scanErr := scoped.QueryRow(sys,
		`SELECT state::TEXT FROM change_request WHERE id = $1`, changeRequestApprovalTestID).Scan(&gotState); scanErr != nil {
		t.Fatalf("read back state: %v", scanErr)
	}
	if gotState != "AUTHORIZE" {
		t.Fatalf("state after approval = %q, want \"AUTHORIZE\"", gotState)
	}

	// ...and the CAB Approval stage (its own group) is provisioned right away.
	var cabStages int
	if scanErr := scoped.QueryRow(sys,
		`SELECT COUNT(*) FROM approval_stage WHERE work_item_id = $1 AND checkpoint_label = 'CAB Approval' AND assignment_group_id = $2::uuid`,
		changeRequestApprovalTestID, crCABGroupID).Scan(&cabStages); scanErr != nil {
		t.Fatalf("count CAB stages: %v", scanErr)
	}
	if cabStages != 1 {
		t.Fatalf("CAB Approval stages after peer approval = %d, want 1", cabStages)
	}
}

// TestChangeRequestIntegration_DecideApprovalRejectionDoesNotCascade confirms
// a rejection never advances state, even though it resolves the stage (the
// same first-responder-wins quorum rule that lets a single approval resolve
// one also lets a single rejection resolve one) -- there is no confirmed
// target for a rejected Assess stage in this schema, so this must be a
// pure no-op on change_request.state.
func TestChangeRequestIntegration_DecideApprovalRejectionDoesNotCascade(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	scoped := repository.NewScoped(pool)
	sys := repository.WithSystemIdentity(context.Background())
	repo := repository.NewChangeRequestRepository(scoped)
	seedApprovalUserForDecisionTest(t, pool)
	seedChangeRequestForApprovalTest(t, scoped, "ASSESS")
	seedApprovalStageForDecisionTest(t, scoped, changeRequestApprovalApproverUserID)

	if _, err := repo.DecideChangeRequestApproval(sys, changeRequestApprovalTestID,
		changeRequestApprovalApproverUserID, "rejected", "cr-approval-test"); err != nil {
		t.Fatalf("DecideChangeRequestApproval(rejected): %v", err)
	}

	var gotState string
	if scanErr := scoped.QueryRow(sys,
		`SELECT state::TEXT FROM change_request WHERE id = $1`, changeRequestApprovalTestID).Scan(&gotState); scanErr != nil {
		t.Fatalf("read back state: %v", scanErr)
	}
	if gotState != "ASSESS" {
		t.Fatalf("state after rejection = %q, want unchanged \"ASSESS\"", gotState)
	}
}

// TestChangeRequestIntegration_DecideApprovalDoesNotCascadeOutsideAssess
// confirms the cascade is scoped exactly to Assess->Authorize: approving an
// approver on a change request that isn't currently in Assess (e.g. one
// already sitting in Authorize, mid its own separate approval stage) must
// leave state untouched -- this repository deliberately does not attempt
// Authorize's own outgoing cascade yet.
func TestChangeRequestIntegration_DecideApprovalDoesNotCascadeOutsideAssess(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	scoped := repository.NewScoped(pool)
	sys := repository.WithSystemIdentity(context.Background())
	repo := repository.NewChangeRequestRepository(scoped)
	seedApprovalUserForDecisionTest(t, pool)
	seedChangeRequestForApprovalTest(t, scoped, "AUTHORIZE")
	seedApprovalStageForDecisionTest(t, scoped, changeRequestApprovalApproverUserID)

	if _, err := repo.DecideChangeRequestApproval(sys, changeRequestApprovalTestID,
		changeRequestApprovalApproverUserID, "approved", "cr-approval-test"); err != nil {
		t.Fatalf("DecideChangeRequestApproval(approved): %v", err)
	}

	var gotState string
	if scanErr := scoped.QueryRow(sys,
		`SELECT state::TEXT FROM change_request WHERE id = $1`, changeRequestApprovalTestID).Scan(&gotState); scanErr != nil {
		t.Fatalf("read back state: %v", scanErr)
	}
	if gotState != "AUTHORIZE" {
		t.Fatalf("state after approval outside Assess = %q, want unchanged \"AUTHORIZE\"", gotState)
	}
}

// TestChangeRequestIntegration_DecideApprovalCancelsSiblingApprovers is the
// regression guard for the other real gap in the original cascade fix:
// resolving a stage used to leave every other still-pending approver on it
// sitting at Requested forever, with no visible way to tell "this group has
// already been decided" apart from "nobody has looked at this yet". Real
// ServiceNow does not do this (confirmed live against a genuine
// multi-approver group): once enough approvers respond to resolve the group,
// every other pending approver on it is moved to Cancelled. This test seeds
// three approvers on one stage and confirms that deciding as just one of
// them resolves the stage AND cancels the other two -- not just the acted-on
// row.
func TestChangeRequestIntegration_DecideApprovalCancelsSiblingApprovers(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	scoped := repository.NewScoped(pool)
	sys := repository.WithSystemIdentity(context.Background())
	repo := repository.NewChangeRequestRepository(scoped)
	seedApprovalUserForDecisionTest(t, pool,
		changeRequestApprovalApproverUserID, changeRequestApprovalApproverUserID2, changeRequestApprovalApproverUserID3)
	seedChangeRequestForApprovalTest(t, scoped, "ASSESS")
	seedApprovalGroupMembers(t, scoped, crCABGroupID, crCABMemberUserID1)
	stageID := seedApprovalStageForDecisionTest(t, scoped,
		changeRequestApprovalApproverUserID, changeRequestApprovalApproverUserID2, changeRequestApprovalApproverUserID3)

	if _, err := repo.DecideChangeRequestApproval(sys, changeRequestApprovalTestID,
		changeRequestApprovalApproverUserID, "approved", "cr-approval-test"); err != nil {
		t.Fatalf("DecideChangeRequestApproval(approved): %v", err)
	}

	rows, err := scoped.Query(sys,
		`SELECT approver_user_id, status FROM approval_stage_approver WHERE stage_id = $1`, stageID)
	if err != nil {
		t.Fatalf("read back approver statuses: %v", err)
	}
	defer rows.Close()

	statusByApprover := map[string]string{}
	for rows.Next() {
		var approverID, status string
		if err := rows.Scan(&approverID, &status); err != nil {
			t.Fatalf("scan approver row: %v", err)
		}
		statusByApprover[approverID] = status
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate approver rows: %v", err)
	}

	if got := statusByApprover[changeRequestApprovalApproverUserID]; got != "approved" {
		t.Errorf("acted-on approver status = %q, want \"approved\"", got)
	}
	if got := statusByApprover[changeRequestApprovalApproverUserID2]; got != "cancelled" {
		t.Errorf("sibling approver 2 status = %q, want \"cancelled\" (not left at \"requested\")", got)
	}
	if got := statusByApprover[changeRequestApprovalApproverUserID3]; got != "cancelled" {
		t.Errorf("sibling approver 3 status = %q, want \"cancelled\" (not left at \"requested\")", got)
	}

	var gotState string
	if scanErr := scoped.QueryRow(sys,
		`SELECT state::TEXT FROM change_request WHERE id = $1`, changeRequestApprovalTestID).Scan(&gotState); scanErr != nil {
		t.Fatalf("read back change request state: %v", scanErr)
	}
	if gotState != "AUTHORIZE" {
		t.Fatalf("state after multi-approver approval = %q, want \"AUTHORIZE\"", gotState)
	}
}

// seedPriorApprovalStagesForDecisionTest inserts count empty, approver-less
// approval_stage rows for changeRequestApprovalTestID, each created further
// in the past than the last (1 hour per position), so that a stage created
// afterward by seedApprovalStageForDecisionTest (which always inserts at
// "now") sorts after all of them -- pushing that real stage to ordinal
// position `count`, i.e. the same "earlier stage exists" shape
// DecideChangeRequestApproval's own isAssessStage check keys off of. Used
// by the rejection-sibling-cancellation tests below to prove that behavior
// isn't scoped to the Assess position the way the state cascade is. No
// explicit cleanup: seedChangeRequestForApprovalTest's own
// "DELETE FROM work_item" cascades through approval_stage's FK the same way
// it already does for the one seedApprovalStageForDecisionTest itself
// creates.
func seedPriorApprovalStagesForDecisionTest(t *testing.T, pool *repository.Scoped, count int) {
	t.Helper()
	ctx := repository.WithSystemIdentity(context.Background())
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("36666666-0000-0000-0000-0000000040%02d", i)
		if _, err := pool.Exec(ctx,
			`INSERT INTO approval_stage (id, created_on, updated_on, created_by, updated_by, work_item_id, raw_status)
			 VALUES ($1, now() - (INTERVAL '1 hour' * $2::int), now(), 'cr-approval-test', 'cr-approval-test', $3, 'requested')`,
			id, count-i, changeRequestApprovalTestID); err != nil {
			t.Fatalf("seed prior approval_stage %d: %v", i, err)
		}
	}
}

// TestChangeRequestIntegration_DecideRejectionCancelsSiblingApprovers is the
// rejection-side counterpart of
// TestChangeRequestIntegration_DecideApprovalCancelsSiblingApprovers above:
// a rejection now resolves a stage exactly as decisively as an approval
// does, so every other still-Requested sibling approver on the same stage
// must be moved to Cancelled too -- not left sitting at Requested forever,
// which is exactly the gap this fix closes. change_request.state must stay
// completely untouched regardless (see
// TestChangeRequestIntegration_DecideApprovalRejectionDoesNotCascade's own
// doc comment -- a rejection never cascades state, forward or backward).
func TestChangeRequestIntegration_DecideRejectionCancelsSiblingApprovers(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	scoped := repository.NewScoped(pool)
	sys := repository.WithSystemIdentity(context.Background())
	repo := repository.NewChangeRequestRepository(scoped)
	seedApprovalUserForDecisionTest(t, pool,
		changeRequestApprovalApproverUserID, changeRequestApprovalApproverUserID2, changeRequestApprovalApproverUserID3)
	seedChangeRequestForApprovalTest(t, scoped, "ASSESS")
	stageID := seedApprovalStageForDecisionTest(t, scoped,
		changeRequestApprovalApproverUserID, changeRequestApprovalApproverUserID2, changeRequestApprovalApproverUserID3)

	if _, err := repo.DecideChangeRequestApproval(sys, changeRequestApprovalTestID,
		changeRequestApprovalApproverUserID, "rejected", "cr-approval-test"); err != nil {
		t.Fatalf("DecideChangeRequestApproval(rejected): %v", err)
	}

	rows, err := scoped.Query(sys,
		`SELECT approver_user_id, status FROM approval_stage_approver WHERE stage_id = $1`, stageID)
	if err != nil {
		t.Fatalf("read back approver statuses: %v", err)
	}
	defer rows.Close()

	statusByApprover := map[string]string{}
	for rows.Next() {
		var approverID, status string
		if err := rows.Scan(&approverID, &status); err != nil {
			t.Fatalf("scan approver row: %v", err)
		}
		statusByApprover[approverID] = status
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate approver rows: %v", err)
	}

	if got := statusByApprover[changeRequestApprovalApproverUserID]; got != "rejected" {
		t.Errorf("acted-on approver status = %q, want \"rejected\"", got)
	}
	if got := statusByApprover[changeRequestApprovalApproverUserID2]; got != "cancelled" {
		t.Errorf("sibling approver 2 status = %q, want \"cancelled\" (not left at \"requested\")", got)
	}
	if got := statusByApprover[changeRequestApprovalApproverUserID3]; got != "cancelled" {
		t.Errorf("sibling approver 3 status = %q, want \"cancelled\" (not left at \"requested\")", got)
	}

	var gotState string
	if scanErr := scoped.QueryRow(sys,
		`SELECT state::TEXT FROM change_request WHERE id = $1`, changeRequestApprovalTestID).Scan(&gotState); scanErr != nil {
		t.Fatalf("read back change request state: %v", scanErr)
	}
	if gotState != "ASSESS" {
		t.Fatalf("state after multi-approver rejection = %q, want unchanged \"ASSESS\" (a rejection never cascades state)", gotState)
	}
}

// TestChangeRequestIntegration_DecideRejectionCancelsSiblingApproversAtEveryCheckpoint
// confirms sibling-cancellation-on-rejection is NOT scoped to the Assess
// position the way the state cascade is -- unlike
// TestChangeRequestIntegration_DecideApprovalDoesNotCascadeOutsideAssess
// (which only guards the state write), this proves the cancellation itself
// fires identically at Authorize and Review. seedPriorApprovalStagesForDecisionTest
// pushes the real stage to the given ordinal the same way a genuine
// Assess-then-Authorize(-then-Review) history would.
func TestChangeRequestIntegration_DecideRejectionCancelsSiblingApproversAtEveryCheckpoint(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}

	for _, tc := range []struct {
		name        string
		state       string
		priorStages int
	}{
		{name: "Authorize", state: "AUTHORIZE", priorStages: 1},
		{name: "Review", state: "REVIEW", priorStages: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool, err := pgxpool.New(context.Background(), dsn)
			if err != nil {
				t.Fatalf("connect: %v", err)
			}
			t.Cleanup(pool.Close)

			scoped := repository.NewScoped(pool)
			sys := repository.WithSystemIdentity(context.Background())
			repo := repository.NewChangeRequestRepository(scoped)
			seedApprovalUserForDecisionTest(t, pool,
				changeRequestApprovalApproverUserID, changeRequestApprovalApproverUserID2, changeRequestApprovalApproverUserID3)
			seedChangeRequestForApprovalTest(t, scoped, tc.state)
			seedPriorApprovalStagesForDecisionTest(t, scoped, tc.priorStages)
			stageID := seedApprovalStageForDecisionTest(t, scoped,
				changeRequestApprovalApproverUserID, changeRequestApprovalApproverUserID2, changeRequestApprovalApproverUserID3)

			if _, err := repo.DecideChangeRequestApproval(sys, changeRequestApprovalTestID,
				changeRequestApprovalApproverUserID, "rejected", "cr-approval-test"); err != nil {
				t.Fatalf("DecideChangeRequestApproval(rejected): %v", err)
			}

			rows, err := scoped.Query(sys,
				`SELECT approver_user_id, status FROM approval_stage_approver WHERE stage_id = $1`, stageID)
			if err != nil {
				t.Fatalf("read back approver statuses: %v", err)
			}
			defer rows.Close()

			statusByApprover := map[string]string{}
			for rows.Next() {
				var approverID, status string
				if err := rows.Scan(&approverID, &status); err != nil {
					t.Fatalf("scan approver row: %v", err)
				}
				statusByApprover[approverID] = status
			}
			if err := rows.Err(); err != nil {
				t.Fatalf("iterate approver rows: %v", err)
			}

			if got := statusByApprover[changeRequestApprovalApproverUserID]; got != "rejected" {
				t.Errorf("acted-on approver status = %q, want \"rejected\"", got)
			}
			if got := statusByApprover[changeRequestApprovalApproverUserID2]; got != "cancelled" {
				t.Errorf("sibling approver 2 status at %s = %q, want \"cancelled\"", tc.name, got)
			}
			if got := statusByApprover[changeRequestApprovalApproverUserID3]; got != "cancelled" {
				t.Errorf("sibling approver 3 status at %s = %q, want \"cancelled\"", tc.name, got)
			}

			var gotState string
			if scanErr := scoped.QueryRow(sys,
				`SELECT state::TEXT FROM change_request WHERE id = $1`, changeRequestApprovalTestID).Scan(&gotState); scanErr != nil {
				t.Fatalf("read back change request state: %v", scanErr)
			}
			if gotState != tc.state {
				t.Fatalf("state after %s rejection = %q, want unchanged %q", tc.name, gotState, tc.state)
			}
		})
	}
}

// TestChangeRequestIntegration_DecideRejectionDoesNotDisturbAlreadyApprovedStage
// is the edge case this fix's own design decision needed: a stage that was
// already resolved by an APPROVAL -- seeded here directly, as a stand-in
// for a ServiceNow-synced stage or one seeded before this fix shipped,
// since this repository's own serialized flow can no longer produce this
// shape itself (see DecideChangeRequestApproval's "rejected" branch's own
// doc comment on hasApproval) -- must not have its other still-Requested
// approvers disturbed by a later, late rejection on a different approver of
// the same stage. A destructive retroactive cancellation here would be
// exactly wrong: the earlier approval already had every right to leave
// those rows alone.
func TestChangeRequestIntegration_DecideRejectionDoesNotDisturbAlreadyApprovedStage(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	scoped := repository.NewScoped(pool)
	sys := repository.WithSystemIdentity(context.Background())
	repo := repository.NewChangeRequestRepository(scoped)
	seedApprovalUserForDecisionTest(t, pool,
		changeRequestApprovalApproverUserID, changeRequestApprovalApproverUserID2, changeRequestApprovalApproverUserID3)
	seedChangeRequestForApprovalTest(t, scoped, "ASSESS")
	stageID := seedApprovalStageForDecisionTest(t, scoped,
		changeRequestApprovalApproverUserID, changeRequestApprovalApproverUserID2, changeRequestApprovalApproverUserID3)

	// Simulate a stage some other path already resolved via approval
	// without running this method's own sibling-cancellation -- directly
	// flipping approver 2's row, bypassing DecideChangeRequestApproval
	// entirely, the same way a ServiceNow sync (or pre-fix data) could have
	// left it. Approver 3 is deliberately left "requested" to prove it
	// survives untouched below.
	if _, err := scoped.Exec(sys,
		`UPDATE approval_stage_approver SET status = 'approved' WHERE stage_id = $1 AND approver_user_id = $2`,
		stageID, changeRequestApprovalApproverUserID2); err != nil {
		t.Fatalf("seed pre-existing approval: %v", err)
	}

	if _, err := repo.DecideChangeRequestApproval(sys, changeRequestApprovalTestID,
		changeRequestApprovalApproverUserID3, "rejected", "cr-approval-test"); err != nil {
		t.Fatalf("DecideChangeRequestApproval(rejected): %v", err)
	}

	rows, err := scoped.Query(sys,
		`SELECT approver_user_id, status FROM approval_stage_approver WHERE stage_id = $1`, stageID)
	if err != nil {
		t.Fatalf("read back approver statuses: %v", err)
	}
	defer rows.Close()

	statusByApprover := map[string]string{}
	for rows.Next() {
		var approverID, status string
		if err := rows.Scan(&approverID, &status); err != nil {
			t.Fatalf("scan approver row: %v", err)
		}
		statusByApprover[approverID] = status
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate approver rows: %v", err)
	}

	if got := statusByApprover[changeRequestApprovalApproverUserID3]; got != "rejected" {
		t.Errorf("acted-on approver status = %q, want \"rejected\"", got)
	}
	if got := statusByApprover[changeRequestApprovalApproverUserID2]; got != "approved" {
		t.Errorf("pre-existing approved approver status = %q, want unchanged \"approved\"", got)
	}
	// The real assertion of this test: approver 1 was never decided by
	// anyone and is NOT a sibling of the rejection's own resolution (the
	// stage was already resolved, by the approval, before the rejection
	// ever ran) -- the hasApproval guard must have skipped cancellation
	// entirely, leaving it exactly as it was.
	if got := statusByApprover[changeRequestApprovalApproverUserID]; got != "requested" {
		t.Errorf("untouched sibling approver status = %q, want unchanged \"requested\" (an already-approved stage must not be disturbed by a later rejection)", got)
	}

	var gotState string
	if scanErr := scoped.QueryRow(sys,
		`SELECT state::TEXT FROM change_request WHERE id = $1`, changeRequestApprovalTestID).Scan(&gotState); scanErr != nil {
		t.Fatalf("read back change request state: %v", scanErr)
	}
	if gotState != "ASSESS" {
		t.Fatalf("state after rejection on an already-approved stage = %q, want unchanged \"ASSESS\"", gotState)
	}
}

// seedChangeRequestForAssignedTeamTest inserts a minimal work_item/
// change_request pair for the AssignedTeamID tests below, following the same
// shape as seedChangeRequestForApprovalTest but under its own id so the two
// test groups can never collide.
func seedChangeRequestForAssignedTeamTest(t *testing.T, pool *repository.Scoped) {
	t.Helper()
	// work_item and change_request are RLS-protected; seed/cleanup as internal.
	ctx := repository.WithSystemIdentity(context.Background())

	cleanup := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM work_item WHERE id = $1`, changeRequestAssignedTeamTestID)
	}
	cleanup()
	t.Cleanup(cleanup)

	if _, err := pool.Exec(ctx,
		`INSERT INTO work_item (id, created_on, updated_on, created_by, updated_by, number, subject, type)
		 VALUES ($1, now(), now(), 'cr-assigned-team-test', 'cr-assigned-team-test', 'CRTEAM001', 'assigned team patch test', 'CHANGE_REQUEST')`,
		changeRequestAssignedTeamTestID); err != nil {
		t.Fatalf("seed work_item: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO change_request (id, state) VALUES ($1, 'NEW'::change_request_state_enum)`,
		changeRequestAssignedTeamTestID); err != nil {
		t.Fatalf("seed change_request: %v", err)
	}
}

// TestChangeRequestIntegration_PatchAssignedTeamID is the regression guard for
// the bug this change fixes: PatchChangeRequestRequest.AssignedTeamID used to
// be read but never written anywhere in PatchChangeRequest's own UPDATE,
// unlike the adjacent AssignedEngineerID handling. Patching it to a real
// "group" id must round-trip through work_item.assignment_group_id and come
// back as ChangeRequest.AssignedTeam on a subsequent read.
func TestChangeRequestIntegration_PatchAssignedTeamID(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	scoped := repository.NewScoped(pool)
	repo := repository.NewChangeRequestRepository(scoped)
	seedChangeRequestForAssignedTeamTest(t, scoped)
	sys := repository.WithSystemIdentity(context.Background())

	teamID := seededGroupID
	updated, err := repo.PatchChangeRequest(sys, changeRequestAssignedTeamTestID,
		domain.PatchChangeRequestRequest{AssignedTeamID: &teamID}, "cr-assigned-team-test")
	if err != nil {
		t.Fatalf("PatchChangeRequest(assignedTeamId=%s): %v", teamID, err)
	}
	if updated.AssignedTeam == nil || updated.AssignedTeam.ID != teamID {
		t.Fatalf("PatchChangeRequest response AssignedTeam = %+v, want ID %q", updated.AssignedTeam, teamID)
	}

	// Read back directly, independent of the repository's own response, to
	// confirm the column itself -- not just the in-memory return value --
	// actually changed.
	var gotAssignmentGroupID string
	if scanErr := scoped.QueryRow(sys,
		`SELECT assignment_group_id::TEXT FROM work_item WHERE id = $1`, changeRequestAssignedTeamTestID).
		Scan(&gotAssignmentGroupID); scanErr != nil {
		t.Fatalf("read back assignment_group_id: %v", scanErr)
	}
	if gotAssignmentGroupID != teamID {
		t.Fatalf("work_item.assignment_group_id = %q, want %q", gotAssignmentGroupID, teamID)
	}

	// GetChangeRequestByID, the way a caller would actually re-read the
	// change request, must agree too.
	fetched, err := repo.GetChangeRequestByID(sys, changeRequestAssignedTeamTestID)
	if err != nil {
		t.Fatalf("GetChangeRequestByID: %v", err)
	}
	if fetched.AssignedTeam == nil || fetched.AssignedTeam.ID != teamID {
		t.Fatalf("GetChangeRequestByID AssignedTeam = %+v, want ID %q", fetched.AssignedTeam, teamID)
	}
}

// TestChangeRequestIntegration_PatchAssignedTeamIDUnknownTeamIsValidationError
// confirms an unknown team id produces a clean ValidationError (the
// changeRequestPatchFKField mapping's "assignedTeamId" entry), not a raw
// Postgres foreign-key-violation error surfaced to the caller.
func TestChangeRequestIntegration_PatchAssignedTeamIDUnknownTeamIsValidationError(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	scoped := repository.NewScoped(pool)
	repo := repository.NewChangeRequestRepository(scoped)
	seedChangeRequestForAssignedTeamTest(t, scoped)
	sys := repository.WithSystemIdentity(context.Background())

	badTeamID := unknownGroupID
	_, err = repo.PatchChangeRequest(sys, changeRequestAssignedTeamTestID,
		domain.PatchChangeRequestRequest{AssignedTeamID: &badTeamID}, "cr-assigned-team-test")
	if err == nil {
		t.Fatal("PatchChangeRequest(assignedTeamId=<unknown>) succeeded, want a ValidationError")
	}
	var valErr *apierror.ValidationError
	if !errors.As(err, &valErr) {
		t.Fatalf("PatchChangeRequest(assignedTeamId=<unknown>) error = %v (%T), want *apierror.ValidationError", err, err)
	}
	if valErr.Msg == "" {
		t.Fatal("ValidationError.Msg is empty")
	}

	// The column must be left untouched by the rolled-back transaction.
	var gotAssignmentGroupID *string
	if scanErr := scoped.QueryRow(sys,
		`SELECT assignment_group_id::TEXT FROM work_item WHERE id = $1`, changeRequestAssignedTeamTestID).
		Scan(&gotAssignmentGroupID); scanErr != nil {
		t.Fatalf("read back assignment_group_id: %v", scanErr)
	}
	if gotAssignmentGroupID != nil {
		t.Fatalf("work_item.assignment_group_id = %v after a failed patch, want unchanged NULL", *gotAssignmentGroupID)
	}
}

// seedChangeRequestForAssessGateTest inserts a minimal NEW-state work_item/
// change_request pair with no assigned team -- the starting point for every
// test below.
func seedChangeRequestForAssessGateTest(t *testing.T, pool *repository.Scoped) {
	t.Helper()
	// work_item and change_request are RLS-protected; seed/cleanup as internal.
	ctx := repository.WithSystemIdentity(context.Background())

	cleanup := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM work_item WHERE id = $1`, changeRequestAssessGateTestID)
	}
	cleanup()
	t.Cleanup(cleanup)

	if _, err := pool.Exec(ctx,
		`INSERT INTO work_item (id, created_on, updated_on, created_by, updated_by, number, subject, type)
		 VALUES ($1, now(), now(), 'cr-assess-gate-test-creator', 'cr-assess-gate-test', 'CRASSESS01', 'assess gate test', 'CHANGE_REQUEST')`,
		changeRequestAssessGateTestID); err != nil {
		t.Fatalf("seed work_item: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO change_request (id, state) VALUES ($1, 'NEW'::change_request_state_enum)`,
		changeRequestAssessGateTestID); err != nil {
		t.Fatalf("seed change_request: %v", err)
	}
	// A Normal change cannot be sent for approval unless the CAB Approval
	// group has someone to give the second approval.
	seedApprovalGroupMembers(t, pool, crCABGroupID, crCABMemberUserID1, crCABMemberUserID2)
}

// seedAssessGateGroup inserts changeRequestAssessGateGroupID's own "group"
// row -- a dedicated fixture for the tests below, deliberately not
// seededGroupID (see that constant's own doc comment on the collision this
// avoids).
func seedAssessGateGroup(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	cleanup := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM "group" WHERE id = $1`, changeRequestAssessGateGroupID)
	}
	cleanup()
	t.Cleanup(cleanup)

	if _, err := pool.Exec(ctx,
		`INSERT INTO "group" (id, created_on, updated_on, created_by, updated_by, name)
		 VALUES ($1, now(), now(), 'cr-assess-gate-test', 'cr-assess-gate-test', 'Assess Gate Test Group')`,
		changeRequestAssessGateGroupID); err != nil {
		t.Fatalf("seed group: %v", err)
	}
}

// seedTeamMembersForAssessGateTest inserts one "user" row and one
// team_member row (keyed by group_id, not team_id -- see
// PatchChangeRequest's own doc comment on why) per given user id, so they
// resolve as members of changeRequestAssessGateGroupID for the
// auto-provisioning tests below. Callers must seed that group row first
// (seedAssessGateGroup) -- group_id's FK requires it to already exist.
func seedTeamMembersForAssessGateTest(t *testing.T, pool *pgxpool.Pool, userIDs ...string) {
	t.Helper()
	ctx := context.Background()

	for i, userID := range userIDs {
		id := userID
		userCleanup := func() {
			_, _ = pool.Exec(ctx, `DELETE FROM "user" WHERE id = $1`, id)
		}
		userCleanup()
		t.Cleanup(userCleanup)

		email := fmt.Sprintf("cr-assess-gate-member-%d@example.com", i+1)
		if _, err := pool.Exec(ctx,
			`INSERT INTO "user" (id, created_on, updated_on, created_by, updated_by, user_name, name, first_name, last_name, email, is_active, is_system_user)
			 VALUES ($1, now(), now(), 'cr-assess-gate-test', 'cr-assess-gate-test', $2, 'Assess Gate Member', 'Assess', 'Gate Member', $2, true, false)`,
			id, email); err != nil {
			t.Fatalf("seed team member user %s: %v", id, err)
		}

		memberCleanup := func() {
			_, _ = pool.Exec(ctx, `DELETE FROM team_member WHERE user_id = $1`, id)
		}
		memberCleanup()
		t.Cleanup(memberCleanup)

		// team_member.team_id is NOT NULL, but no test here exercises the
		// internal team registry -- the one seeded team row
		// (scripts/csm-compose/seed-entity-service.sql's "Example Corp ABT",
		// id 901) satisfies the column's NOT NULL constraint without
		// implying anything about this test's own group_id-keyed membership
		// (changeRequestAssessGateGroupID, a wholly separate "group" row).
		if _, err := pool.Exec(ctx,
			`INSERT INTO team_member (id, created_on, updated_on, created_by, updated_by, team_id, user_id, group_id)
			 VALUES (gen_random_uuid(), now(), now(), 'cr-assess-gate-test', 'cr-assess-gate-test', $1::uuid, $2, $3::uuid)`,
			seededGroupID, id, changeRequestAssessGateGroupID); err != nil {
			t.Fatalf("seed team_member for user %s: %v", id, err)
		}
	}
}

// TestChangeRequestIntegration_PatchAssessRequiresAssignedTeam is the
// regression guard for the compulsory gate: PatchChangeRequest must reject a
// {state: "assess"} patch with a clean ValidationError when the change
// request has no assigned team and the request itself doesn't supply one --
// never a silent state change with nothing to assign the Assess stage to.
func TestChangeRequestIntegration_PatchAssessRequiresAssignedTeam(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	scoped := repository.NewScoped(pool)
	sys := repository.WithSystemIdentity(context.Background())
	repo := repository.NewChangeRequestRepository(scoped)
	seedChangeRequestForAssessGateTest(t, scoped)

	assess := domain.ChangeRequestStateAssess
	_, err = repo.PatchChangeRequest(sys, changeRequestAssessGateTestID,
		domain.PatchChangeRequestRequest{State: &assess}, "cr-assess-gate-test")
	if err == nil {
		t.Fatal("PatchChangeRequest(state=assess) with no assigned team succeeded, want a ValidationError")
	}
	var valErr *apierror.ValidationError
	if !errors.As(err, &valErr) {
		t.Fatalf("PatchChangeRequest(state=assess) with no assigned team error = %v (%T), want *apierror.ValidationError", err, err)
	}

	var gotState string
	if scanErr := scoped.QueryRow(sys,
		`SELECT state::TEXT FROM change_request WHERE id = $1`, changeRequestAssessGateTestID).Scan(&gotState); scanErr != nil {
		t.Fatalf("read back state: %v", scanErr)
	}
	if gotState != "NEW" {
		t.Fatalf("state after a rejected assess patch = %q, want unchanged \"NEW\"", gotState)
	}
}

// TestChangeRequestIntegration_PatchAssessWithTeamAlreadyOnRecordSucceeds
// confirms the compulsory gate accepts a team set by an earlier, separate
// PATCH (the real flow: the Edit dialog saves assignedTeamId first, then
// "Move to Assess" is sent as its own request with no assignedTeamId at
// all) -- not just a team supplied in the very same request.
func TestChangeRequestIntegration_PatchAssessWithTeamAlreadyOnRecordSucceeds(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	scoped := repository.NewScoped(pool)
	sys := repository.WithSystemIdentity(context.Background())
	repo := repository.NewChangeRequestRepository(scoped)
	seedChangeRequestForAssessGateTest(t, scoped)
	seedAssessGateGroup(t, pool)
	// A non-empty group: the Assess provisioning path now rejects an empty
	// one outright (see TestChangeRequestIntegration_PatchAssessRejectsEmptyGroup),
	// so this test -- about the team-already-on-record path specifically --
	// needs a real member for its own second PatchChangeRequest call to
	// reach "succeeds" at all.
	seedTeamMembersForAssessGateTest(t, pool, changeRequestAssessGateMemberUserID)
	t.Cleanup(func() {
		_, _ = scoped.Exec(sys, `DELETE FROM approval_stage WHERE work_item_id = $1`, changeRequestAssessGateTestID)
	})

	teamID := changeRequestAssessGateGroupID
	if _, err := repo.PatchChangeRequest(sys, changeRequestAssessGateTestID,
		domain.PatchChangeRequestRequest{AssignedTeamID: &teamID}, "cr-assess-gate-test"); err != nil {
		t.Fatalf("PatchChangeRequest(assignedTeamId=%s): %v", teamID, err)
	}

	assess := domain.ChangeRequestStateAssess
	updated, err := repo.PatchChangeRequest(sys, changeRequestAssessGateTestID,
		domain.PatchChangeRequestRequest{State: &assess}, "cr-assess-gate-test")
	if err != nil {
		t.Fatalf("PatchChangeRequest(state=assess) with a team already on the record: %v", err)
	}
	if updated.State == nil || *updated.State != string(assess) {
		t.Fatalf("state after patch = %v, want %q", updated.State, assess)
	}
}

// TestChangeRequestIntegration_PatchAssessProvisionsApproversFromGroupMembers
// is the regression guard for the auto-provisioning feature: the moment a
// change request enters Assess, every team_member row keyed to the assigned
// team's group_id must become a Requested approval_stage_approver on a
// freshly created Assess-stage approval_stage -- not an empty Approvals tab
// with nobody to approve it.
func TestChangeRequestIntegration_PatchAssessProvisionsApproversFromGroupMembers(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	scoped := repository.NewScoped(pool)
	sys := repository.WithSystemIdentity(context.Background())
	repo := repository.NewChangeRequestRepository(scoped)
	seedChangeRequestForAssessGateTest(t, scoped)
	seedAssessGateGroup(t, pool)
	seedTeamMembersForAssessGateTest(t, pool, changeRequestAssessGateMemberUserID, changeRequestAssessGateMemberUserID2)
	t.Cleanup(func() {
		_, _ = scoped.Exec(sys, `DELETE FROM approval_stage WHERE work_item_id = $1`, changeRequestAssessGateTestID)
	})

	teamID := changeRequestAssessGateGroupID
	assess := domain.ChangeRequestStateAssess
	if _, err := repo.PatchChangeRequest(sys, changeRequestAssessGateTestID,
		domain.PatchChangeRequestRequest{AssignedTeamID: &teamID, State: &assess}, "cr-assess-gate-test"); err != nil {
		t.Fatalf("PatchChangeRequest(assignedTeamId=%s, state=assess): %v", teamID, err)
	}

	var stageID, stageGroupID string
	if scanErr := scoped.QueryRow(sys,
		`SELECT id, assignment_group_id::TEXT FROM approval_stage WHERE work_item_id = $1`, changeRequestAssessGateTestID).
		Scan(&stageID, &stageGroupID); scanErr != nil {
		t.Fatalf("read back approval_stage: %v", scanErr)
	}
	if stageGroupID != teamID {
		t.Fatalf("approval_stage.assignment_group_id = %q, want %q", stageGroupID, teamID)
	}

	rows, err := scoped.Query(sys,
		`SELECT approver_user_id::TEXT, status FROM approval_stage_approver WHERE stage_id = $1 ORDER BY approver_user_id`, stageID)
	if err != nil {
		t.Fatalf("query approval_stage_approver: %v", err)
	}
	defer rows.Close()
	gotApprovers := map[string]string{}
	for rows.Next() {
		var uid, status string
		if err := rows.Scan(&uid, &status); err != nil {
			t.Fatalf("scan approval_stage_approver: %v", err)
		}
		gotApprovers[uid] = status
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("approval_stage_approver rows: %v", err)
	}

	want := map[string]string{
		changeRequestAssessGateMemberUserID:  "requested",
		changeRequestAssessGateMemberUserID2: "requested",
	}
	if len(gotApprovers) != len(want) {
		t.Fatalf("approval_stage_approver rows = %+v, want exactly %+v", gotApprovers, want)
	}
	for uid, wantStatus := range want {
		if gotApprovers[uid] != wantStatus {
			t.Fatalf("approver %s status = %q, want %q", uid, gotApprovers[uid], wantStatus)
		}
	}
}

// TestChangeRequestIntegration_PatchAssessDoesNotReprovisionWhenStageExists
// confirms a second {state: "assess"} patch against a change request that
// already has an approval_stage (e.g. a resend, or an unrelated field edit
// sent while already in Assess) never creates a duplicate stage or
// re-seeds approvers -- DecideChangeRequestApproval owns everything about
// an existing stage from the moment it's created.
func TestChangeRequestIntegration_PatchAssessDoesNotReprovisionWhenStageExists(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	scoped := repository.NewScoped(pool)
	sys := repository.WithSystemIdentity(context.Background())
	repo := repository.NewChangeRequestRepository(scoped)
	seedChangeRequestForAssessGateTest(t, scoped)
	seedAssessGateGroup(t, pool)
	seedTeamMembersForAssessGateTest(t, pool, changeRequestAssessGateMemberUserID, changeRequestAssessGateMemberUserID2)
	t.Cleanup(func() {
		_, _ = scoped.Exec(sys, `DELETE FROM approval_stage WHERE work_item_id = $1`, changeRequestAssessGateTestID)
	})

	teamID := changeRequestAssessGateGroupID
	assess := domain.ChangeRequestStateAssess
	if _, err := repo.PatchChangeRequest(sys, changeRequestAssessGateTestID,
		domain.PatchChangeRequestRequest{AssignedTeamID: &teamID, State: &assess}, "cr-assess-gate-test"); err != nil {
		t.Fatalf("PatchChangeRequest(assignedTeamId=%s, state=assess) #1: %v", teamID, err)
	}
	if _, err := repo.PatchChangeRequest(sys, changeRequestAssessGateTestID,
		domain.PatchChangeRequestRequest{State: &assess}, "cr-assess-gate-test"); err != nil {
		t.Fatalf("PatchChangeRequest(state=assess) #2 (resend): %v", err)
	}

	var stageCount int
	if scanErr := scoped.QueryRow(sys,
		`SELECT COUNT(*) FROM approval_stage WHERE work_item_id = $1`, changeRequestAssessGateTestID).Scan(&stageCount); scanErr != nil {
		t.Fatalf("count approval_stage: %v", scanErr)
	}
	if stageCount != 1 {
		t.Fatalf("approval_stage rows after two assess patches = %d, want exactly 1", stageCount)
	}

	var approverCount int
	if scanErr := scoped.QueryRow(sys,
		`SELECT COUNT(*) FROM approval_stage_approver asa JOIN approval_stage ast ON ast.id = asa.stage_id WHERE ast.work_item_id = $1`,
		changeRequestAssessGateTestID).Scan(&approverCount); scanErr != nil {
		t.Fatalf("count approval_stage_approver: %v", scanErr)
	}
	if approverCount != 2 {
		t.Fatalf("approval_stage_approver rows after two assess patches = %d, want exactly 2 (not re-seeded)", approverCount)
	}
}

// TestChangeRequestIntegration_PatchAssessRejectsEmptyGroup is the
// regression guard for a CodeRabbit-caught gap: the approval_stage used to
// be created before team_member was ever queried, so an assigned team with
// no members still committed an empty, un-approvable stage -- the change
// request would be stuck in Assess forever, since "no approval_stage exists
// yet" is exactly the condition that gates (re-)provisioning. A team with no
// members must instead reject the whole PATCH with a ValidationError and
// leave no approval_stage behind at all.
func TestChangeRequestIntegration_PatchAssessRejectsEmptyGroup(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	scoped := repository.NewScoped(pool)
	sys := repository.WithSystemIdentity(context.Background())
	repo := repository.NewChangeRequestRepository(scoped)
	seedChangeRequestForAssessGateTest(t, scoped)
	seedAssessGateGroup(t, pool)
	// Deliberately no seedTeamMembersForAssessGateTest call -- the group
	// exists (so assignedTeamId itself is valid) but has zero members.
	t.Cleanup(func() {
		_, _ = scoped.Exec(sys, `DELETE FROM approval_stage WHERE work_item_id = $1`, changeRequestAssessGateTestID)
	})

	teamID := changeRequestAssessGateGroupID
	assess := domain.ChangeRequestStateAssess
	_, err = repo.PatchChangeRequest(sys, changeRequestAssessGateTestID,
		domain.PatchChangeRequestRequest{AssignedTeamID: &teamID, State: &assess}, "cr-assess-gate-test")
	if err == nil {
		t.Fatal("PatchChangeRequest(state=assess) with an empty assigned team succeeded, want a ValidationError")
	}
	var valErr *apierror.ValidationError
	if !errors.As(err, &valErr) {
		t.Fatalf("PatchChangeRequest(state=assess) with an empty assigned team error = %v (%T), want *apierror.ValidationError", err, err)
	}

	var gotState string
	if scanErr := scoped.QueryRow(sys,
		`SELECT state::TEXT FROM change_request WHERE id = $1`, changeRequestAssessGateTestID).Scan(&gotState); scanErr != nil {
		t.Fatalf("read back state: %v", scanErr)
	}
	if gotState != "NEW" {
		t.Fatalf("state after a rejected assess patch = %q, want unchanged \"NEW\"", gotState)
	}

	var stageCount int
	if scanErr := scoped.QueryRow(sys,
		`SELECT COUNT(*) FROM approval_stage WHERE work_item_id = $1`, changeRequestAssessGateTestID).Scan(&stageCount); scanErr != nil {
		t.Fatalf("count approval_stage: %v", scanErr)
	}
	if stageCount != 0 {
		t.Fatalf("approval_stage rows after a rejected assess patch = %d, want 0 (no empty stage left behind)", stageCount)
	}
}

// TestChangeRequestIntegration_PatchAssessDeduplicatesGroupMembers is the
// regression guard for a second CodeRabbit catch: team_member has no unique
// constraint on (user_id, group_id), so a duplicated membership row must
// still provision exactly one requested approval_stage_approver per person,
// never two.
func TestChangeRequestIntegration_PatchAssessDeduplicatesGroupMembers(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	scoped := repository.NewScoped(pool)
	sys := repository.WithSystemIdentity(context.Background())
	repo := repository.NewChangeRequestRepository(scoped)
	seedChangeRequestForAssessGateTest(t, scoped)
	seedAssessGateGroup(t, pool)
	seedTeamMembersForAssessGateTest(t, pool, changeRequestAssessGateMemberUserID)
	t.Cleanup(func() {
		_, _ = scoped.Exec(sys, `DELETE FROM approval_stage WHERE work_item_id = $1`, changeRequestAssessGateTestID)
	})

	// A second team_member row for the SAME user against the SAME group --
	// seedTeamMembersForAssessGateTest's own cleanup (DELETE ... WHERE
	// user_id = $1) already covers this row too, since it shares the user id.
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO team_member (id, created_on, updated_on, created_by, updated_by, team_id, user_id, group_id)
		 VALUES (gen_random_uuid(), now(), now(), 'cr-assess-gate-test', 'cr-assess-gate-test', $1::uuid, $2, $3::uuid)`,
		seededGroupID, changeRequestAssessGateMemberUserID, changeRequestAssessGateGroupID); err != nil {
		t.Fatalf("seed duplicate team_member row: %v", err)
	}

	teamID := changeRequestAssessGateGroupID
	assess := domain.ChangeRequestStateAssess
	if _, err := repo.PatchChangeRequest(sys, changeRequestAssessGateTestID,
		domain.PatchChangeRequestRequest{AssignedTeamID: &teamID, State: &assess}, "cr-assess-gate-test"); err != nil {
		t.Fatalf("PatchChangeRequest(assignedTeamId=%s, state=assess): %v", teamID, err)
	}

	var approverCount int
	if scanErr := scoped.QueryRow(sys,
		`SELECT COUNT(*) FROM approval_stage_approver asa JOIN approval_stage ast ON ast.id = asa.stage_id WHERE ast.work_item_id = $1`,
		changeRequestAssessGateTestID).Scan(&approverCount); scanErr != nil {
		t.Fatalf("count approval_stage_approver: %v", scanErr)
	}
	if approverCount != 1 {
		t.Fatalf("approval_stage_approver rows for a user with a duplicated team_member row = %d, want exactly 1", approverCount)
	}
}

// setAssessGateRequestedBy stamps change_request.requested_by_user_id for
// changeRequestAssessGateTestID -- seedChangeRequestForAssessGateTest itself
// leaves this column NULL (no test before the requester-self-approval tests
// below cared who "requested" the change), so those tests set it explicitly
// after seeding. userID must already be a real "user" row (the column is a
// real FK) -- callers pass one of seedTeamMembersForAssessGateTest's own
// seeded ids.
func setAssessGateRequestedBy(t *testing.T, pool *repository.Scoped, userID string) {
	t.Helper()
	ctx := repository.WithSystemIdentity(context.Background())
	if _, err := pool.Exec(ctx,
		`UPDATE change_request SET requested_by_user_id = $1::uuid WHERE id = $2`,
		userID, changeRequestAssessGateTestID); err != nil {
		t.Fatalf("set change_request.requested_by_user_id: %v", err)
	}
}

// TestChangeRequestIntegration_PatchAssessProvisionsRequesterAsCancelled is
// the regression guard for ServiceNow's real, confirmed self-approval
// prevention: when the change request's own requested_by_user_id is also a
// member of the assigned team, that person's approval_stage_approver row
// must be provisioned already "cancelled" (mirroring the same state
// ServiceNow itself assigns at creation -- confirmed live against
// CHG0039122's own activity log, whose very first "Field changes" entry
// already shows State: Cancelled, not a later transition from Requested),
// while every other team member's row stays "requested" exactly as before.
func TestChangeRequestIntegration_PatchAssessProvisionsRequesterAsCancelled(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	scoped := repository.NewScoped(pool)
	sys := repository.WithSystemIdentity(context.Background())
	repo := repository.NewChangeRequestRepository(scoped)
	seedChangeRequestForAssessGateTest(t, scoped)
	seedAssessGateGroup(t, pool)
	seedTeamMembersForAssessGateTest(t, pool, changeRequestAssessGateMemberUserID, changeRequestAssessGateMemberUserID2)
	setAssessGateRequestedBy(t, scoped, changeRequestAssessGateMemberUserID)
	t.Cleanup(func() {
		_, _ = scoped.Exec(sys, `DELETE FROM approval_stage WHERE work_item_id = $1`, changeRequestAssessGateTestID)
	})

	teamID := changeRequestAssessGateGroupID
	assess := domain.ChangeRequestStateAssess
	if _, err := repo.PatchChangeRequest(sys, changeRequestAssessGateTestID,
		domain.PatchChangeRequestRequest{AssignedTeamID: &teamID, State: &assess}, "cr-assess-gate-test"); err != nil {
		t.Fatalf("PatchChangeRequest(assignedTeamId=%s, state=assess): %v", teamID, err)
	}

	var stageID string
	if scanErr := scoped.QueryRow(sys,
		`SELECT id FROM approval_stage WHERE work_item_id = $1`, changeRequestAssessGateTestID).Scan(&stageID); scanErr != nil {
		t.Fatalf("read back approval_stage: %v", scanErr)
	}

	rows, err := scoped.Query(sys,
		`SELECT approver_user_id::TEXT, status FROM approval_stage_approver WHERE stage_id = $1`, stageID)
	if err != nil {
		t.Fatalf("query approval_stage_approver: %v", err)
	}
	defer rows.Close()
	gotApprovers := map[string]string{}
	for rows.Next() {
		var uid, status string
		if err := rows.Scan(&uid, &status); err != nil {
			t.Fatalf("scan approval_stage_approver: %v", err)
		}
		gotApprovers[uid] = status
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("approval_stage_approver rows: %v", err)
	}

	want := map[string]string{
		changeRequestAssessGateMemberUserID:  "cancelled",
		changeRequestAssessGateMemberUserID2: "requested",
	}
	if len(gotApprovers) != len(want) {
		t.Fatalf("approval_stage_approver rows = %+v, want exactly %+v", gotApprovers, want)
	}
	for uid, wantStatus := range want {
		if gotApprovers[uid] != wantStatus {
			t.Fatalf("approver %s status = %q, want %q", uid, gotApprovers[uid], wantStatus)
		}
	}
}

// TestChangeRequestIntegration_PatchAssessRejectsWhenOnlyMemberIsRequester is
// the regression guard for the edge case the requester-exclusion rule above
// can reintroduce: a dead-end approval_stage nobody can ever approve, this
// time because the assigned team's only member is the change request's own
// requester, so excluding them leaves zero requested approvers. Same
// "validate BEFORE the stage is created" discipline as
// TestChangeRequestIntegration_PatchAssessRejectsEmptyGroup -- the whole
// {state: "assess"} PATCH must be rejected with a ValidationError and leave
// no approval_stage row behind at all.
func TestChangeRequestIntegration_PatchAssessRejectsWhenOnlyMemberIsRequester(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	scoped := repository.NewScoped(pool)
	sys := repository.WithSystemIdentity(context.Background())
	repo := repository.NewChangeRequestRepository(scoped)
	seedChangeRequestForAssessGateTest(t, scoped)
	seedAssessGateGroup(t, pool)
	seedTeamMembersForAssessGateTest(t, pool, changeRequestAssessGateMemberUserID)
	setAssessGateRequestedBy(t, scoped, changeRequestAssessGateMemberUserID)
	t.Cleanup(func() {
		_, _ = scoped.Exec(sys, `DELETE FROM approval_stage WHERE work_item_id = $1`, changeRequestAssessGateTestID)
	})

	teamID := changeRequestAssessGateGroupID
	assess := domain.ChangeRequestStateAssess
	_, err = repo.PatchChangeRequest(sys, changeRequestAssessGateTestID,
		domain.PatchChangeRequestRequest{AssignedTeamID: &teamID, State: &assess}, "cr-assess-gate-test")
	if err == nil {
		t.Fatal("PatchChangeRequest(state=assess) whose only team member is the requester succeeded, want a ValidationError")
	}
	var valErr *apierror.ValidationError
	if !errors.As(err, &valErr) {
		t.Fatalf("PatchChangeRequest(state=assess) whose only team member is the requester error = %v (%T), want *apierror.ValidationError", err, err)
	}

	var gotState string
	if scanErr := scoped.QueryRow(sys,
		`SELECT state::TEXT FROM change_request WHERE id = $1`, changeRequestAssessGateTestID).Scan(&gotState); scanErr != nil {
		t.Fatalf("read back state: %v", scanErr)
	}
	if gotState != "NEW" {
		t.Fatalf("state after a rejected assess patch = %q, want unchanged \"NEW\"", gotState)
	}

	var stageCount int
	if scanErr := scoped.QueryRow(sys,
		`SELECT COUNT(*) FROM approval_stage WHERE work_item_id = $1`, changeRequestAssessGateTestID).Scan(&stageCount); scanErr != nil {
		t.Fatalf("count approval_stage: %v", scanErr)
	}
	if stageCount != 0 {
		t.Fatalf("approval_stage rows after a rejected assess patch = %d, want 0 (no dead-end stage left behind)", stageCount)
	}
}

// --- Authorize checkpoint ("Risk approvals" in real ServiceNow) ---
//
// Mirrors the Assess-gate tests above exactly, with one structural
// difference: provisionApprovalStage only creates a stage when the number of
// approval_stage rows already on the work item equals the checkpoint's own
// ordinal position (0 for Assess, 1 for Authorize -- see that function's own
// doc comment for why). Every test below therefore seeds a single,
// pre-existing Assess-position stage first (seedExistingApprovalStage,
// bypassing provisionApprovalStage entirely -- its own approvers are never
// queried by these tests), so the {state: "authorize"} PATCH under test
// lands its own new stage at position 1, the ordinal
// changeRequestApprovalStagePosition reads as "Authorize".

// seedExistingApprovalStage inserts a single approval_stage row directly --
// bypassing provisionApprovalStage entirely -- so a test can establish "this
// work item already has an earlier checkpoint's own stage" as a
// precondition, without caring about that earlier stage's own approvers.
// assignment_group_id is seededGroupID purely for FK validity (see that
// constant's own doc comment on why it's safe to reuse for exactly this) --
// no test that calls this ever queries that group's members.
func seedExistingApprovalStage(t *testing.T, pool *repository.Scoped, workItemID string) {
	t.Helper()
	ctx := repository.WithSystemIdentity(context.Background())
	if _, err := pool.Exec(ctx,
		`INSERT INTO approval_stage (id, created_on, updated_on, created_by, updated_by, work_item_id, assignment_group_id)
		 VALUES (gen_random_uuid(), now(), now(), 'cr-authorize-gate-test', 'cr-authorize-gate-test', $1, $2::uuid)`,
		workItemID, seededGroupID); err != nil {
		t.Fatalf("seed existing (Assess-position) approval_stage: %v", err)
	}
}

// seedChangeRequestForReviewGateTest inserts a minimal work_item/
// change_request pair already sitting in Implement -- the realistic
// starting point for every Review-gate test below, since in production
// Review is only ever reached after Assess and Authorize have already run
// (there is no cascading approval-decision mechanism past Authorize, so
// Review is reached exclusively via a direct {state: "review"} PATCH along
// the main ...->Scheduled->Implement->Review path -- see
// patchChangeRequestTx's own Review-branch comment).
func seedChangeRequestForReviewGateTest(t *testing.T, pool *repository.Scoped) {
	t.Helper()
	// work_item and change_request are RLS-protected; seed/cleanup as internal.
	ctx := repository.WithSystemIdentity(context.Background())

	cleanup := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM work_item WHERE id = $1`, changeRequestReviewGateTestID)
	}
	cleanup()
	t.Cleanup(cleanup)

	if _, err := pool.Exec(ctx,
		`INSERT INTO work_item (id, created_on, updated_on, created_by, updated_by, number, subject, type)
		 VALUES ($1, now(), now(), 'cr-review-gate-test-creator', 'cr-review-gate-test', 'CRREV001', 'review gate test', 'CHANGE_REQUEST')`,
		changeRequestReviewGateTestID); err != nil {
		t.Fatalf("seed work_item: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO change_request (id, state) VALUES ($1, 'IMPLEMENT'::change_request_state_enum)`,
		changeRequestReviewGateTestID); err != nil {
		t.Fatalf("seed change_request: %v", err)
	}
}

// seedReviewGateGroup inserts changeRequestReviewGateGroupID's own "group"
// row -- a dedicated fixture for the tests below, deliberately not shared
// with changeRequestAssessGateGroupID, changeRequestAuthorizeGateGroupID or
// seededGroupID (same isolation reasoning as changeRequestAssessGateGroupID's
// own doc comment).
func seedReviewGateGroup(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	cleanup := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM "group" WHERE id = $1`, changeRequestReviewGateGroupID)
	}
	cleanup()
	t.Cleanup(cleanup)

	if _, err := pool.Exec(ctx,
		`INSERT INTO "group" (id, created_on, updated_on, created_by, updated_by, name)
		 VALUES ($1, now(), now(), 'cr-review-gate-test', 'cr-review-gate-test', 'Review Gate Test Group')`,
		changeRequestReviewGateGroupID); err != nil {
		t.Fatalf("seed group: %v", err)
	}
}

// seedTeamMembersForReviewGateTest inserts one "user" row and one
// team_member row (keyed by group_id, same reasoning as
// seedTeamMembersForAssessGateTest/seedTeamMembersForAuthorizeGateTest) per
// given user id, so they resolve as members of changeRequestReviewGateGroupID
// for the auto-provisioning tests below. Callers must seed that group row
// first (seedReviewGateGroup).
func seedTeamMembersForReviewGateTest(t *testing.T, pool *pgxpool.Pool, userIDs ...string) {
	t.Helper()
	ctx := context.Background()

	for i, userID := range userIDs {
		id := userID
		userCleanup := func() {
			_, _ = pool.Exec(ctx, `DELETE FROM "user" WHERE id = $1`, id)
		}
		userCleanup()
		t.Cleanup(userCleanup)

		email := fmt.Sprintf("cr-review-gate-member-%d@example.com", i+1)
		if _, err := pool.Exec(ctx,
			`INSERT INTO "user" (id, created_on, updated_on, created_by, updated_by, user_name, name, first_name, last_name, email, is_active, is_system_user)
			 VALUES ($1, now(), now(), 'cr-review-gate-test', 'cr-review-gate-test', $2, 'Review Gate Member', 'Review', 'Gate Member', $2, true, false)`,
			id, email); err != nil {
			t.Fatalf("seed team member user %s: %v", id, err)
		}

		memberCleanup := func() {
			_, _ = pool.Exec(ctx, `DELETE FROM team_member WHERE user_id = $1`, id)
		}
		memberCleanup()
		t.Cleanup(memberCleanup)

		// team_member.team_id is NOT NULL -- same seededGroupID-as-filler
		// reasoning as seedTeamMembersForAssessGateTest's own identical line.
		if _, err := pool.Exec(ctx,
			`INSERT INTO team_member (id, created_on, updated_on, created_by, updated_by, team_id, user_id, group_id)
			 VALUES (gen_random_uuid(), now(), now(), 'cr-review-gate-test', 'cr-review-gate-test', $1::uuid, $2, $3::uuid)`,
			seededGroupID, id, changeRequestReviewGateGroupID); err != nil {
			t.Fatalf("seed team_member for user %s: %v", id, err)
		}
	}
}

// setReviewGateRequestedBy stamps change_request.requested_by_user_id for
// changeRequestReviewGateTestID, mirroring setAssessGateRequestedBy/
// setAuthorizeGateRequestedBy.
func setReviewGateRequestedBy(t *testing.T, pool *repository.Scoped, userID string) {
	t.Helper()
	ctx := repository.WithSystemIdentity(context.Background())
	if _, err := pool.Exec(ctx,
		`UPDATE change_request SET requested_by_user_id = $1::uuid WHERE id = $2`,
		userID, changeRequestReviewGateTestID); err != nil {
		t.Fatalf("set change_request.requested_by_user_id: %v", err)
	}
}

// TestChangeRequestIntegration_PatchReviewProvisionsApproversFromGroupMembers
// is the Review-checkpoint counterpart of
// TestChangeRequestIntegration_PatchAuthorizeProvisionsApproversFromGroupMembers:
// a {state: "review"} PATCH against a change request that already has its
// Assess- and Authorize-position stages provisions one requested
// approval_stage_approver row per member of the assigned team, at a new,
// third approval_stage.
func TestChangeRequestIntegration_PatchReviewProvisionsApproversFromGroupMembers(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	scoped := repository.NewScoped(pool)
	sys := repository.WithSystemIdentity(context.Background())
	repo := repository.NewChangeRequestRepository(scoped)
	seedChangeRequestForReviewGateTest(t, scoped)
	// Two pre-existing stages -- standing in for the Assess- and
	// Authorize-position stages a real change request would already carry by
	// the time it reaches Review -- so provisionApprovalStage's own
	// COUNT(*) == checkpoint.Position gate (2) is satisfied.
	seedExistingApprovalStage(t, scoped, changeRequestReviewGateTestID)
	seedExistingApprovalStage(t, scoped, changeRequestReviewGateTestID)
	seedReviewGateGroup(t, pool)
	seedTeamMembersForReviewGateTest(t, pool, changeRequestReviewGateMemberUserID, changeRequestReviewGateMemberUserID2)
	t.Cleanup(func() {
		_, _ = scoped.Exec(sys, `DELETE FROM approval_stage WHERE work_item_id = $1`, changeRequestReviewGateTestID)
	})

	teamID := changeRequestReviewGateGroupID
	review := domain.ChangeRequestStateReview
	if _, err := repo.PatchChangeRequest(sys, changeRequestReviewGateTestID,
		domain.PatchChangeRequestRequest{AssignedTeamID: &teamID, State: &review}, "cr-review-gate-test"); err != nil {
		t.Fatalf("PatchChangeRequest(assignedTeamId=%s, state=review): %v", teamID, err)
	}

	var stageCount int
	if scanErr := scoped.QueryRow(sys,
		`SELECT COUNT(*) FROM approval_stage WHERE work_item_id = $1`, changeRequestReviewGateTestID).Scan(&stageCount); scanErr != nil {
		t.Fatalf("count approval_stage: %v", scanErr)
	}
	if stageCount != 3 {
		t.Fatalf("approval_stage rows after a review patch = %d, want exactly 3 (the two pre-existing stages plus the new Review stage)", stageCount)
	}

	var stageID, stageGroupID string
	if scanErr := scoped.QueryRow(sys,
		`SELECT id, assignment_group_id::TEXT FROM approval_stage WHERE work_item_id = $1 AND assignment_group_id = $2::uuid`,
		changeRequestReviewGateTestID, teamID).Scan(&stageID, &stageGroupID); scanErr != nil {
		t.Fatalf("read back review approval_stage: %v", scanErr)
	}
	if stageGroupID != teamID {
		t.Fatalf("approval_stage.assignment_group_id = %q, want %q", stageGroupID, teamID)
	}

	rows, err := scoped.Query(sys,
		`SELECT approver_user_id::TEXT, status FROM approval_stage_approver WHERE stage_id = $1 ORDER BY approver_user_id`, stageID)
	if err != nil {
		t.Fatalf("query approval_stage_approver: %v", err)
	}
	defer rows.Close()
	gotApprovers := map[string]string{}
	for rows.Next() {
		var uid, status string
		if err := rows.Scan(&uid, &status); err != nil {
			t.Fatalf("scan approval_stage_approver: %v", err)
		}
		gotApprovers[uid] = status
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("approval_stage_approver rows: %v", err)
	}

	want := map[string]string{
		changeRequestReviewGateMemberUserID:  "requested",
		changeRequestReviewGateMemberUserID2: "requested",
	}
	if len(gotApprovers) != len(want) {
		t.Fatalf("approval_stage_approver rows = %+v, want exactly %+v", gotApprovers, want)
	}
	for uid, wantStatus := range want {
		if gotApprovers[uid] != wantStatus {
			t.Fatalf("approver %s status = %q, want %q", uid, gotApprovers[uid], wantStatus)
		}
	}
}

// TestChangeRequestIntegration_PatchReviewDoesNotReprovisionWhenStageExists
// is the Review-checkpoint counterpart of
// TestChangeRequestIntegration_PatchAuthorizeDoesNotReprovisionWhenStageExists:
// a second {state: "review"} patch against a change request that already has
// its Review-position stage must never create a duplicate stage or re-seed
// approvers.
func TestChangeRequestIntegration_PatchReviewDoesNotReprovisionWhenStageExists(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	scoped := repository.NewScoped(pool)
	sys := repository.WithSystemIdentity(context.Background())
	repo := repository.NewChangeRequestRepository(scoped)
	seedChangeRequestForReviewGateTest(t, scoped)
	seedExistingApprovalStage(t, scoped, changeRequestReviewGateTestID)
	seedExistingApprovalStage(t, scoped, changeRequestReviewGateTestID)
	seedReviewGateGroup(t, pool)
	seedTeamMembersForReviewGateTest(t, pool, changeRequestReviewGateMemberUserID, changeRequestReviewGateMemberUserID2)
	t.Cleanup(func() {
		_, _ = scoped.Exec(sys, `DELETE FROM approval_stage WHERE work_item_id = $1`, changeRequestReviewGateTestID)
	})

	teamID := changeRequestReviewGateGroupID
	review := domain.ChangeRequestStateReview
	if _, err := repo.PatchChangeRequest(sys, changeRequestReviewGateTestID,
		domain.PatchChangeRequestRequest{AssignedTeamID: &teamID, State: &review}, "cr-review-gate-test"); err != nil {
		t.Fatalf("PatchChangeRequest(assignedTeamId=%s, state=review) #1: %v", teamID, err)
	}
	if _, err := repo.PatchChangeRequest(sys, changeRequestReviewGateTestID,
		domain.PatchChangeRequestRequest{State: &review}, "cr-review-gate-test"); err != nil {
		t.Fatalf("PatchChangeRequest(state=review) #2 (resend): %v", err)
	}

	var stageCount int
	if scanErr := scoped.QueryRow(sys,
		`SELECT COUNT(*) FROM approval_stage WHERE work_item_id = $1`, changeRequestReviewGateTestID).Scan(&stageCount); scanErr != nil {
		t.Fatalf("count approval_stage: %v", scanErr)
	}
	if stageCount != 3 {
		t.Fatalf("approval_stage rows after two review patches = %d, want exactly 3 (not reprovisioned)", stageCount)
	}

	var approverCount int
	if scanErr := scoped.QueryRow(sys,
		`SELECT COUNT(*) FROM approval_stage_approver asa JOIN approval_stage ast ON ast.id = asa.stage_id WHERE ast.work_item_id = $1`,
		changeRequestReviewGateTestID).Scan(&approverCount); scanErr != nil {
		t.Fatalf("count approval_stage_approver: %v", scanErr)
	}
	if approverCount != 2 {
		t.Fatalf("approval_stage_approver rows after two review patches = %d, want exactly 2 (not re-seeded)", approverCount)
	}
}

// TestChangeRequestIntegration_PatchReviewRejectsEmptyGroup is the
// Review-checkpoint counterpart of
// TestChangeRequestIntegration_PatchAuthorizeRejectsEmptyGroup: an assigned
// team with no members must reject the whole {state: "review"} PATCH with a
// ValidationError and leave no new approval_stage behind -- only the two
// pre-existing stages this test seeds up front.
func TestChangeRequestIntegration_PatchReviewRejectsEmptyGroup(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	scoped := repository.NewScoped(pool)
	sys := repository.WithSystemIdentity(context.Background())
	repo := repository.NewChangeRequestRepository(scoped)
	seedChangeRequestForReviewGateTest(t, scoped)
	seedExistingApprovalStage(t, scoped, changeRequestReviewGateTestID)
	seedExistingApprovalStage(t, scoped, changeRequestReviewGateTestID)
	seedReviewGateGroup(t, pool)
	// Deliberately no seedTeamMembersForReviewGateTest call -- the group
	// exists (so assignedTeamId itself is valid) but has zero members.
	t.Cleanup(func() {
		_, _ = scoped.Exec(sys, `DELETE FROM approval_stage WHERE work_item_id = $1`, changeRequestReviewGateTestID)
	})

	teamID := changeRequestReviewGateGroupID
	review := domain.ChangeRequestStateReview
	_, err = repo.PatchChangeRequest(sys, changeRequestReviewGateTestID,
		domain.PatchChangeRequestRequest{AssignedTeamID: &teamID, State: &review}, "cr-review-gate-test")
	if err == nil {
		t.Fatal("PatchChangeRequest(state=review) with an empty assigned team succeeded, want a ValidationError")
	}
	var valErr *apierror.ValidationError
	if !errors.As(err, &valErr) {
		t.Fatalf("PatchChangeRequest(state=review) with an empty assigned team error = %v (%T), want *apierror.ValidationError", err, err)
	}

	var stageCount int
	if scanErr := scoped.QueryRow(sys,
		`SELECT COUNT(*) FROM approval_stage WHERE work_item_id = $1`, changeRequestReviewGateTestID).Scan(&stageCount); scanErr != nil {
		t.Fatalf("count approval_stage: %v", scanErr)
	}
	if stageCount != 2 {
		t.Fatalf("approval_stage rows after a rejected review patch = %d, want exactly 2 (only the two pre-existing stages, no dead-end Review stage)", stageCount)
	}
}

// TestChangeRequestIntegration_PatchReviewDeduplicatesGroupMembers is the
// Review-checkpoint counterpart of
// TestChangeRequestIntegration_PatchAuthorizeDeduplicatesGroupMembers.
func TestChangeRequestIntegration_PatchReviewDeduplicatesGroupMembers(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	scoped := repository.NewScoped(pool)
	sys := repository.WithSystemIdentity(context.Background())
	repo := repository.NewChangeRequestRepository(scoped)
	seedChangeRequestForReviewGateTest(t, scoped)
	seedExistingApprovalStage(t, scoped, changeRequestReviewGateTestID)
	seedExistingApprovalStage(t, scoped, changeRequestReviewGateTestID)
	seedReviewGateGroup(t, pool)
	seedTeamMembersForReviewGateTest(t, pool, changeRequestReviewGateMemberUserID)
	t.Cleanup(func() {
		_, _ = scoped.Exec(sys, `DELETE FROM approval_stage WHERE work_item_id = $1`, changeRequestReviewGateTestID)
	})

	// A second team_member row for the SAME user against the SAME group --
	// seedTeamMembersForReviewGateTest's own cleanup (DELETE ... WHERE
	// user_id = $1) already covers this row too, since it shares the user id.
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO team_member (id, created_on, updated_on, created_by, updated_by, team_id, user_id, group_id)
		 VALUES (gen_random_uuid(), now(), now(), 'cr-review-gate-test', 'cr-review-gate-test', $1::uuid, $2, $3::uuid)`,
		seededGroupID, changeRequestReviewGateMemberUserID, changeRequestReviewGateGroupID); err != nil {
		t.Fatalf("seed duplicate team_member row: %v", err)
	}

	teamID := changeRequestReviewGateGroupID
	review := domain.ChangeRequestStateReview
	if _, err := repo.PatchChangeRequest(sys, changeRequestReviewGateTestID,
		domain.PatchChangeRequestRequest{AssignedTeamID: &teamID, State: &review}, "cr-review-gate-test"); err != nil {
		t.Fatalf("PatchChangeRequest(assignedTeamId=%s, state=review): %v", teamID, err)
	}

	var approverCount int
	if scanErr := scoped.QueryRow(sys,
		`SELECT COUNT(*) FROM approval_stage_approver asa
		 JOIN approval_stage ast ON ast.id = asa.stage_id
		 WHERE ast.work_item_id = $1 AND ast.assignment_group_id = $2::uuid`,
		changeRequestReviewGateTestID, teamID).Scan(&approverCount); scanErr != nil {
		t.Fatalf("count approval_stage_approver: %v", scanErr)
	}
	if approverCount != 1 {
		t.Fatalf("approval_stage_approver rows for a user with a duplicated team_member row = %d, want exactly 1", approverCount)
	}
}

// TestChangeRequestIntegration_PatchReviewProvisionsRequesterAsCancelled is
// the Review-checkpoint counterpart of
// TestChangeRequestIntegration_PatchAuthorizeProvisionsRequesterAsCancelled --
// the identical self-approval-exclusion rule applies at this checkpoint too.
func TestChangeRequestIntegration_PatchReviewProvisionsRequesterAsCancelled(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	scoped := repository.NewScoped(pool)
	sys := repository.WithSystemIdentity(context.Background())
	repo := repository.NewChangeRequestRepository(scoped)
	seedChangeRequestForReviewGateTest(t, scoped)
	seedExistingApprovalStage(t, scoped, changeRequestReviewGateTestID)
	seedExistingApprovalStage(t, scoped, changeRequestReviewGateTestID)
	seedReviewGateGroup(t, pool)
	seedTeamMembersForReviewGateTest(t, pool, changeRequestReviewGateMemberUserID, changeRequestReviewGateMemberUserID2)
	setReviewGateRequestedBy(t, scoped, changeRequestReviewGateMemberUserID)
	t.Cleanup(func() {
		_, _ = scoped.Exec(sys, `DELETE FROM approval_stage WHERE work_item_id = $1`, changeRequestReviewGateTestID)
	})

	teamID := changeRequestReviewGateGroupID
	review := domain.ChangeRequestStateReview
	if _, err := repo.PatchChangeRequest(sys, changeRequestReviewGateTestID,
		domain.PatchChangeRequestRequest{AssignedTeamID: &teamID, State: &review}, "cr-review-gate-test"); err != nil {
		t.Fatalf("PatchChangeRequest(assignedTeamId=%s, state=review): %v", teamID, err)
	}

	var stageID string
	if scanErr := scoped.QueryRow(sys,
		`SELECT id FROM approval_stage WHERE work_item_id = $1 AND assignment_group_id = $2::uuid`,
		changeRequestReviewGateTestID, teamID).Scan(&stageID); scanErr != nil {
		t.Fatalf("read back approval_stage: %v", scanErr)
	}

	rows, err := scoped.Query(sys,
		`SELECT approver_user_id::TEXT, status FROM approval_stage_approver WHERE stage_id = $1`, stageID)
	if err != nil {
		t.Fatalf("query approval_stage_approver: %v", err)
	}
	defer rows.Close()
	gotApprovers := map[string]string{}
	for rows.Next() {
		var uid, status string
		if err := rows.Scan(&uid, &status); err != nil {
			t.Fatalf("scan approval_stage_approver: %v", err)
		}
		gotApprovers[uid] = status
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("approval_stage_approver rows: %v", err)
	}

	want := map[string]string{
		changeRequestReviewGateMemberUserID:  "cancelled",
		changeRequestReviewGateMemberUserID2: "requested",
	}
	if len(gotApprovers) != len(want) {
		t.Fatalf("approval_stage_approver rows = %+v, want exactly %+v", gotApprovers, want)
	}
	for uid, wantStatus := range want {
		if gotApprovers[uid] != wantStatus {
			t.Fatalf("approver %s status = %q, want %q", uid, gotApprovers[uid], wantStatus)
		}
	}
}

// TestChangeRequestIntegration_PatchReviewRejectsWhenOnlyMemberIsRequester is
// the Review-checkpoint counterpart of
// TestChangeRequestIntegration_PatchAuthorizeRejectsWhenOnlyMemberIsRequester.
func TestChangeRequestIntegration_PatchReviewRejectsWhenOnlyMemberIsRequester(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	scoped := repository.NewScoped(pool)
	sys := repository.WithSystemIdentity(context.Background())
	repo := repository.NewChangeRequestRepository(scoped)
	seedChangeRequestForReviewGateTest(t, scoped)
	seedExistingApprovalStage(t, scoped, changeRequestReviewGateTestID)
	seedExistingApprovalStage(t, scoped, changeRequestReviewGateTestID)
	seedReviewGateGroup(t, pool)
	seedTeamMembersForReviewGateTest(t, pool, changeRequestReviewGateMemberUserID)
	setReviewGateRequestedBy(t, scoped, changeRequestReviewGateMemberUserID)
	t.Cleanup(func() {
		_, _ = scoped.Exec(sys, `DELETE FROM approval_stage WHERE work_item_id = $1`, changeRequestReviewGateTestID)
	})

	teamID := changeRequestReviewGateGroupID
	review := domain.ChangeRequestStateReview
	_, err = repo.PatchChangeRequest(sys, changeRequestReviewGateTestID,
		domain.PatchChangeRequestRequest{AssignedTeamID: &teamID, State: &review}, "cr-review-gate-test")
	if err == nil {
		t.Fatal("PatchChangeRequest(state=review) whose only team member is the requester succeeded, want a ValidationError")
	}
	var valErr *apierror.ValidationError
	if !errors.As(err, &valErr) {
		t.Fatalf("PatchChangeRequest(state=review) whose only team member is the requester error = %v (%T), want *apierror.ValidationError", err, err)
	}

	var stageCount int
	if scanErr := scoped.QueryRow(sys,
		`SELECT COUNT(*) FROM approval_stage WHERE work_item_id = $1`, changeRequestReviewGateTestID).Scan(&stageCount); scanErr != nil {
		t.Fatalf("count approval_stage: %v", scanErr)
	}
	if stageCount != 2 {
		t.Fatalf("approval_stage rows after a rejected review patch = %d, want exactly 2 (only the two pre-existing stages, no dead-end Review stage)", stageCount)
	}
}

// TestChangeRequestIntegration_AssessAuthorizeAndReviewStagesCoexist extends
// TestChangeRequestIntegration_AssessAndAuthorizeStagesCoexist's own real,
// full production path one checkpoint further: a {state: "assess"} PATCH
// (provisioning the Assess-position stage), an approval decision on it
// (DecideChangeRequestApproval's own Assess->Authorize cascade, which also
// provisions the Authorize-position stage as a best-effort step), and then a
// direct {state: "review"} PATCH -- the one real entry point Review's own
// provisioning has (see patchChangeRequestTx's own Review-branch comment) --
// provisioning a third, independent stage. Confirms all three stages land
// correctly and independently: the Assess stage's own approvers (one
// Approved, one Cancelled by the decision's own sibling-cancellation rule)
// and the Authorize stage's own approvers (both freshly "requested", reusing
// the Assess checkpoint's own assigned team) are both left completely
// untouched by the Review stage's own, separately-provisioned approvers
// (against a different, explicitly-supplied team), and each stage is
// attributed to its own, distinct id rather than any one clobbering another.
func TestChangeRequestIntegration_AssessAuthorizeAndReviewStagesCoexist(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	scoped := repository.NewScoped(pool)
	sys := repository.WithSystemIdentity(context.Background())
	repo := repository.NewChangeRequestRepository(scoped)
	seedChangeRequestForAssessGateTest(t, scoped)
	seedAssessGateGroup(t, pool)
	seedTeamMembersForAssessGateTest(t, pool, changeRequestAssessGateMemberUserID, changeRequestAssessGateMemberUserID2)
	seedReviewGateGroup(t, pool)
	seedTeamMembersForReviewGateTest(t, pool, changeRequestReviewGateMemberUserID, changeRequestReviewGateMemberUserID2)
	t.Cleanup(func() {
		_, _ = scoped.Exec(sys, `DELETE FROM approval_stage WHERE work_item_id = $1`, changeRequestAssessGateTestID)
	})

	assessTeamID := changeRequestAssessGateGroupID
	assess := domain.ChangeRequestStateAssess
	if _, err := repo.PatchChangeRequest(sys, changeRequestAssessGateTestID,
		domain.PatchChangeRequestRequest{AssignedTeamID: &assessTeamID, State: &assess}, "cr-assess-gate-test"); err != nil {
		t.Fatalf("PatchChangeRequest(assignedTeamId=%s, state=assess): %v", assessTeamID, err)
	}

	var assessStageID string
	if scanErr := scoped.QueryRow(sys,
		`SELECT id FROM approval_stage WHERE work_item_id = $1`, changeRequestAssessGateTestID).Scan(&assessStageID); scanErr != nil {
		t.Fatalf("read back Assess approval_stage: %v", scanErr)
	}

	if _, err := repo.DecideChangeRequestApproval(sys, changeRequestAssessGateTestID,
		changeRequestAssessGateMemberUserID, "approved", "cr-assess-gate-test"); err != nil {
		t.Fatalf("DecideChangeRequestApproval(approved): %v", err)
	}

	var gotState string
	if scanErr := scoped.QueryRow(sys,
		`SELECT state::TEXT FROM change_request WHERE id = $1`, changeRequestAssessGateTestID).Scan(&gotState); scanErr != nil {
		t.Fatalf("read back state after cascade: %v", scanErr)
	}
	if gotState != "AUTHORIZE" {
		t.Fatalf("state after approval = %q, want \"AUTHORIZE\"", gotState)
	}

	var authorizeStageID string
	if scanErr := scoped.QueryRow(sys,
		`SELECT id FROM approval_stage WHERE work_item_id = $1 AND id != $2`,
		changeRequestAssessGateTestID, assessStageID).Scan(&authorizeStageID); scanErr != nil {
		t.Fatalf("read back Authorize approval_stage: %v", scanErr)
	}

	// Review's own entry point: a direct {state: "review"} PATCH, supplying
	// a deliberately different assigned team so this test also confirms
	// Review's own provisioning uses THIS request's team, not whatever was
	// left on work_item by the Assess/Authorize steps above.
	reviewTeamID := changeRequestReviewGateGroupID
	review := domain.ChangeRequestStateReview
	if _, err := repo.PatchChangeRequest(sys, changeRequestAssessGateTestID,
		domain.PatchChangeRequestRequest{AssignedTeamID: &reviewTeamID, State: &review}, "cr-assess-gate-test"); err != nil {
		t.Fatalf("PatchChangeRequest(assignedTeamId=%s, state=review): %v", reviewTeamID, err)
	}

	var stageCount int
	if scanErr := scoped.QueryRow(sys,
		`SELECT COUNT(*) FROM approval_stage WHERE work_item_id = $1`, changeRequestAssessGateTestID).Scan(&stageCount); scanErr != nil {
		t.Fatalf("count approval_stage: %v", scanErr)
	}
	if stageCount != 3 {
		t.Fatalf("approval_stage rows after assess, the assess->authorize cascade, and a review patch = %d, want exactly 3", stageCount)
	}

	var reviewStageID string
	if scanErr := scoped.QueryRow(sys,
		`SELECT id FROM approval_stage WHERE work_item_id = $1 AND id != $2 AND id != $3`,
		changeRequestAssessGateTestID, assessStageID, authorizeStageID).Scan(&reviewStageID); scanErr != nil {
		t.Fatalf("read back Review approval_stage: %v", scanErr)
	}
	if reviewStageID == assessStageID || reviewStageID == authorizeStageID {
		t.Fatal("Review stage id collides with an earlier stage's id -- a new stage was not actually created")
	}

	// The Assess stage's own approvers: unchanged by anything that followed.
	assessApprovers := map[string]string{}
	assessRows, err := scoped.Query(sys,
		`SELECT approver_user_id::TEXT, status FROM approval_stage_approver WHERE stage_id = $1`, assessStageID)
	if err != nil {
		t.Fatalf("query Assess approval_stage_approver: %v", err)
	}
	for assessRows.Next() {
		var uid, status string
		if err := assessRows.Scan(&uid, &status); err != nil {
			assessRows.Close()
			t.Fatalf("scan Assess approval_stage_approver: %v", err)
		}
		assessApprovers[uid] = status
	}
	assessRows.Close()
	if err := assessRows.Err(); err != nil {
		t.Fatalf("Assess approval_stage_approver rows: %v", err)
	}
	wantAssess := map[string]string{
		changeRequestAssessGateMemberUserID:  "approved",
		changeRequestAssessGateMemberUserID2: "cancelled",
	}
	if len(assessApprovers) != len(wantAssess) {
		t.Fatalf("Assess approval_stage_approver rows = %+v, want exactly %+v", assessApprovers, wantAssess)
	}
	for uid, wantStatus := range wantAssess {
		if assessApprovers[uid] != wantStatus {
			t.Fatalf("Assess approver %s status = %q, want %q", uid, assessApprovers[uid], wantStatus)
		}
	}

	// The CAB stage's own approvers: both CAB Approval members, freshly
	// "requested" -- unchanged by the Review patch that followed.
	authorizeApprovers := map[string]string{}
	authorizeRows, err := scoped.Query(sys,
		`SELECT approver_user_id::TEXT, status FROM approval_stage_approver WHERE stage_id = $1`, authorizeStageID)
	if err != nil {
		t.Fatalf("query Authorize approval_stage_approver: %v", err)
	}
	for authorizeRows.Next() {
		var uid, status string
		if err := authorizeRows.Scan(&uid, &status); err != nil {
			authorizeRows.Close()
			t.Fatalf("scan Authorize approval_stage_approver: %v", err)
		}
		authorizeApprovers[uid] = status
	}
	authorizeRows.Close()
	if err := authorizeRows.Err(); err != nil {
		t.Fatalf("Authorize approval_stage_approver rows: %v", err)
	}
	wantAuthorize := map[string]string{
		crCABMemberUserID1: "requested",
		crCABMemberUserID2: "requested",
	}
	if len(authorizeApprovers) != len(wantAuthorize) {
		t.Fatalf("Authorize approval_stage_approver rows = %+v, want exactly %+v", authorizeApprovers, wantAuthorize)
	}
	for uid, wantStatus := range wantAuthorize {
		if authorizeApprovers[uid] != wantStatus {
			t.Fatalf("Authorize approver %s status = %q, want %q", uid, authorizeApprovers[uid], wantStatus)
		}
	}

	// The Review stage's own approvers: both members of the deliberately
	// different, explicitly-supplied review team, freshly "requested".
	reviewApprovers := map[string]string{}
	reviewRows, err := scoped.Query(sys,
		`SELECT approver_user_id::TEXT, status FROM approval_stage_approver WHERE stage_id = $1`, reviewStageID)
	if err != nil {
		t.Fatalf("query Review approval_stage_approver: %v", err)
	}
	for reviewRows.Next() {
		var uid, status string
		if err := reviewRows.Scan(&uid, &status); err != nil {
			reviewRows.Close()
			t.Fatalf("scan Review approval_stage_approver: %v", err)
		}
		reviewApprovers[uid] = status
	}
	reviewRows.Close()
	if err := reviewRows.Err(); err != nil {
		t.Fatalf("Review approval_stage_approver rows: %v", err)
	}
	wantReview := map[string]string{
		changeRequestReviewGateMemberUserID:  "requested",
		changeRequestReviewGateMemberUserID2: "requested",
	}
	if len(reviewApprovers) != len(wantReview) {
		t.Fatalf("Review approval_stage_approver rows = %+v, want exactly %+v", reviewApprovers, wantReview)
	}
	for uid, wantStatus := range wantReview {
		if reviewApprovers[uid] != wantStatus {
			t.Fatalf("Review approver %s status = %q, want %q", uid, reviewApprovers[uid], wantStatus)
		}
	}
}

// seedChangeRequestForOnHoldTest inserts a minimal work_item/change_request
// pair in the given state, with is_on_hold/on_hold_reason/on_hold_started_on
// set directly via SQL rather than through PatchChangeRequest -- the tests
// below are split between exercising the GATE (which needs an "already on
// hold" precondition to exist before the PATCH under test ever runs) and the
// WRITE path itself (covered separately), so seeding the precondition
// directly keeps the two concerns from tangling. onHold=false seeds a
// never-been-on-hold record (is_on_hold FALSE, reason/since left NULL),
// matching every change request created before migration 0178 ever ran.
func seedChangeRequestForOnHoldTest(t *testing.T, pool *repository.Scoped, state string, onHold bool, reason *string) {
	t.Helper()
	// work_item and change_request are RLS-protected; seed/cleanup as internal.
	ctx := repository.WithSystemIdentity(context.Background())

	cleanup := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM work_item WHERE id = $1`, changeRequestOnHoldTestID)
	}
	cleanup()
	t.Cleanup(cleanup)

	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed (%.60s): %v", sql, err)
		}
	}

	mustExec(`INSERT INTO work_item (id, created_on, updated_on, created_by, updated_by, number, subject, type)
	          VALUES ($1, now(), now(), 'cr-onhold-test', 'cr-onhold-test', 'CRONHOLD1', 'on-hold gate test', 'CHANGE_REQUEST')`,
		changeRequestOnHoldTestID)

	if onHold {
		mustExec(`INSERT INTO change_request (id, state, is_on_hold, on_hold_reason, on_hold_started_on)
		          VALUES ($1, $2::change_request_state_enum, TRUE, $3, now())`,
			changeRequestOnHoldTestID, state, reason)
	} else {
		mustExec(`INSERT INTO change_request (id, state, is_on_hold) VALUES ($1, $2::change_request_state_enum, FALSE)`,
			changeRequestOnHoldTestID, state)
	}
}

// TestChangeRequestIntegration_PatchOnHoldPersistsAndReadsBack confirms
// {onHold: true, onHoldReason: ...} actually persists change_request.is_on_hold/
// on_hold_reason/on_hold_started_on, both in PatchChangeRequest's own
// response and independently on a fresh GetChangeRequestByID read.
func TestChangeRequestIntegration_PatchOnHoldPersistsAndReadsBack(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	scoped := repository.NewScoped(pool)
	sys := repository.WithSystemIdentity(context.Background())
	repo := repository.NewChangeRequestRepository(scoped)
	seedChangeRequestForOnHoldTest(t, scoped, "NEW", false, nil)

	yes := true
	reason := "maintenance window"
	cr, err := repo.PatchChangeRequest(sys, changeRequestOnHoldTestID,
		domain.PatchChangeRequestRequest{OnHold: &yes, OnHoldReason: &reason}, "cr-onhold-test")
	if err != nil {
		t.Fatalf("PatchChangeRequest(onHold=true): %v", err)
	}
	if cr.OnHold == nil || !*cr.OnHold {
		t.Fatalf("OnHold after patch = %v, want true", cr.OnHold)
	}
	if cr.OnHoldReason == nil || *cr.OnHoldReason != reason {
		t.Fatalf("OnHoldReason after patch = %v, want %q", cr.OnHoldReason, reason)
	}
	if cr.OnHoldSince == nil || *cr.OnHoldSince == "" {
		t.Fatalf("OnHoldSince after patch = %v, want a non-empty timestamp", cr.OnHoldSince)
	}

	// Read back independently via GetChangeRequestByID -- not just trusting
	// PatchChangeRequest's own response -- to confirm this actually
	// persisted to change_request rather than only round-tripping in memory.
	got, err := repo.GetChangeRequestByID(sys, changeRequestOnHoldTestID)
	if err != nil {
		t.Fatalf("GetChangeRequestByID: %v", err)
	}
	if got.OnHold == nil || !*got.OnHold {
		t.Fatalf("GetChangeRequestByID OnHold = %v, want true", got.OnHold)
	}
	if got.OnHoldReason == nil || *got.OnHoldReason != reason {
		t.Fatalf("GetChangeRequestByID OnHoldReason = %v, want %q", got.OnHoldReason, reason)
	}
	if got.OnHoldSince == nil || *got.OnHoldSince != *cr.OnHoldSince {
		t.Fatalf("GetChangeRequestByID OnHoldSince = %v, want %v", got.OnHoldSince, cr.OnHoldSince)
	}
}

// TestChangeRequestIntegration_PatchStateRejectedWhileOnHold is the gate's
// main regression guard: a state-advancing PATCH against a change request
// that is CURRENTLY on hold (seeded directly, not via this same PATCH) must
// be refused with a ValidationError, and the state column itself must stay
// untouched.
func TestChangeRequestIntegration_PatchStateRejectedWhileOnHold(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	scoped := repository.NewScoped(pool)
	sys := repository.WithSystemIdentity(context.Background())
	repo := repository.NewChangeRequestRepository(scoped)
	reason := "customer change freeze"
	seedChangeRequestForOnHoldTest(t, scoped, "NEW", true, &reason)

	// CANCELED rather than ASSESS/AUTHORIZE/REVIEW deliberately -- this test
	// is about the on-hold gate specifically, not approver auto-provisioning,
	// and CANCELED needs no assignedTeamId and triggers no provisioning path
	// at all, so a failure here can only be the on-hold gate.
	target := domain.ChangeRequestStateCanceled
	_, err = repo.PatchChangeRequest(sys, changeRequestOnHoldTestID,
		domain.PatchChangeRequestRequest{State: &target}, "cr-onhold-test")
	if err == nil {
		t.Fatal("PatchChangeRequest(state) against an on-hold record: want error, got nil")
	}
	var ve *apierror.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("PatchChangeRequest(state) against an on-hold record: got %T (%v), want *apierror.ValidationError", err, err)
	}

	var gotState string
	if scanErr := scoped.QueryRow(sys,
		`SELECT state::TEXT FROM change_request WHERE id = $1`, changeRequestOnHoldTestID).Scan(&gotState); scanErr != nil {
		t.Fatalf("read back state: %v", scanErr)
	}
	if gotState != "NEW" {
		t.Fatalf("state after rejected patch = %q, want unchanged \"NEW\"", gotState)
	}
}

// TestChangeRequestIntegration_PatchClearsOnHoldAndAdvancesStateTogether
// confirms the one deliberate exception to the gate above: {state: X,
// onHold: false} in the SAME request is allowed through even though the
// record is currently on hold -- "take it off hold and advance in one
// call". Also confirms clearing OnHold in this combined request clears
// on_hold_reason/on_hold_started_on exactly the same way a standalone
// {onHold: false} would.
func TestChangeRequestIntegration_PatchClearsOnHoldAndAdvancesStateTogether(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	scoped := repository.NewScoped(pool)
	sys := repository.WithSystemIdentity(context.Background())
	repo := repository.NewChangeRequestRepository(scoped)
	reason := "customer change freeze"
	seedChangeRequestForOnHoldTest(t, scoped, "NEW", true, &reason)

	no := false
	target := domain.ChangeRequestStateCanceled
	cr, err := repo.PatchChangeRequest(sys, changeRequestOnHoldTestID,
		domain.PatchChangeRequestRequest{State: &target, OnHold: &no}, "cr-onhold-test")
	if err != nil {
		t.Fatalf("PatchChangeRequest(state=canceled, onHold=false) against an on-hold record: %v", err)
	}
	if cr.OnHold == nil || *cr.OnHold {
		t.Fatalf("OnHold after simultaneous clear+advance = %v, want false", cr.OnHold)
	}
	if cr.OnHoldReason != nil {
		t.Fatalf("OnHoldReason after simultaneous clear+advance = %v, want nil", cr.OnHoldReason)
	}
	if cr.OnHoldSince != nil {
		t.Fatalf("OnHoldSince after simultaneous clear+advance = %v, want nil", cr.OnHoldSince)
	}
	if cr.State == nil || *cr.State != string(domain.ChangeRequestStateCanceled) {
		t.Fatalf("state after simultaneous clear+advance = %v, want %q", cr.State, domain.ChangeRequestStateCanceled)
	}
}

// TestChangeRequestIntegration_PatchOffHoldAlwaysSucceeds confirms taking a
// record off hold (OnHold: false, with no other field at all -- no State)
// is never blocked, regardless of the record's current lifecycle state,
// terminal states included.
func TestChangeRequestIntegration_PatchOffHoldAlwaysSucceeds(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	scoped := repository.NewScoped(pool)
	sys := repository.WithSystemIdentity(context.Background())
	repo := repository.NewChangeRequestRepository(scoped)

	for _, seedState := range []string{"NEW", "ASSESS", "CLOSED", "CANCELED"} {
		t.Run(seedState, func(t *testing.T) {
			reason := "temporary freeze"
			seedChangeRequestForOnHoldTest(t, scoped, seedState, true, &reason)

			no := false
			cr, err := repo.PatchChangeRequest(sys, changeRequestOnHoldTestID,
				domain.PatchChangeRequestRequest{OnHold: &no}, "cr-onhold-test")
			if err != nil {
				t.Fatalf("PatchChangeRequest(onHold=false) on a %s-state record: %v", seedState, err)
			}
			if cr.OnHold == nil || *cr.OnHold {
				t.Fatalf("OnHold after patch = %v, want false", cr.OnHold)
			}
			if cr.OnHoldReason != nil {
				t.Fatalf("OnHoldReason after patch = %v, want nil", cr.OnHoldReason)
			}
			if cr.OnHoldSince != nil {
				t.Fatalf("OnHoldSince after patch = %v, want nil", cr.OnHoldSince)
			}
			if cr.State == nil || strings.ToUpper(*cr.State) != seedState {
				t.Fatalf("state after off-hold patch = %v, want unchanged %q", cr.State, seedState)
			}
		})
	}
}

// TestChangeRequestIntegration_PatchNonStateFieldSucceedsWhileOnHold confirms
// the gate is scoped exactly to State: a PATCH that never touches State at
// all (editing Description here) must succeed normally even while the
// record is on hold, and must leave the on-hold columns completely
// untouched -- being on hold only ever blocks a state-changing PATCH, never
// any other field.
func TestChangeRequestIntegration_PatchNonStateFieldSucceedsWhileOnHold(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	scoped := repository.NewScoped(pool)
	sys := repository.WithSystemIdentity(context.Background())
	repo := repository.NewChangeRequestRepository(scoped)
	reason := "vendor maintenance window"
	seedChangeRequestForOnHoldTest(t, scoped, "NEW", true, &reason)

	newDescription := "updated while on hold"
	cr, err := repo.PatchChangeRequest(sys, changeRequestOnHoldTestID,
		domain.PatchChangeRequestRequest{Description: &newDescription}, "cr-onhold-test")
	if err != nil {
		t.Fatalf("PatchChangeRequest(description) on an on-hold record: %v", err)
	}
	if cr.Description == nil || *cr.Description != newDescription {
		t.Fatalf("Description after patch = %v, want %q", cr.Description, newDescription)
	}
	if cr.OnHold == nil || !*cr.OnHold {
		t.Fatalf("OnHold after unrelated patch = %v, want unchanged true", cr.OnHold)
	}
	if cr.OnHoldReason == nil || *cr.OnHoldReason != reason {
		t.Fatalf("OnHoldReason after unrelated patch = %v, want unchanged %q", cr.OnHoldReason, reason)
	}
}

// seedChangeRequestCustomerFlagAccount inserts one account/account_contact
// pair shared by every customer-flag test below (account_contact_id is
// just an FK -- project_contact carries its own, independent email column,
// so one account_contact row can back every project_contact fixture these
// tests seed).
func seedChangeRequestCustomerFlagAccount(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	cleanup := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM account_contact WHERE id = $1`, crCustomerFlagAccountContactID)
		_, _ = pool.Exec(ctx, `DELETE FROM account WHERE id = $1`, crCustomerFlagAccountID)
	}
	cleanup()
	t.Cleanup(cleanup)

	if _, err := pool.Exec(ctx,
		`INSERT INTO account (id, created_on, updated_on, created_by, updated_by, name, number, sf_id)
		 VALUES ($1, now(), now(), 'cr-customer-flag-test', 'cr-customer-flag-test', 'CR Customer Flag Test Account', 'CR-CF-ACC-1', 'CR-CF-SF-ACC-1')`,
		crCustomerFlagAccountID); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO account_contact (id, created_on, updated_on, created_by, updated_by, user_name, account_id)
		 VALUES ($1, now(), now(), 'cr-customer-flag-test', 'cr-customer-flag-test', 'CR Customer Flag Test Contact', $2)`,
		crCustomerFlagAccountContactID, crCustomerFlagAccountID); err != nil {
		t.Fatalf("seed account_contact: %v", err)
	}
}

// seedChangeRequestCustomerFlagProject inserts one project row under
// crCustomerFlagAccountID -- id/key must be unique per call site (project.key
// is UNIQUE).
func seedChangeRequestCustomerFlagProject(t *testing.T, pool *pgxpool.Pool, projectID, key string) {
	t.Helper()
	ctx := context.Background()

	cleanup := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM project WHERE id = $1`, projectID)
	}
	cleanup()
	t.Cleanup(cleanup)

	if _, err := pool.Exec(ctx,
		`INSERT INTO project (id, created_on, updated_on, created_by, updated_by, key, sf_id, name, account_id)
		 VALUES ($1, now(), now(), 'cr-customer-flag-test', 'cr-customer-flag-test', $2, $2, $2, $3)`,
		projectID, key, crCustomerFlagAccountID); err != nil {
		t.Fatalf("seed project %s: %v", key, err)
	}
}

// ensureProjectRole guarantees a project_role row for role exists --
// idempotent, same "safe to run against a database that already has it"
// discipline as migration 0128_project_admin_role_group.sql's own ADMIN-role
// seed. project_role rows are otherwise only ever created by the ServiceNow
// sync job (see case_repo_announcement_visibility_integration_test.go's own
// doc comment on project_role/project_group/project_group_role), which a
// bare local docker-compose stack never runs -- PORTAL_USER/SECURITY_CONTACT
// cannot be assumed present there. Left in place afterward, not cleaned up:
// it is reference/catalog data, same as every project_role row any other
// caller might also depend on existing.
func ensureProjectRole(t *testing.T, pool *pgxpool.Pool, role string) string {
	t.Helper()
	ctx := context.Background()

	var id string
	err := pool.QueryRow(ctx, `SELECT id::text FROM project_role WHERE role = $1::project_role_enum`, role).Scan(&id)
	if err == nil {
		return id
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("look up project_role %s: %v", role, err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO project_role (id, created_on, updated_on, created_by, updated_by, role)
		 VALUES (gen_random_uuid(), now(), now(), 'cr-customer-flag-test', 'cr-customer-flag-test', $1::project_role_enum)
		 RETURNING id::text`, role).Scan(&id); err != nil {
		t.Fatalf("seed project_role %s: %v", role, err)
	}
	return id
}

// seedProjectContactWithRole inserts a project_contact row (email, state) on
// projectID, holding role via its own dedicated project_group/
// project_group_role pair -- the identical project_contact ->
// project_contact_group -> project_group_role -> project_role join chain
// CaseRepository.ProjectContactEmailsByRole/ProjectContactRepository's own
// projectContactColumns already use, which
// callerMayGrantChangeRequestCustomerFlag (change_request_repo.go) reuses
// verbatim for this feature's own authorization check. groupName must be
// unique across this whole test file (project_group."group" is UNIQUE).
func seedProjectContactWithRole(t *testing.T, pool *pgxpool.Pool, projectID, email, state, role, groupName string) {
	t.Helper()
	ctx := context.Background()
	roleID := ensureProjectRole(t, pool, role)

	var groupID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO project_group (id, created_on, updated_on, created_by, updated_by, "group")
		 VALUES (gen_random_uuid(), now(), now(), 'cr-customer-flag-test', 'cr-customer-flag-test', $1)
		 RETURNING id::text`, groupName).Scan(&groupID); err != nil {
		t.Fatalf("seed project_group %s: %v", groupName, err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM project_group WHERE id = $1`, groupID) })

	if _, err := pool.Exec(ctx,
		`INSERT INTO project_group_role (id, created_on, updated_on, created_by, updated_by, project_group_id, project_role_id)
		 VALUES (gen_random_uuid(), now(), now(), 'cr-customer-flag-test', 'cr-customer-flag-test', $1, $2)`,
		groupID, roleID); err != nil {
		t.Fatalf("seed project_group_role: %v", err)
	}

	var contactID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO project_contact (id, created_on, updated_on, created_by, updated_by, email, account_contact_id, project_id, state)
		 VALUES (gen_random_uuid(), now(), now(), 'cr-customer-flag-test', 'cr-customer-flag-test', $1, $2, $3, $4::project_contact_state_enum)
		 RETURNING id::text`, email, crCustomerFlagAccountContactID, projectID, state).Scan(&contactID); err != nil {
		t.Fatalf("seed project_contact %s: %v", email, err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM project_contact WHERE id = $1`, contactID) })

	if _, err := pool.Exec(ctx,
		`INSERT INTO project_contact_group (id, created_on, updated_on, created_by, updated_by, project_contact_id, project_group_id)
		 VALUES (gen_random_uuid(), now(), now(), 'cr-customer-flag-test', 'cr-customer-flag-test', $1, $2)`,
		contactID, groupID); err != nil {
		t.Fatalf("seed project_contact_group: %v", err)
	}
}

// seedChangeRequestForCustomerFlagTest inserts a minimal work_item/
// change_request pair linked to projectID, with is_customer_approved/
// is_customer_reviewed seeded directly via SQL -- bypassing PatchChangeRequest
// entirely -- to whatever precondition a given test needs to exist before the
// PATCH under test ever runs. Same split-precondition-from-write-path
// discipline as seedChangeRequestForOnHoldTest's own doc comment.
func seedChangeRequestForCustomerFlagTest(t *testing.T, pool *repository.Scoped, id, number, projectID string, approved, reviewed bool) {
	t.Helper()
	// work_item/change_request are RLS-protected; seed/cleanup as internal.
	ctx := repository.WithSystemIdentity(context.Background())

	cleanup := func() {
		_, _ = pool.Exec(ctx, `DELETE FROM work_item WHERE id = $1`, id)
	}
	cleanup()
	t.Cleanup(cleanup)

	mustExec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed (%.70s): %v", sql, err)
		}
	}

	mustExec(`INSERT INTO work_item (id, created_on, updated_on, created_by, updated_by, number, subject, type, project_id)
	          VALUES ($1, now(), now(), 'cr-customer-flag-test', 'cr-customer-flag-test', $2, 'customer flag auth test', 'CHANGE_REQUEST', $3::uuid)`,
		id, number, projectID)
	mustExec(`INSERT INTO change_request (id, state, is_customer_approved, is_customer_reviewed)
	          VALUES ($1, 'NEW'::change_request_state_enum, $2, $3)`,
		id, approved, reviewed)
}

// externalCallerCtx builds a ctx carrying a non-internal (Unrestricted:
// false) caller identity for email -- repository.Scoped's own InTx/Query/
// QueryRow/Exec only need SOME identity present (CallerIdentityFromContext's
// ok=true), and change_request/work_item's own RLS policies resolve
// project membership for this email server-side from project_contact
// (rls.go's setViewerProjectIDsSQL), not from anything set here.
func externalCallerCtx(email string) context.Context {
	return repository.WithCallerIdentity(context.Background(), repository.SearchScope{ViewerEmail: email})
}

// TestChangeRequestIntegration_PatchCustomerFlagInternalCallerCanSetBoth
// confirms an internal caller may flip BOTH is_customer_approved and
// is_customer_reviewed from false to true, in one PATCH, with no project
// membership or project_contact row involved at all.
func TestChangeRequestIntegration_PatchCustomerFlagInternalCallerCanSetBoth(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	scoped := repository.NewScoped(pool)
	sys := repository.WithSystemIdentity(context.Background())
	repo := repository.NewChangeRequestRepository(scoped)

	seedChangeRequestCustomerFlagAccount(t, pool)
	seedChangeRequestCustomerFlagProject(t, pool, crCustomerFlagProjectID, "CRCFPROJ01")
	seedChangeRequestForCustomerFlagTest(t, scoped, crCustomerFlagInternalTestID, "CRCFINT001", crCustomerFlagProjectID, false, false)

	yes := true
	cr, err := repo.PatchChangeRequest(sys, crCustomerFlagInternalTestID,
		domain.PatchChangeRequestRequest{IsCustomerApproved: &yes, IsCustomerReviewed: &yes}, "cr-customer-flag-test")
	if err != nil {
		t.Fatalf("PatchChangeRequest(isCustomerApproved=true, isCustomerReviewed=true) as internal: %v", err)
	}
	if !cr.HasCustomerApproved {
		t.Error("HasCustomerApproved after patch = false, want true")
	}
	if !cr.HasCustomerReviewed {
		t.Error("HasCustomerReviewed after patch = false, want true")
	}

	got, err := repo.GetChangeRequestByID(sys, crCustomerFlagInternalTestID)
	if err != nil {
		t.Fatalf("GetChangeRequestByID: %v", err)
	}
	if !got.HasCustomerApproved || !got.HasCustomerReviewed {
		t.Errorf("GetChangeRequestByID HasCustomerApproved/HasCustomerReviewed = %v/%v, want true/true", got.HasCustomerApproved, got.HasCustomerReviewed)
	}
}

// TestChangeRequestIntegration_PatchCustomerFlagQualifyingPortalUserContactCanApprove
// confirms a REGISTERED project_contact holding PORTAL_USER on the change
// request's OWN project may flip is_customer_approved false -> true.
func TestChangeRequestIntegration_PatchCustomerFlagQualifyingPortalUserContactCanApprove(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	scoped := repository.NewScoped(pool)
	repo := repository.NewChangeRequestRepository(scoped)

	seedChangeRequestCustomerFlagAccount(t, pool)
	seedChangeRequestCustomerFlagProject(t, pool, crCustomerFlagProjectID, "CRCFPROJ02")
	seedProjectContactWithRole(t, pool, crCustomerFlagProjectID,
		crCustomerFlagPortalUserEmail, "REGISTERED", "PORTAL_USER", "CR Customer Flag Portal User Group")
	seedChangeRequestForCustomerFlagTest(t, scoped, crCustomerFlagQualifyingTestID, "CRCFQUAL01", crCustomerFlagProjectID, false, false)

	yes := true
	cr, err := repo.PatchChangeRequest(externalCallerCtx(crCustomerFlagPortalUserEmail), crCustomerFlagQualifyingTestID,
		domain.PatchChangeRequestRequest{IsCustomerApproved: &yes}, crCustomerFlagPortalUserEmail)
	if err != nil {
		t.Fatalf("PatchChangeRequest(isCustomerApproved=true) as a REGISTERED PORTAL_USER contact on the same project: %v", err)
	}
	if !cr.HasCustomerApproved {
		t.Error("HasCustomerApproved after patch = false, want true")
	}
}

// TestChangeRequestIntegration_PatchCustomerFlagContactOnDifferentProjectCannot
// confirms a REGISTERED PORTAL_USER contact on a DIFFERENT project than the
// change request's own cannot flip its flag -- expected as *apierror.
// ForbiddenError, this feature's own authorization check
// (callerMayGrantChangeRequestCustomerFlag only matches a project_contact
// row against THIS change request's own project_id, so a contact registered
// on some other project never matches). Confirmed live against this
// suite's own CHANGE_REQUEST_TEST_DSN that this is genuinely what fires
// here, not work_item's own RLS (migration 0147) rejecting the write one
// layer earlier as a NotFoundError instead: this stack's DSN connects as
// the `postgres` role, a real Postgres superuser, which unconditionally
// bypasses every RLS policy regardless of FORCE ROW LEVEL SECURITY (a
// Postgres behavior, not a bug here or in migration 0147) -- so the
// work_item UPDATE this PATCH also performs always passes in this
// environment regardless of project membership, and this feature's own
// check is genuinely the only thing standing between this caller and the
// write. A deployment connecting as a non-superuser role would intercept
// this same scenario one layer earlier, as a NotFoundError from work_item's
// own RLS instead -- either way, the caller cannot flip the flag.
func TestChangeRequestIntegration_PatchCustomerFlagContactOnDifferentProjectCannot(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	scoped := repository.NewScoped(pool)
	sys := repository.WithSystemIdentity(context.Background())
	repo := repository.NewChangeRequestRepository(scoped)

	seedChangeRequestCustomerFlagAccount(t, pool)
	seedChangeRequestCustomerFlagProject(t, pool, crCustomerFlagProjectID, "CRCFPROJ03")
	seedChangeRequestCustomerFlagProject(t, pool, crCustomerFlagOtherProjectID, "CRCFPROJ04")
	seedProjectContactWithRole(t, pool, crCustomerFlagOtherProjectID,
		crCustomerFlagOtherProjectEmail, "REGISTERED", "PORTAL_USER", "CR Customer Flag Other Project Group")
	seedChangeRequestForCustomerFlagTest(t, scoped, crCustomerFlagDifferentProjectTestID, "CRCFDIFF01", crCustomerFlagProjectID, false, false)

	yes := true
	_, err = repo.PatchChangeRequest(externalCallerCtx(crCustomerFlagOtherProjectEmail), crCustomerFlagDifferentProjectTestID,
		domain.PatchChangeRequestRequest{IsCustomerApproved: &yes}, crCustomerFlagOtherProjectEmail)
	if err == nil {
		t.Fatal("PatchChangeRequest(isCustomerApproved=true) from a different project's own contact: want error, got nil")
	}
	var fe *apierror.ForbiddenError
	if !errors.As(err, &fe) {
		t.Fatalf("got %T (%v), want *apierror.ForbiddenError", err, err)
	}

	var gotApproved *bool
	if scanErr := scoped.QueryRow(sys, `SELECT is_customer_approved FROM change_request WHERE id = $1`, crCustomerFlagDifferentProjectTestID).Scan(&gotApproved); scanErr != nil {
		t.Fatalf("read back is_customer_approved: %v", scanErr)
	}
	if gotApproved != nil && *gotApproved {
		t.Fatal("is_customer_approved after rejected patch = true, want unchanged false")
	}
}

// TestChangeRequestIntegration_PatchCustomerFlagWrongRoleContactForbiddenStateUnaffected
// covers two of this feature's own required guarantees together: (1) a
// REGISTERED contact who holds no PORTAL_USER role anywhere on the change
// request's project -- a literal "project with no qualifying contact" -- is
// rejected with a ForbiddenError (this feature's own authorization check,
// not RLS: the contact genuinely IS a project member, so the write reaches
// change_request_repo.go's own gate), and (2) that rejection has NO bearing
// whatsoever on the change request's own state transitions: a separate
// {state: "canceled"} PATCH immediately afterward (CANCELED needs no
// assignedTeamId and triggers no approval-provisioning path, isolating this
// assertion to the on-hold/customer-flag gates alone, same reasoning
// TestChangeRequestIntegration_PatchStateRejectedWhileOnHold's own doc
// comment already uses) still succeeds normally.
func TestChangeRequestIntegration_PatchCustomerFlagWrongRoleContactForbiddenStateUnaffected(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	scoped := repository.NewScoped(pool)
	sys := repository.WithSystemIdentity(context.Background())
	repo := repository.NewChangeRequestRepository(scoped)

	seedChangeRequestCustomerFlagAccount(t, pool)
	seedChangeRequestCustomerFlagProject(t, pool, crCustomerFlagNoRoleProjectID, "CRCFPROJ05")
	seedProjectContactWithRole(t, pool, crCustomerFlagNoRoleProjectID,
		crCustomerFlagWrongRoleEmail, "REGISTERED", "SECURITY_CONTACT", "CR Customer Flag Wrong Role Group")
	seedChangeRequestForCustomerFlagTest(t, scoped, crCustomerFlagWrongRoleTestID, "CRCFWRNG01", crCustomerFlagNoRoleProjectID, false, false)

	yes := true
	_, err = repo.PatchChangeRequest(externalCallerCtx(crCustomerFlagWrongRoleEmail), crCustomerFlagWrongRoleTestID,
		domain.PatchChangeRequestRequest{IsCustomerApproved: &yes}, crCustomerFlagWrongRoleEmail)
	if err == nil {
		t.Fatal("PatchChangeRequest(isCustomerApproved=true) from a REGISTERED non-PORTAL_USER contact: want error, got nil")
	}
	var fe *apierror.ForbiddenError
	if !errors.As(err, &fe) {
		t.Fatalf("got %T (%v), want *apierror.ForbiddenError", err, err)
	}

	var gotApproved *bool
	if scanErr := scoped.QueryRow(sys, `SELECT is_customer_approved FROM change_request WHERE id = $1`, crCustomerFlagWrongRoleTestID).Scan(&gotApproved); scanErr != nil {
		t.Fatalf("read back is_customer_approved: %v", scanErr)
	}
	if gotApproved != nil && *gotApproved {
		t.Fatal("is_customer_approved after rejected patch = true, want unchanged false")
	}

	// The rejection above must have zero bearing on this same change
	// request's own state transitions.
	target := domain.ChangeRequestStateCanceled
	cr, err := repo.PatchChangeRequest(sys, crCustomerFlagWrongRoleTestID,
		domain.PatchChangeRequestRequest{State: &target}, "cr-customer-flag-test")
	if err != nil {
		t.Fatalf("PatchChangeRequest(state=canceled) after a customer-flag rejection on the same record: %v", err)
	}
	if cr.State == nil || *cr.State != string(domain.ChangeRequestStateCanceled) {
		t.Fatalf("state after patch = %v, want %q", cr.State, domain.ChangeRequestStateCanceled)
	}
}

// TestChangeRequestIntegration_PatchCustomerFlagInvitedContactCannotReach
// confirms registration state matters as much as role: a contact holding
// PORTAL_USER but still only INVITED (never accepted) on the change
// request's own project cannot flip its flag -- callerMayGrantChangeRequestCustomerFlag's
// own query requires state = REGISTERED exactly, so an INVITED contact
// never matches, surfacing the same *apierror.ForbiddenError* as the
// different-project case above and for the identical reason explained
// there (this environment's DSN connects as a Postgres superuser, which
// unconditionally bypasses work_item's own RLS, so this feature's own
// check is genuinely what is exercised here).
func TestChangeRequestIntegration_PatchCustomerFlagInvitedContactCannotReach(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	scoped := repository.NewScoped(pool)
	repo := repository.NewChangeRequestRepository(scoped)

	seedChangeRequestCustomerFlagAccount(t, pool)
	seedChangeRequestCustomerFlagProject(t, pool, crCustomerFlagProjectID, "CRCFPROJ06")
	seedProjectContactWithRole(t, pool, crCustomerFlagProjectID,
		crCustomerFlagInvitedEmail, "INVITED", "PORTAL_USER", "CR Customer Flag Invited Group")
	seedChangeRequestForCustomerFlagTest(t, scoped, crCustomerFlagInvitedTestID, "CRCFINVT01", crCustomerFlagProjectID, false, false)

	yes := true
	_, err = repo.PatchChangeRequest(externalCallerCtx(crCustomerFlagInvitedEmail), crCustomerFlagInvitedTestID,
		domain.PatchChangeRequestRequest{IsCustomerApproved: &yes}, crCustomerFlagInvitedEmail)
	if err == nil {
		t.Fatal("PatchChangeRequest(isCustomerApproved=true) from a still-INVITED PORTAL_USER contact: want error, got nil")
	}
	var fe *apierror.ForbiddenError
	if !errors.As(err, &fe) {
		t.Fatalf("got %T (%v), want *apierror.ForbiddenError", err, err)
	}
}

// TestChangeRequestIntegration_PatchCustomerFlagLockedTrueCannotRevert is the
// main regression guard for this feature's one-way lock: once either flag
// is true, a PATCH attempting to set it back to false is always rejected
// with a ValidationError -- regardless of whether the caller is internal or
// the same qualifying customer contact that could have set it true in the
// first place.
func TestChangeRequestIntegration_PatchCustomerFlagLockedTrueCannotRevert(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	scoped := repository.NewScoped(pool)
	sys := repository.WithSystemIdentity(context.Background())
	repo := repository.NewChangeRequestRepository(scoped)

	seedChangeRequestCustomerFlagAccount(t, pool)
	seedChangeRequestCustomerFlagProject(t, pool, crCustomerFlagProjectID, "CRCFPROJ07")
	seedProjectContactWithRole(t, pool, crCustomerFlagProjectID,
		crCustomerFlagPortalUserEmail, "REGISTERED", "PORTAL_USER", "CR Customer Flag Portal User Group")
	seedChangeRequestForCustomerFlagTest(t, scoped, crCustomerFlagLockedTestID, "CRCFLOCK01", crCustomerFlagProjectID, true, true)

	no := false

	// Internal caller: still rejected -- there is no override path.
	_, err = repo.PatchChangeRequest(sys, crCustomerFlagLockedTestID,
		domain.PatchChangeRequestRequest{IsCustomerApproved: &no}, "cr-customer-flag-test")
	if err == nil {
		t.Fatal("PatchChangeRequest(isCustomerApproved=false) as internal against an already-true record: want error, got nil")
	}
	var ve *apierror.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("got %T (%v), want *apierror.ValidationError", err, err)
	}

	// The same qualifying contact that could have set it true: also rejected.
	_, err = repo.PatchChangeRequest(externalCallerCtx(crCustomerFlagPortalUserEmail), crCustomerFlagLockedTestID,
		domain.PatchChangeRequestRequest{IsCustomerReviewed: &no}, crCustomerFlagPortalUserEmail)
	if err == nil {
		t.Fatal("PatchChangeRequest(isCustomerReviewed=false) as the qualifying contact against an already-true record: want error, got nil")
	}
	if !errors.As(err, &ve) {
		t.Fatalf("got %T (%v), want *apierror.ValidationError", err, err)
	}

	var gotApproved, gotReviewed bool
	if scanErr := scoped.QueryRow(sys, `SELECT is_customer_approved, is_customer_reviewed FROM change_request WHERE id = $1`,
		crCustomerFlagLockedTestID).Scan(&gotApproved, &gotReviewed); scanErr != nil {
		t.Fatalf("read back: %v", scanErr)
	}
	if !gotApproved || !gotReviewed {
		t.Fatalf("is_customer_approved/is_customer_reviewed after both rejected patches = %v/%v, want unchanged true/true", gotApproved, gotReviewed)
	}
}

// TestChangeRequestIntegration_PatchCustomerFlagFalseToFalseNoOpSucceeds
// confirms setting an already-false flag to false again is a no-op that
// always succeeds trivially -- deliberately exercised by a caller who does
// NOT qualify to grant it (a REGISTERED contact holding no PORTAL_USER
// role), proving no-op writes need no authorization check at all.
func TestChangeRequestIntegration_PatchCustomerFlagFalseToFalseNoOpSucceeds(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	scoped := repository.NewScoped(pool)
	repo := repository.NewChangeRequestRepository(scoped)

	seedChangeRequestCustomerFlagAccount(t, pool)
	seedChangeRequestCustomerFlagProject(t, pool, crCustomerFlagNoRoleProjectID, "CRCFPROJ08")
	seedProjectContactWithRole(t, pool, crCustomerFlagNoRoleProjectID,
		crCustomerFlagWrongRoleEmail, "REGISTERED", "SECURITY_CONTACT", "CR Customer Flag No-op Wrong Role Group")
	seedChangeRequestForCustomerFlagTest(t, scoped, crCustomerFlagNoOpTestID, "CRCFNOOP01", crCustomerFlagNoRoleProjectID, false, false)

	no := false
	cr, err := repo.PatchChangeRequest(externalCallerCtx(crCustomerFlagWrongRoleEmail), crCustomerFlagNoOpTestID,
		domain.PatchChangeRequestRequest{IsCustomerApproved: &no, IsCustomerReviewed: &no}, crCustomerFlagWrongRoleEmail)
	if err != nil {
		t.Fatalf("PatchChangeRequest(isCustomerApproved=false, isCustomerReviewed=false) no-op from a non-qualifying contact: %v", err)
	}
	if cr.HasCustomerApproved || cr.HasCustomerReviewed {
		t.Fatalf("HasCustomerApproved/HasCustomerReviewed after no-op patch = %v/%v, want false/false", cr.HasCustomerApproved, cr.HasCustomerReviewed)
	}
}

// TestChangeRequestIntegration_PatchCustomerFlagsLockIndependentPerField
// confirms one field's own lock state has no bearing on the other's: a
// single PATCH with is_customer_approved ALREADY true (so true -> true, a
// no-op, always allowed) and is_customer_reviewed false -> true (the one
// real gated transition, allowed here since the caller is internal)
// succeeds as a whole, changing only is_customer_reviewed.
func TestChangeRequestIntegration_PatchCustomerFlagsLockIndependentPerField(t *testing.T) {
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	scoped := repository.NewScoped(pool)
	sys := repository.WithSystemIdentity(context.Background())
	repo := repository.NewChangeRequestRepository(scoped)

	seedChangeRequestCustomerFlagAccount(t, pool)
	seedChangeRequestCustomerFlagProject(t, pool, crCustomerFlagProjectID, "CRCFPROJ09")
	seedChangeRequestForCustomerFlagTest(t, scoped, crCustomerFlagIndependentTestID, "CRCFINDP01", crCustomerFlagProjectID, true, false)

	yes := true
	cr, err := repo.PatchChangeRequest(sys, crCustomerFlagIndependentTestID,
		domain.PatchChangeRequestRequest{IsCustomerApproved: &yes, IsCustomerReviewed: &yes}, "cr-customer-flag-test")
	if err != nil {
		t.Fatalf("PatchChangeRequest(isCustomerApproved=true [no-op], isCustomerReviewed=true [new]): %v", err)
	}
	if !cr.HasCustomerApproved {
		t.Error("HasCustomerApproved after patch = false, want unchanged true")
	}
	if !cr.HasCustomerReviewed {
		t.Error("HasCustomerReviewed after patch = false, want true")
	}
}

// ---------------------------------------------------------------------------
// Type-dependent approval flow (change_request_approval_flow.go).
//
// Lifecycle tests for Normal (peer then CAB), Emergency (ECAB only) and
// Standard (no approval), the creator/SRE approver rules, the CAB/ECAB groups
// the migration creates, the automatic move to Scheduled, and the mandatory
// type on create -- all against the real Postgres this file's neighbours use:
//
//	CHANGE_REQUEST_TEST_DSN=postgres://... go test ./internal/repository/ -run ChangeRequestFlowIntegration
// ---------------------------------------------------------------------------

const (
	// crCABGroupID / crECABGroupID are the fixed ids migration
	// 0188_change_request_approval_groups.sql gives the two groups.
	crCABGroupID  = "00000000-0000-4000-8000-00000000ca01"
	crECABGroupID = "00000000-0000-4000-8000-00000000eca1"

	// crCABMemberUserID{1,2} / crECABMemberUserID are seeded as members of the
	// CAB / ECAB groups by the tests that need an eligible second approver.
	crCABMemberUserID1 = "3aaaaaaa-0000-0000-0000-0000000000c1"
	crCABMemberUserID2 = "3aaaaaaa-0000-0000-0000-0000000000c2"
	crECABMemberUserID = "3aaaaaaa-0000-0000-0000-0000000000e1"

	crFlowSubject = "cr-approval-flow integration test"

	crFlowCreatorID  = "3aaaaaaa-0000-0000-0000-000000000001"
	crFlowPeerAID    = "3aaaaaaa-0000-0000-0000-000000000002"
	crFlowPeerBID    = "3aaaaaaa-0000-0000-0000-000000000003"
	crFlowSREID      = "3aaaaaaa-0000-0000-0000-000000000004"
	crFlowOutsiderID = "3aaaaaaa-0000-0000-0000-000000000005"

	// crFlowGroupID is the change's assigned group: creator, both peers and
	// one SRE-team member belong to it.
	crFlowGroupID = "3aaaaaaa-0000-0000-0000-0000000000a1"
	// crFlowSREGroupID is an SRE group (Apollo-like): only the SRE member.
	crFlowSREGroupID = "3aaaaaaa-0000-0000-0000-0000000000a2"
	// crFlowSRETeamID is the "team" row of type sre-abt the SRE member belongs to.
	crFlowSRETeamID = "3aaaaaaa-0000-0000-0000-0000000000a3"
	// crFlowDevopsGroupID is the peer approval fallback group ("Devops Approval").
	crFlowDevopsGroupID = "3aaaaaaa-0000-0000-0000-0000000000a4"
)

func crFlowEmail(userID string) string {
	return fmt.Sprintf("crflow-%s@example.com", userID[len(userID)-12:])
}

// seedApprovalGroupMembers makes each userID a (freshly seeded) user and a
// member of the "group" groupID (team_member.group_id). The group row must
// exist. Everything is removed again on cleanup.
func seedApprovalGroupMembers(t *testing.T, pool *repository.Scoped, groupID string, userIDs ...string) {
	t.Helper()
	ctx := repository.WithSystemIdentity(context.Background())
	isolateApprovalGroup(t, pool, groupID)
	for _, uid := range userIDs {
		id := uid
		cleanup := func() { _, _ = pool.Exec(ctx, `DELETE FROM "user" WHERE id = $1`, id) }
		cleanup()
		t.Cleanup(cleanup)
		if _, err := pool.Exec(ctx,
			`INSERT INTO "user" (id, created_on, updated_on, created_by, updated_by, user_name, name, first_name, last_name, email, is_active, is_system_user)
			 VALUES ($1, now(), now(), 'cr-flow-test', 'cr-flow-test', $2, 'CR Flow User', 'CR', 'Flow User', $2, true, false)`,
			id, crFlowEmail(id)); err != nil {
			t.Fatalf("seed user %s: %v", id, err)
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO team_member (id, created_on, updated_on, created_by, updated_by, team_id, user_id, group_id)
			 VALUES (gen_random_uuid(), now(), now(), 'cr-flow-test', 'cr-flow-test', $1::uuid, $2, $3::uuid)`,
			seededGroupID, id, groupID); err != nil {
			t.Fatalf("seed team_member %s: %v", id, err)
		}
	}
}

var isolatedApprovalGroups sync.Map

// isolateApprovalGroup removes whoever is already a member of the CAB / ECAB
// group (local seed data puts the two dev users there; a synced environment
// has real members) for the duration of the calling test, so the test sees
// exactly the members it seeds itself, and puts them back on cleanup. Once per
// test and group; any other group is left alone.
func isolateApprovalGroup(t *testing.T, pool *repository.Scoped, groupID string) {
	t.Helper()
	if groupID != crCABGroupID && groupID != crECABGroupID {
		return
	}
	key := t.Name() + "/" + groupID
	if _, done := isolatedApprovalGroups.LoadOrStore(key, true); done {
		return
	}
	ctx := repository.WithSystemIdentity(context.Background())
	rows, err := pool.Query(ctx, `SELECT id::text, team_id::text, user_id::text FROM team_member WHERE group_id = $1::uuid`, groupID)
	if err != nil {
		t.Fatalf("snapshot approval group members: %v", err)
	}
	type member struct{ id, team, user string }
	var saved []member
	for rows.Next() {
		var m member
		if err := rows.Scan(&m.id, &m.team, &m.user); err != nil {
			rows.Close()
			t.Fatalf("scan approval group member: %v", err)
		}
		saved = append(saved, m)
	}
	rows.Close()
	if _, err := pool.Exec(ctx, `DELETE FROM team_member WHERE group_id = $1::uuid`, groupID); err != nil {
		t.Fatalf("clear approval group members: %v", err)
	}
	t.Cleanup(func() {
		isolatedApprovalGroups.Delete(key)
		for _, m := range saved {
			_, _ = pool.Exec(ctx,
				`INSERT INTO team_member (id, created_on, updated_on, created_by, updated_by, team_id, user_id, group_id)
				 VALUES ($1::uuid, now(), now(), 'cr-flow-test', 'cr-flow-test', $2::uuid, $3::uuid, $4::uuid) ON CONFLICT (id) DO NOTHING`,
				m.id, m.team, m.user, groupID)
		}
	})
}

type crFlow struct {
	t      *testing.T
	pool   *pgxpool.Pool
	scoped *repository.Scoped
	repo   repository.ChangeRequestRepository
	sys    context.Context
}

func newCRFlow(t *testing.T) *crFlow {
	t.Helper()
	dsn := os.Getenv("CHANGE_REQUEST_TEST_DSN")
	if dsn == "" {
		t.Skip("CHANGE_REQUEST_TEST_DSN not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	scoped := repository.NewScoped(pool)
	f := &crFlow{t: t, pool: pool, scoped: scoped, repo: repository.NewChangeRequestRepository(scoped), sys: repository.WithSystemIdentity(context.Background())}

	clean := func() {
		_, _ = scoped.Exec(f.sys, `DELETE FROM work_item WHERE subject = $1`, crFlowSubject)
		for _, g := range []string{crFlowGroupID, crFlowSREGroupID, crFlowDevopsGroupID} {
			_, _ = scoped.Exec(f.sys, `DELETE FROM "group" WHERE id = $1`, g)
		}
		_, _ = scoped.Exec(f.sys, `DELETE FROM team WHERE id = $1`, crFlowSRETeamID)
	}
	clean()
	t.Cleanup(clean)
	// Tests that seed no CAB/ECAB members expect those groups empty.
	isolateApprovalGroup(t, scoped, crCABGroupID)
	isolateApprovalGroup(t, scoped, crECABGroupID)
	return f
}

// seedAssignedGroup creates the assigned group (creator, peers A/B, and the
// SRE member who is in an sre-abt team) plus the SRE team and SRE group.
func (f *crFlow) seedAssignedGroup() {
	f.t.Helper()
	mustExec := func(sql string, args ...any) {
		f.t.Helper()
		if _, err := f.scoped.Exec(f.sys, sql, args...); err != nil {
			f.t.Fatalf("seed (%.60s): %v", sql, err)
		}
	}
	for id, name := range map[string]string{crFlowGroupID: "CR Flow Assigned Group", crFlowSREGroupID: "CR Flow SRE Group"} {
		mustExec(`INSERT INTO "group" (id, created_on, updated_on, created_by, updated_by, name) VALUES ($1, now(), now(), 'cr-flow-test', 'cr-flow-test', $2)`, id, name)
	}
	mustExec(`INSERT INTO team (id, created_on, updated_on, created_by, updated_by, name, type, key)
	          VALUES ($1, now(), now(), 'cr-flow-test', 'cr-flow-test', 'CR Flow SRE Team', 'sre-abt', 'crflow-sre')`, crFlowSRETeamID)

	seedApprovalGroupMembers(f.t, f.scoped, crFlowGroupID, crFlowCreatorID, crFlowPeerAID, crFlowPeerBID)
	seedApprovalGroupMembers(f.t, f.scoped, crFlowGroupID, crFlowSREID)
	// The SRE member belongs to an SRE team (Apollo-like) as well as the
	// assigned group.
	f.makeSRE(crFlowSREID, crFlowGroupID)
	seedApprovalGroupMembers(f.t, f.scoped, crFlowGroupID, crFlowOutsiderID)
}

// makeSRE puts userID into the sre-abt team (team_id = the SRE team) while
// keeping their membership of group groupID.
func (f *crFlow) makeSRE(userID, groupID string) {
	f.t.Helper()
	if _, err := f.scoped.Exec(f.sys, `DELETE FROM team_member WHERE user_id = $1`, userID); err != nil {
		f.t.Fatalf("reset team_member: %v", err)
	}
	if _, err := f.scoped.Exec(f.sys,
		`INSERT INTO team_member (id, created_on, updated_on, created_by, updated_by, team_id, user_id, group_id)
		 VALUES (gen_random_uuid(), now(), now(), 'cr-flow-test', 'cr-flow-test', $1::uuid, $2, $3::uuid)`,
		crFlowSRETeamID, userID, groupID); err != nil {
		f.t.Fatalf("seed SRE team_member: %v", err)
	}
}

func (f *crFlow) create(typ domain.ChangeRequestType, groupID string) string {
	f.t.Helper()
	g := groupID
	resp, err := f.repo.CreateChangeRequest(f.sys, domain.CreateChangeRequestRequest{
		Subject: crFlowSubject, Type: &typ, GroupID: &g,
	}, crFlowEmail(crFlowCreatorID))
	if err != nil {
		f.t.Fatalf("CreateChangeRequest(%s): %v", typ, err)
	}
	// RequestedBy is the creator too (the portal sends the signed-in user).
	if _, err := f.scoped.Exec(f.sys, `UPDATE change_request SET requested_by_user_id = $1::uuid WHERE id = $2`, crFlowCreatorID, resp.ChangeRequest.ID); err != nil {
		f.t.Fatalf("set requested_by: %v", err)
	}
	return resp.ChangeRequest.ID
}

func (f *crFlow) patchState(id string, state domain.ChangeRequestState) (domain.ChangeRequest, error) {
	s := state
	return f.repo.PatchChangeRequest(f.sys, id, domain.PatchChangeRequestRequest{State: &s}, crFlowEmail(crFlowCreatorID))
}

func (f *crFlow) requestApproval(id string) {
	f.t.Helper()
	if _, err := f.patchState(id, domain.ChangeRequestStateAssess); err != nil {
		f.t.Fatalf("Request Approval ({state: assess}): %v", err)
	}
}

func (f *crFlow) state(id string) string {
	f.t.Helper()
	var st string
	if err := f.scoped.QueryRow(f.sys, `SELECT COALESCE(state::text, '') FROM change_request WHERE id = $1`, id).Scan(&st); err != nil {
		f.t.Fatalf("read state: %v", err)
	}
	return st
}

func (f *crFlow) legal(id string) []string {
	f.t.Helper()
	cr, err := f.repo.GetChangeRequestByID(f.sys, id)
	if err != nil {
		f.t.Fatalf("GetChangeRequestByID: %v", err)
	}
	return cr.LegalNextStates
}

type crFlowStage struct {
	label     string
	groupID   string
	approvers map[string]string
}

func (f *crFlow) stages(id string) []crFlowStage {
	f.t.Helper()
	rows, err := f.scoped.Query(f.sys,
		`SELECT id, COALESCE(checkpoint_label, ''), COALESCE(assignment_group_id::text, '') FROM approval_stage WHERE work_item_id = $1 ORDER BY created_on, id`, id)
	if err != nil {
		f.t.Fatalf("query stages: %v", err)
	}
	type raw struct{ id, label, group string }
	var raws []raw
	for rows.Next() {
		var r raw
		if err := rows.Scan(&r.id, &r.label, &r.group); err != nil {
			rows.Close()
			f.t.Fatalf("scan stage: %v", err)
		}
		raws = append(raws, r)
	}
	rows.Close()
	var out []crFlowStage
	for _, r := range raws {
		st := crFlowStage{label: r.label, groupID: r.group, approvers: map[string]string{}}
		arows, err := f.scoped.Query(f.sys, `SELECT approver_user_id::text, status FROM approval_stage_approver WHERE stage_id = $1`, r.id)
		if err != nil {
			f.t.Fatalf("query approvers: %v", err)
		}
		for arows.Next() {
			var uid, status string
			if err := arows.Scan(&uid, &status); err != nil {
				arows.Close()
				f.t.Fatalf("scan approver: %v", err)
			}
			st.approvers[uid] = status
		}
		arows.Close()
		out = append(out, st)
	}
	return out
}

func (f *crFlow) decide(id, userID, decision string) error {
	_, err := f.repo.DecideChangeRequestApproval(f.sys, id, userID, decision, crFlowEmail(userID))
	return err
}

func assertStates(t *testing.T, what string, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("%s = %v, want %v", what, got, want)
	}
}

func assertApprovers(t *testing.T, what string, got map[string]string, want map[string]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s approvers = %v, want %v", what, got, want)
	}
	for uid, st := range want {
		if got[uid] != st {
			t.Fatalf("%s approver %s = %q, want %q (all: %v)", what, uid, got[uid], st, got)
		}
	}
}

// Normal: New -> (Request Approval) Assess [Peer Approval] -> Authorize [CAB
// Approval] -> Scheduled automatically on CAB approval -> Implement -> Review
// -> Closed. Also pins legalNextStates at every step and that Scheduled is
// never offered or accepted manually.
func TestChangeRequestFlowIntegration_NormalFullLifecycle(t *testing.T) {
	f := newCRFlow(t)
	f.seedAssignedGroup()
	seedApprovalGroupMembers(t, f.scoped, crCABGroupID, crCABMemberUserID1, crCABMemberUserID2)
	id := f.create(domain.ChangeRequestTypeNormal, crFlowGroupID)

	if got := f.state(id); got != "NEW" {
		t.Fatalf("state after create = %q, want NEW", got)
	}
	assertStates(t, "legalNextStates(New)", f.legal(id), "assess", "canceled")

	// Request Approval -> Assess with the PEER stage: the two peers are
	// requested, the creator is cancelled (never approves their own change),
	// the SRE-team member is not provisioned at all.
	f.requestApproval(id)
	if got := f.state(id); got != "ASSESS" {
		t.Fatalf("state after Request Approval = %q, want ASSESS", got)
	}
	stages := f.stages(id)
	if len(stages) != 1 || stages[0].label != "Peer Approval" || stages[0].groupID != crFlowGroupID {
		t.Fatalf("stages after Request Approval = %+v, want exactly one \"Peer Approval\" stage on the assigned group", stages)
	}
	assertApprovers(t, "peer stage", stages[0].approvers, map[string]string{
		crFlowCreatorID: "cancelled", crFlowPeerAID: "requested", crFlowPeerBID: "requested", crFlowOutsiderID: "requested",
	})
	assertStates(t, "legalNextStates(Assess)", f.legal(id), "authorize", "canceled")

	// There is no manual shortcut past the approvals.
	for _, target := range []domain.ChangeRequestState{domain.ChangeRequestStateScheduled, domain.ChangeRequestStateAuthorize} {
		_, err := f.patchState(id, target)
		var ve *apierror.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("manual {state: %s} from Assess err = %v (%T), want *apierror.ValidationError", target, err, err)
		}
	}
	if got := f.state(id); got != "ASSESS" {
		t.Fatalf("state after refused manual transitions = %q, want still ASSESS", got)
	}

	// Peer approval right after Request Approval: CAB is its own group and
	// comes next.
	if err := f.decide(id, crFlowPeerAID, "approved"); err != nil {
		t.Fatalf("peer approval: %v", err)
	}
	if got := f.state(id); got != "AUTHORIZE" {
		t.Fatalf("state after peer approval = %q, want AUTHORIZE", got)
	}
	stages = f.stages(id)
	if len(stages) != 2 || stages[1].label != "CAB Approval" || stages[1].groupID != crCABGroupID {
		t.Fatalf("stages after peer approval = %+v, want a second \"CAB Approval\" stage on the CAB group", stages)
	}
	assertApprovers(t, "peer stage after approval", stages[0].approvers, map[string]string{
		crFlowCreatorID: "cancelled", crFlowPeerAID: "approved", crFlowPeerBID: "cancelled", crFlowOutsiderID: "cancelled",
	})
	assertApprovers(t, "CAB stage", stages[1].approvers, map[string]string{crCABMemberUserID1: "requested", crCABMemberUserID2: "requested"})
	assertStates(t, "legalNextStates(Authorize)", f.legal(id), "canceled")

	// A peer cannot also give the CAB approval (not in the CAB group), and no
	// manual Schedule exists.
	if err := f.decide(id, crFlowPeerBID, "approved"); err == nil {
		t.Fatal("a peer approver with no CAB row decided the CAB stage, want NotFound")
	}
	if _, err := f.patchState(id, domain.ChangeRequestStateScheduled); err == nil {
		t.Fatal("manual {state: scheduled} from Authorize succeeded, want a refusal")
	}

	// CAB approval moves the change to Scheduled by itself.
	if err := f.decide(id, crCABMemberUserID1, "approved"); err != nil {
		t.Fatalf("CAB approval: %v", err)
	}
	if got := f.state(id); got != "SCHEDULED" {
		t.Fatalf("state after CAB approval = %q, want SCHEDULED (automatic)", got)
	}
	assertStates(t, "legalNextStates(Scheduled)", f.legal(id), "implement", "canceled")

	for _, step := range []struct {
		to   domain.ChangeRequestState
		want string
	}{
		{domain.ChangeRequestStateImplement, "IMPLEMENT"},
		{domain.ChangeRequestStateReview, "REVIEW"},
		{domain.ChangeRequestStateClosed, "CLOSED"},
	} {
		if _, err := f.patchState(id, step.to); err != nil {
			t.Fatalf("PATCH state %s: %v", step.to, err)
		}
		if got := f.state(id); got != step.want {
			t.Fatalf("state after %s = %q, want %q", step.to, got, step.want)
		}
	}
	if got := f.legal(id); got != nil {
		t.Fatalf("legalNextStates(Closed) = %v, want none", got)
	}
}

// Emergency: no peer approval; Request Approval goes straight to Authorize
// with ONLY the ECAB stage (its own group, not CAB); ECAB approval schedules.
func TestChangeRequestFlowIntegration_EmergencyLifecycle(t *testing.T) {
	f := newCRFlow(t)
	f.seedAssignedGroup()
	// CAB members exist too -- an emergency must NOT involve them.
	seedApprovalGroupMembers(t, f.scoped, crCABGroupID, crCABMemberUserID1)
	seedApprovalGroupMembers(t, f.scoped, crECABGroupID, crECABMemberUserID)
	id := f.create(domain.ChangeRequestTypeEmergency, crFlowGroupID)

	assertStates(t, "legalNextStates(New)", f.legal(id), "assess", "canceled")
	f.requestApproval(id)

	if got := f.state(id); got != "AUTHORIZE" {
		t.Fatalf("emergency state after Request Approval = %q, want AUTHORIZE (no peer step)", got)
	}
	stages := f.stages(id)
	if len(stages) != 1 || stages[0].label != "ECAB Approval" || stages[0].groupID != crECABGroupID {
		t.Fatalf("emergency stages = %+v, want exactly one \"ECAB Approval\" stage on the ECAB group", stages)
	}
	assertApprovers(t, "ECAB stage", stages[0].approvers, map[string]string{crECABMemberUserID: "requested"})
	assertStates(t, "legalNextStates(Authorize)", f.legal(id), "canceled")

	// Neither a peer nor a regular CAB member can decide an emergency.
	for _, uid := range []string{crFlowPeerAID, crCABMemberUserID1} {
		if err := f.decide(id, uid, "approved"); err == nil {
			t.Fatalf("user %s decided the ECAB stage without an ECAB row", uid)
		}
	}

	if err := f.decide(id, crECABMemberUserID, "approved"); err != nil {
		t.Fatalf("ECAB approval: %v", err)
	}
	if got := f.state(id); got != "SCHEDULED" {
		t.Fatalf("state after ECAB approval = %q, want SCHEDULED (automatic)", got)
	}
	assertStates(t, "legalNextStates(Scheduled)", f.legal(id), "implement", "canceled")
	if len(f.stages(id)) != 1 {
		t.Fatalf("emergency ended with %d stages, want 1", len(f.stages(id)))
	}
}

// Standard: no approval stages at all; Request Approval goes straight to
// Scheduled, then Implement -> Review -> Closed.
func TestChangeRequestFlowIntegration_StandardLifecycle(t *testing.T) {
	f := newCRFlow(t)
	f.seedAssignedGroup()
	id := f.create(domain.ChangeRequestTypeStandard, crFlowGroupID)

	assertStates(t, "legalNextStates(New)", f.legal(id), "assess", "canceled")
	f.requestApproval(id)

	if got := f.state(id); got != "SCHEDULED" {
		t.Fatalf("standard state after Request Approval = %q, want SCHEDULED (no approval needed)", got)
	}
	if n := len(f.stages(id)); n != 0 {
		t.Fatalf("standard change has %d approval stages, want none (neither Peer nor CAB)", n)
	}
	assertStates(t, "legalNextStates(Scheduled)", f.legal(id), "implement", "canceled")

	// Request Approval is only legal from New: it must not drag a Scheduled
	// change back.
	if _, err := f.patchState(id, domain.ChangeRequestStateAssess); err != nil {
		t.Fatalf("idempotent resend of Request Approval on a Scheduled Standard change: %v", err)
	}
	for _, to := range []domain.ChangeRequestState{domain.ChangeRequestStateImplement, domain.ChangeRequestStateReview} {
		if _, err := f.patchState(id, to); err != nil {
			t.Fatalf("PATCH state %s: %v", to, err)
		}
	}
	if _, err := f.patchState(id, domain.ChangeRequestStateClosed); err != nil {
		t.Fatalf("PATCH state closed: %v", err)
	}
	if n := len(f.stages(id)); n != 0 {
		t.Fatalf("standard change ended with %d approval stages, want none (Review approval is Normal-only)", n)
	}
}

// Request Approval is only legal from New; once the change has moved on it is
// refused rather than dragging the state backward.
func TestChangeRequestFlowIntegration_RequestApprovalOnlyFromNew(t *testing.T) {
	f := newCRFlow(t)
	f.seedAssignedGroup()
	id := f.create(domain.ChangeRequestTypeStandard, crFlowGroupID)
	f.requestApproval(id)
	if _, err := f.patchState(id, domain.ChangeRequestStateImplement); err != nil {
		t.Fatalf("implement: %v", err)
	}
	_, err := f.patchState(id, domain.ChangeRequestStateAssess)
	var ve *apierror.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("Request Approval on an Implement change err = %v (%T), want *apierror.ValidationError", err, err)
	}
	if got := f.state(id); got != "IMPLEMENT" {
		t.Fatalf("state after refused Request Approval = %q, want IMPLEMENT", got)
	}
}

// The creator may never approve their own change request -- neither the
// peer stage nor CAB/ECAB -- but may still cancel it.
func TestChangeRequestFlowIntegration_CreatorCannotApproveAnyStage(t *testing.T) {
	f := newCRFlow(t)
	f.seedAssignedGroup()
	seedApprovalGroupMembers(t, f.scoped, crCABGroupID, crCABMemberUserID1)
	id := f.create(domain.ChangeRequestTypeNormal, crFlowGroupID)
	f.requestApproval(id)

	assertForbidden := func(what, userID string) {
		t.Helper()
		err := f.decide(id, userID, "approved")
		var fe *apierror.ForbiddenError
		if !errors.As(err, &fe) {
			t.Fatalf("%s: err = %v (%T), want *apierror.ForbiddenError", what, err, err)
		}
		if !strings.Contains(fe.Msg, "creator") {
			t.Fatalf("%s: message %q should say the creator cannot approve", what, fe.Msg)
		}
	}

	// Peer stage: the creator's row was born cancelled; even a REQUESTED row
	// forced in by drift must not be decidable.
	assertForbidden("creator on the peer stage (cancelled row)", crFlowCreatorID)
	if _, err := f.scoped.Exec(f.sys,
		`UPDATE approval_stage_approver SET status = 'requested' WHERE work_item_id = $1 AND approver_user_id = $2`, id, crFlowCreatorID); err != nil {
		t.Fatalf("force creator row to requested: %v", err)
	}
	assertForbidden("creator on the peer stage (requested row)", crFlowCreatorID)
	rejectErr := f.decide(id, crFlowCreatorID, "rejected")
	var fe *apierror.ForbiddenError
	if !errors.As(rejectErr, &fe) {
		t.Fatalf("creator rejecting err = %v, want ForbiddenError (the creator does not decide at all)", rejectErr)
	}

	// CAB stage: put the creator in the CAB group's stage by force.
	if err := f.decide(id, crFlowPeerAID, "approved"); err != nil {
		t.Fatalf("peer approval: %v", err)
	}
	if _, err := f.scoped.Exec(f.sys,
		`INSERT INTO approval_stage_approver (id, created_on, updated_on, created_by, updated_by, stage_id, work_item_id, approver_user_id, status)
		 SELECT gen_random_uuid(), now(), now(), 'cr-flow-test', 'cr-flow-test', s.id, s.work_item_id, $2::uuid, 'requested'
		 FROM approval_stage s WHERE s.work_item_id = $1 AND s.checkpoint_label = 'CAB Approval'`, id, crFlowCreatorID); err != nil {
		t.Fatalf("force creator onto the CAB stage: %v", err)
	}
	assertForbidden("creator on the CAB stage", crFlowCreatorID)
	if got := f.state(id); got != "AUTHORIZE" {
		t.Fatalf("state after refused creator approvals = %q, want AUTHORIZE (unchanged)", got)
	}

	// The creator may still cancel.
	if _, err := f.patchState(id, domain.ChangeRequestStateCanceled); err != nil {
		t.Fatalf("creator cancelling their own change: %v", err)
	}
	if got := f.state(id); got != "CANCELED" {
		t.Fatalf("state after cancel = %q, want CANCELED", got)
	}
}

// The creator is also recognised through work_item.created_by (their email)
// when change_request.requested_by_user_id was never set.
func TestChangeRequestFlowIntegration_CreatorRecognisedByCreatedByEmail(t *testing.T) {
	f := newCRFlow(t)
	f.seedAssignedGroup()
	seedApprovalGroupMembers(t, f.scoped, crCABGroupID, crCABMemberUserID1)
	id := f.create(domain.ChangeRequestTypeNormal, crFlowGroupID)
	if _, err := f.scoped.Exec(f.sys, `UPDATE change_request SET requested_by_user_id = NULL WHERE id = $1`, id); err != nil {
		t.Fatalf("clear requested_by: %v", err)
	}
	f.requestApproval(id)
	stages := f.stages(id)
	if got := stages[0].approvers[crFlowCreatorID]; got != "cancelled" {
		t.Fatalf("creator (identified only by created_by email) peer row = %q, want cancelled", got)
	}
}

// SRE team members (Apollo/Artemis/any SRE group) are never peer approvers.
func TestChangeRequestFlowIntegration_SREMemberCannotBePeerApprover(t *testing.T) {
	f := newCRFlow(t)
	f.seedAssignedGroup()
	f.makeSRE(crFlowSREID, crFlowGroupID)
	seedApprovalGroupMembers(t, f.scoped, crCABGroupID, crCABMemberUserID1)
	id := f.create(domain.ChangeRequestTypeNormal, crFlowGroupID)
	f.requestApproval(id)

	stages := f.stages(id)
	if _, present := stages[0].approvers[crFlowSREID]; present {
		t.Fatalf("SRE member was provisioned into the peer approval group: %v", stages[0].approvers)
	}

	// Decision time: even a row that exists (drift, or a membership that
	// changed after provisioning) cannot be decided by an SRE member.
	if _, err := f.scoped.Exec(f.sys,
		`INSERT INTO approval_stage_approver (id, created_on, updated_on, created_by, updated_by, stage_id, work_item_id, approver_user_id, status)
		 SELECT gen_random_uuid(), now(), now(), 'cr-flow-test', 'cr-flow-test', s.id, s.work_item_id, $2::uuid, 'requested'
		 FROM approval_stage s WHERE s.work_item_id = $1 AND s.checkpoint_label = 'Peer Approval'`, id, crFlowSREID); err != nil {
		t.Fatalf("force SRE member onto the peer stage: %v", err)
	}
	err := f.decide(id, crFlowSREID, "approved")
	var fe *apierror.ForbiddenError
	if !errors.As(err, &fe) || !strings.Contains(fe.Msg, "SRE") {
		t.Fatalf("SRE member peer decision err = %v (%T), want a ForbiddenError naming the SRE rule", err, err)
	}
	if got := f.state(id); got != "ASSESS" {
		t.Fatalf("state after refused SRE approval = %q, want ASSESS", got)
	}
}

// When the assigned group IS an SRE group (Apollo), nobody in it may approve;
// the peer pool falls back to the "Devops Approval" group of experienced
// engineers. With no such group the request is refused clearly.
func TestChangeRequestFlowIntegration_SREAssignedGroupFallsBackToPeerApprovalGroup(t *testing.T) {
	f := newCRFlow(t)
	f.seedAssignedGroup()
	f.makeSRE(crFlowSREID, crFlowSREGroupID)
	seedApprovalGroupMembers(t, f.scoped, crCABGroupID, crCABMemberUserID1)

	var existing int
	if err := f.scoped.QueryRow(f.sys, `SELECT COUNT(*) FROM "group" WHERE name = $1`, domain.PeerApprovalFallbackGroupName).Scan(&existing); err != nil {
		t.Fatalf("count devops group: %v", err)
	}
	if existing != 0 {
		t.Skipf("a %q group already exists in this database", domain.PeerApprovalFallbackGroupName)
	}

	// No fallback group: refused, nothing half-written.
	id := f.create(domain.ChangeRequestTypeNormal, crFlowSREGroupID)
	_, err := f.patchState(id, domain.ChangeRequestStateAssess)
	var ve *apierror.ValidationError
	if !errors.As(err, &ve) || !strings.Contains(ve.Msg, "no eligible peer approvers") {
		t.Fatalf("Request Approval on an SRE-assigned change with no peer group err = %v (%T), want the no-eligible-peer-approvers ValidationError", err, err)
	}
	if got := f.state(id); got != "NEW" {
		t.Fatalf("state after refused Request Approval = %q, want NEW", got)
	}
	if n := len(f.stages(id)); n != 0 {
		t.Fatalf("refused Request Approval left %d stages", n)
	}

	// With the Devops Approval group (an SRE member sits in it too -- still
	// excluded), the experienced engineers become the peer approvers.
	if _, err := f.scoped.Exec(f.sys,
		`INSERT INTO "group" (id, created_on, updated_on, created_by, updated_by, name) VALUES ($1, now(), now(), 'cr-flow-test', 'cr-flow-test', $2)`,
		crFlowDevopsGroupID, domain.PeerApprovalFallbackGroupName); err != nil {
		t.Fatalf("seed devops group: %v", err)
	}
	seedApprovalGroupMembers(t, f.scoped, crFlowDevopsGroupID, crFlowPeerAID, crFlowPeerBID)
	if _, err := f.scoped.Exec(f.sys,
		`INSERT INTO team_member (id, created_on, updated_on, created_by, updated_by, team_id, user_id, group_id)
		 VALUES (gen_random_uuid(), now(), now(), 'cr-flow-test', 'cr-flow-test', $1::uuid, $2, $3::uuid)`,
		crFlowSRETeamID, crFlowSREID, crFlowDevopsGroupID); err != nil {
		t.Fatalf("seed SRE member in devops group: %v", err)
	}
	f.requestApproval(id)
	stages := f.stages(id)
	if len(stages) != 1 || stages[0].groupID != crFlowDevopsGroupID {
		t.Fatalf("stages = %+v, want one peer stage on the Devops Approval group", stages)
	}
	assertApprovers(t, "fallback peer stage", stages[0].approvers, map[string]string{crFlowPeerAID: "requested", crFlowPeerBID: "requested"})
}

// A Normal change cannot be sent for approval into a flow with nobody to give
// the CAB approval; the request is refused and nothing is written.
func TestChangeRequestFlowIntegration_NormalRequiresEligibleCABApprovers(t *testing.T) {
	f := newCRFlow(t)
	f.seedAssignedGroup()
	id := f.create(domain.ChangeRequestTypeNormal, crFlowGroupID)

	_, err := f.patchState(id, domain.ChangeRequestStateAssess)
	var ve *apierror.ValidationError
	if !errors.As(err, &ve) || !strings.Contains(ve.Msg, domain.CABApprovalGroupName) {
		t.Fatalf("Request Approval with an empty CAB group err = %v (%T), want a ValidationError naming %q", err, err, domain.CABApprovalGroupName)
	}
	if got := f.state(id); got != "NEW" {
		t.Fatalf("state after refused Request Approval = %q, want NEW", got)
	}
	if n := len(f.stages(id)); n != 0 {
		t.Fatalf("refused Request Approval left %d approval stages", n)
	}

	// Same for Emergency and the ECAB group.
	eid := f.create(domain.ChangeRequestTypeEmergency, crFlowGroupID)
	_, err = f.patchState(eid, domain.ChangeRequestStateAssess)
	if !errors.As(err, &ve) || !strings.Contains(ve.Msg, domain.ECABApprovalGroupName) {
		t.Fatalf("emergency Request Approval with an empty ECAB group err = %v (%T), want a ValidationError naming %q", err, err, domain.ECABApprovalGroupName)
	}

	// A CAB group whose only member is the creator is also a dead end.
	seedApprovalGroupMembers(t, f.scoped, crCABGroupID, crFlowCreatorID)
	if _, err = f.patchState(id, domain.ChangeRequestStateAssess); err == nil {
		t.Fatal("Request Approval succeeded although the only CAB member is the creator")
	}
}

// CAB approval must not be able to strand a change: if the CAB group has lost
// its members by the time the peer approves, the peer's decision is refused
// (rolled back) instead of advancing to an Authorize nobody can decide.
func TestChangeRequestFlowIntegration_PeerApprovalRefusedWhenCABGroupEmptied(t *testing.T) {
	f := newCRFlow(t)
	f.seedAssignedGroup()
	seedApprovalGroupMembers(t, f.scoped, crCABGroupID, crCABMemberUserID1)
	id := f.create(domain.ChangeRequestTypeNormal, crFlowGroupID)
	f.requestApproval(id)
	if _, err := f.scoped.Exec(f.sys, `DELETE FROM team_member WHERE user_id = $1`, crCABMemberUserID1); err != nil {
		t.Fatalf("empty the CAB group: %v", err)
	}

	err := f.decide(id, crFlowPeerAID, "approved")
	var ve *apierror.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("peer approval into an empty CAB group err = %v (%T), want *apierror.ValidationError", err, err)
	}
	if got := f.state(id); got != "ASSESS" {
		t.Fatalf("state after refused peer approval = %q, want ASSESS (rolled back)", got)
	}
	stages := f.stages(id)
	if len(stages) != 1 || stages[0].approvers[crFlowPeerAID] != "requested" {
		t.Fatalf("stages after rolled-back approval = %+v, want the peer still pending", stages)
	}
}

// A CAB/ECAB rejection keeps the existing rejection behaviour: siblings are
// cancelled and the state is NOT advanced (nor rolled back).
func TestChangeRequestFlowIntegration_CABRejectionDoesNotSchedule(t *testing.T) {
	f := newCRFlow(t)
	f.seedAssignedGroup()
	seedApprovalGroupMembers(t, f.scoped, crCABGroupID, crCABMemberUserID1, crCABMemberUserID2)
	id := f.create(domain.ChangeRequestTypeNormal, crFlowGroupID)
	f.requestApproval(id)
	if err := f.decide(id, crFlowPeerAID, "approved"); err != nil {
		t.Fatalf("peer approval: %v", err)
	}
	if err := f.decide(id, crCABMemberUserID1, "rejected"); err != nil {
		t.Fatalf("CAB rejection: %v", err)
	}
	if got := f.state(id); got != "AUTHORIZE" {
		t.Fatalf("state after CAB rejection = %q, want AUTHORIZE (no cascade)", got)
	}
	stages := f.stages(id)
	assertApprovers(t, "CAB stage after rejection", stages[1].approvers, map[string]string{crCABMemberUserID1: "rejected", crCABMemberUserID2: "cancelled"})
}

// The type cannot be changed once approval has been requested: the stages
// provisioned belong to the type they were provisioned for.
func TestChangeRequestFlowIntegration_TypeLockedAfterApprovalRequested(t *testing.T) {
	f := newCRFlow(t)
	f.seedAssignedGroup()
	seedApprovalGroupMembers(t, f.scoped, crCABGroupID, crCABMemberUserID1)
	id := f.create(domain.ChangeRequestTypeNormal, crFlowGroupID)

	// Before approval is requested the type may still be corrected.
	std := domain.ChangeRequestTypeStandard
	if _, err := f.repo.PatchChangeRequest(f.sys, id, domain.PatchChangeRequestRequest{Type: &std}, "x@example.com"); err != nil {
		t.Fatalf("changing the type on a New change: %v", err)
	}
	nrm := domain.ChangeRequestTypeNormal
	if _, err := f.repo.PatchChangeRequest(f.sys, id, domain.PatchChangeRequestRequest{Type: &nrm}, "x@example.com"); err != nil {
		t.Fatalf("changing the type back: %v", err)
	}
	f.requestApproval(id)
	emg := domain.ChangeRequestTypeEmergency
	_, err := f.repo.PatchChangeRequest(f.sys, id, domain.PatchChangeRequestRequest{Type: &emg}, "x@example.com")
	var ve *apierror.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("changing the type after Request Approval err = %v (%T), want *apierror.ValidationError", err, err)
	}
}

// canDecide is true only on the viewer's own pending row, and only when they
// may actually decide it.
func TestChangeRequestFlowIntegration_CanDecide(t *testing.T) {
	f := newCRFlow(t)
	f.seedAssignedGroup()
	f.makeSRE(crFlowSREID, crFlowGroupID)
	seedApprovalGroupMembers(t, f.scoped, crCABGroupID, crCABMemberUserID1)
	id := f.create(domain.ChangeRequestTypeNormal, crFlowGroupID)
	f.requestApproval(id)
	// Drift: rows for the creator and the SRE member exist as requested.
	for _, uid := range []string{crFlowCreatorID, crFlowSREID} {
		if _, err := f.scoped.Exec(f.sys,
			`DELETE FROM approval_stage_approver WHERE work_item_id = $1 AND approver_user_id = $2`, id, uid); err != nil {
			t.Fatalf("reset row: %v", err)
		}
		if _, err := f.scoped.Exec(f.sys,
			`INSERT INTO approval_stage_approver (id, created_on, updated_on, created_by, updated_by, stage_id, work_item_id, approver_user_id, status)
			 SELECT gen_random_uuid(), now(), now(), 'cr-flow-test', 'cr-flow-test', s.id, s.work_item_id, $2::uuid, 'requested'
			 FROM approval_stage s WHERE s.work_item_id = $1`, id, uid); err != nil {
			t.Fatalf("seed row: %v", err)
		}
	}

	canDecide := func(viewerID string) map[string]bool {
		t.Helper()
		ctx := repository.WithCallerIdentity(context.Background(), repository.SearchScope{Unrestricted: true, ViewerEmail: crFlowEmail(viewerID)})
		got, err := f.repo.GetChangeRequestApprovals(ctx, id)
		if err != nil {
			t.Fatalf("GetChangeRequestApprovals: %v", err)
		}
		out := map[string]bool{}
		for _, a := range got.Approvals {
			for _, ap := range a.Approvers {
				if ap.CanDecide {
					out[ap.ID] = true
				}
			}
		}
		return out
	}

	if got := canDecide(crFlowPeerAID); len(got) != 1 || !got[crFlowPeerAID] {
		t.Fatalf("canDecide for a peer = %v, want only their own row", got)
	}
	if got := canDecide(crFlowCreatorID); len(got) != 0 {
		t.Fatalf("canDecide for the creator = %v, want none", got)
	}
	if got := canDecide(crFlowSREID); len(got) != 0 {
		t.Fatalf("canDecide for an SRE member = %v, want none", got)
	}
	if got := canDecide(crCABMemberUserID1); len(got) != 0 {
		t.Fatalf("canDecide for a CAB member while still in Assess = %v, want none (no CAB row yet)", got)
	}
	// System identity carries no viewer: never true.
	sysGot, err := f.repo.GetChangeRequestApprovals(f.sys, id)
	if err != nil {
		t.Fatalf("GetChangeRequestApprovals(system): %v", err)
	}
	for _, a := range sysGot.Approvals {
		for _, ap := range a.Approvers {
			if ap.CanDecide {
				t.Fatalf("canDecide set without a viewer identity: %+v", ap)
			}
		}
	}

	if err := f.decide(id, crFlowPeerAID, "approved"); err != nil {
		t.Fatalf("peer approval: %v", err)
	}
	if got := canDecide(crFlowPeerAID); len(got) != 0 {
		t.Fatalf("canDecide for the peer after deciding = %v, want none", got)
	}
	if got := canDecide(crCABMemberUserID1); len(got) != 1 || !got[crCABMemberUserID1] {
		t.Fatalf("canDecide for a CAB member on the CAB stage = %v, want their own row", got)
	}
}

// The CAB Approval and ECAB Approval groups exist after the migrations, and
// re-running the migration neither fails nor duplicates them.
func TestChangeRequestFlowIntegration_ApprovalGroupsExistAndMigrationIsIdempotent(t *testing.T) {
	f := newCRFlow(t)
	for _, name := range []string{domain.CABApprovalGroupName, domain.ECABApprovalGroupName} {
		var n int
		if err := f.scoped.QueryRow(f.sys, `SELECT COUNT(*) FROM "group" WHERE name = $1`, name).Scan(&n); err != nil {
			t.Fatalf("count %q: %v", name, err)
		}
		if n < 1 {
			t.Fatalf("group %q does not exist after migrations", name)
		}
	}
	var cabID, ecabID string
	if err := f.scoped.QueryRow(f.sys, `SELECT id::text FROM "group" WHERE name = $1 ORDER BY created_on LIMIT 1`, domain.CABApprovalGroupName).Scan(&cabID); err != nil {
		t.Fatalf("read CAB group: %v", err)
	}
	if err := f.scoped.QueryRow(f.sys, `SELECT id::text FROM "group" WHERE name = $1 ORDER BY created_on LIMIT 1`, domain.ECABApprovalGroupName).Scan(&ecabID); err != nil {
		t.Fatalf("read ECAB group: %v", err)
	}
	if cabID == ecabID {
		t.Fatal("CAB Approval and ECAB Approval are the same group; they must be separate groups")
	}

	sqlBytes, err := os.ReadFile("../../migrations/0188_change_request_approval_groups.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	var before, after []string
	collect := func(into *[]string) {
		rows, err := f.scoped.Query(f.sys, `SELECT id::text FROM "group" WHERE name IN ($1, $2) ORDER BY id`, domain.CABApprovalGroupName, domain.ECABApprovalGroupName)
		if err != nil {
			t.Fatalf("list groups: %v", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				t.Fatalf("scan: %v", err)
			}
			*into = append(*into, id)
		}
	}
	collect(&before)
	for i := 0; i < 2; i++ {
		if _, err := f.pool.Exec(context.Background(), string(sqlBytes)); err != nil {
			t.Fatalf("re-running migration 0188 (pass %d): %v", i+1, err)
		}
	}
	collect(&after)
	sort.Strings(before)
	sort.Strings(after)
	if strings.Join(before, ",") != strings.Join(after, ",") {
		t.Fatalf("re-running the migration changed the groups: before %v, after %v", before, after)
	}
}

// A type is mandatory on create, and only standard/normal/emergency are
// allowed -- in both Postgres create paths (portal and ServiceNow-first).
func TestChangeRequestFlowIntegration_CreateRequiresCreatableType(t *testing.T) {
	f := newCRFlow(t)
	var ve *apierror.ValidationError

	if _, err := f.repo.CreateChangeRequest(f.sys, domain.CreateChangeRequestRequest{Subject: crFlowSubject}, "x@example.com"); !errors.As(err, &ve) || !strings.Contains(ve.Msg, "type is required") {
		t.Fatalf("CreateChangeRequest without a type err = %v (%T), want a \"type is required\" ValidationError", err, err)
	}
	if _, err := f.repo.CreateChangeRequestFromServiceNow(f.sys, domain.CreateChangeRequestRequest{Subject: crFlowSubject}, "3aaaaaaa-0000-0000-0000-0000000000f1", "CRFLOWSN001", "x@example.com"); !errors.As(err, &ve) || !strings.Contains(ve.Msg, "type is required") {
		t.Fatalf("CreateChangeRequestFromServiceNow without a type err = %v (%T), want a \"type is required\" ValidationError", err, err)
	}
	azure := domain.ChangeRequestTypeAzure
	if _, err := f.repo.CreateChangeRequest(f.sys, domain.CreateChangeRequestRequest{Subject: crFlowSubject, Type: &azure}, "x@example.com"); !errors.As(err, &ve) {
		t.Fatalf("CreateChangeRequest with type azure err = %v (%T), want a ValidationError", err, err)
	}

	for _, typ := range domain.ChangeRequestCreatableTypes {
		typ := typ
		resp, err := f.repo.CreateChangeRequest(f.sys, domain.CreateChangeRequestRequest{Subject: crFlowSubject, Type: &typ}, "x@example.com")
		if err != nil {
			t.Fatalf("CreateChangeRequest(%s): %v", typ, err)
		}
		cr, err := f.repo.GetChangeRequestByID(f.sys, resp.ChangeRequest.ID)
		if err != nil {
			t.Fatalf("GetChangeRequestByID: %v", err)
		}
		if cr.Type == nil || !strings.EqualFold(*cr.Type, string(typ)) {
			t.Fatalf("created type = %v, want %q", cr.Type, typ)
		}
	}
}

// ---------------------------------------------------------------------------
// Customer Approval / Customer Review checkboxes
// (change_request.customer_approval_required / customer_review_required,
// migration 0189). They add an optional customer step on each side of the
// implementation: CAB / ECAB approval (or Request Approval on a Standard
// change) moves the change to Customer Approval instead of Scheduled, where a
// human records the customer's approval; Review offers Customer Review (then
// Closed) instead of Closed.
// ---------------------------------------------------------------------------

func boolp(b bool) *bool { return &b }

// createGated is crFlow.create with the creation form's two checkboxes.
func (f *crFlow) createGated(typ domain.ChangeRequestType, groupID string, approval, review *bool) string {
	f.t.Helper()
	g := groupID
	resp, err := f.repo.CreateChangeRequest(f.sys, domain.CreateChangeRequestRequest{
		Subject: crFlowSubject, Type: &typ, GroupID: &g,
		CustomerApprovalRequired: approval, CustomerReviewRequired: review,
	}, crFlowEmail(crFlowCreatorID))
	if err != nil {
		f.t.Fatalf("CreateChangeRequest(%s): %v", typ, err)
	}
	if _, err := f.scoped.Exec(f.sys, `UPDATE change_request SET requested_by_user_id = $1::uuid WHERE id = $2`, crFlowCreatorID, resp.ChangeRequest.ID); err != nil {
		f.t.Fatalf("set requested_by: %v", err)
	}
	return resp.ChangeRequest.ID
}

func (f *crFlow) patch(id string, req domain.PatchChangeRequestRequest) (domain.ChangeRequest, error) {
	return f.repo.PatchChangeRequest(f.sys, id, req, crFlowEmail(crFlowCreatorID))
}

func (f *crFlow) get(id string) domain.ChangeRequest {
	f.t.Helper()
	cr, err := f.repo.GetChangeRequestByID(f.sys, id)
	if err != nil {
		f.t.Fatalf("GetChangeRequestByID: %v", err)
	}
	return cr
}

// step PATCHes a state and asserts the resulting stored state and legalNextStates.
func (f *crFlow) step(id string, to domain.ChangeRequestState, wantState string, wantLegal ...string) {
	f.t.Helper()
	if _, err := f.patchState(id, to); err != nil {
		f.t.Fatalf("PATCH {state: %s}: %v", to, err)
	}
	f.expect(id, "after PATCH "+string(to), wantState, wantLegal...)
}

// expect asserts the stored state and legalNextStates.
func (f *crFlow) expect(id, when, wantState string, wantLegal ...string) {
	f.t.Helper()
	if got := f.state(id); got != wantState {
		f.t.Fatalf("state %s = %q, want %q", when, got, wantState)
	}
	assertStates(f.t, "legalNextStates "+when, f.legal(id), wantLegal...)
}

func (f *crFlow) customerOutcome(id string) (approved, reviewed bool) {
	f.t.Helper()
	cr := f.get(id)
	return cr.HasCustomerApproved, cr.HasCustomerReviewed
}

func (f *crFlow) wantValidationError(what string, err error, contains string) {
	f.t.Helper()
	var ve *apierror.ValidationError
	if !errors.As(err, &ve) {
		f.t.Fatalf("%s: err = %v (%T), want *apierror.ValidationError", what, err, err)
	}
	if !strings.Contains(ve.Msg, contains) {
		f.t.Fatalf("%s: message %q should contain %q", what, ve.Msg, contains)
	}
}

// approvePeerAndCAB drives a Normal change from Assess through peer approval
// and CAB approval, asserting the state after each step; wantAfterCAB is where
// CAB approval must leave the change.
func (f *crFlow) approvePeerAndCAB(id, wantAfterCAB string, wantLegalAfterCAB ...string) {
	f.t.Helper()
	if err := f.decide(id, crFlowPeerAID, "approved"); err != nil {
		f.t.Fatalf("peer approval: %v", err)
	}
	f.expect(id, "after peer approval", "AUTHORIZE", "canceled")
	if err := f.decide(id, crCABMemberUserID1, "approved"); err != nil {
		f.t.Fatalf("CAB approval: %v", err)
	}
	f.expect(id, "after CAB approval", wantAfterCAB, wantLegalAfterCAB...)
}

// Normal x {Customer Approval ticked, unticked} x {Customer Review ticked,
// unticked}: the state, legalNextStates and the recorded customer outcome after
// EVERY step.
func TestChangeRequestFlowIntegration_NormalCustomerGateLifecycles(t *testing.T) {
	for _, tc := range []struct {
		name             string
		approval, review bool
	}{
		{"approval-unticked/review-unticked", false, false},
		{"approval-ticked/review-unticked", true, false},
		{"approval-unticked/review-ticked", false, true},
		{"approval-ticked/review-ticked", true, true},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			f := newCRFlow(t)
			f.seedAssignedGroup()
			seedApprovalGroupMembers(t, f.scoped, crCABGroupID, crCABMemberUserID1, crCABMemberUserID2)
			id := f.createGated(domain.ChangeRequestTypeNormal, crFlowGroupID, boolp(tc.approval), boolp(tc.review))

			// The checkboxes are stored and read back as ticked.
			cr := f.get(id)
			if cr.CustomerApprovalRequired != tc.approval || cr.CustomerReviewRequired != tc.review {
				t.Fatalf("created flags read back = %v/%v, want %v/%v", cr.CustomerApprovalRequired, cr.CustomerReviewRequired, tc.approval, tc.review)
			}
			f.expect(id, "after create", "NEW", "assess", "canceled")

			f.requestApproval(id)
			f.expect(id, "after Request Approval", "ASSESS", "authorize", "canceled")

			// Both gates sit AFTER the internal approvals: nothing short-circuits them.
			f.approvePeerAndCAB(id, map[bool]string{true: "CUSTOMER_APPROVAL", false: "SCHEDULED"}[tc.approval],
				map[bool][]string{true: {"scheduled", "canceled"}, false: {"implement", "canceled"}}[tc.approval]...)
			if approved, _ := f.customerOutcome(id); approved {
				t.Fatal("is_customer_approved is already true before the customer approved anything")
			}

			if tc.approval {
				// The customer's approval is recorded by the human action
				// "scheduled", which is legal only here.
				f.step(id, domain.ChangeRequestStateScheduled, "SCHEDULED", "implement", "canceled")
				if approved, _ := f.customerOutcome(id); !approved {
					t.Fatal("is_customer_approved = false after recording the customer's approval, want true")
				}
			} else if approved, _ := f.customerOutcome(id); approved {
				t.Fatal("is_customer_approved = true although no customer approval was required or given")
			}

			f.step(id, domain.ChangeRequestStateImplement, "IMPLEMENT", "review", "canceled")
			if tc.review {
				f.step(id, domain.ChangeRequestStateReview, "REVIEW", "customer_review", "canceled")
				// Review cannot close directly when the customer's review is required.
				_, err := f.patchState(id, domain.ChangeRequestStateClosed)
				f.wantValidationError("closed from review (customer review required)", err, "customer review is required")
				f.expect(id, "after refused close", "REVIEW", "customer_review", "canceled")
				f.step(id, domain.ChangeRequestStateCustomerReview, "CUSTOMER_REVIEW", "closed", "canceled")
				if _, reviewed := f.customerOutcome(id); reviewed {
					t.Fatal("is_customer_reviewed is already true before the customer review was recorded")
				}
				f.step(id, domain.ChangeRequestStateClosed, "CLOSED")
				if _, reviewed := f.customerOutcome(id); !reviewed {
					t.Fatal("is_customer_reviewed = false after closing from customer_review, want true")
				}
			} else {
				f.step(id, domain.ChangeRequestStateReview, "REVIEW", "closed", "canceled")
				// Customer Review is not offered, and not accepted, when not required.
				_, err := f.patchState(id, domain.ChangeRequestStateCustomerReview)
				f.wantValidationError("customer_review (not required)", err, "customer review is not required")
				f.expect(id, "after refused customer_review", "REVIEW", "closed", "canceled")
				f.step(id, domain.ChangeRequestStateClosed, "CLOSED")
				if _, reviewed := f.customerOutcome(id); reviewed {
					t.Fatal("is_customer_reviewed = true although no customer review was required or given")
				}
			}
			if approved, _ := f.customerOutcome(id); approved != tc.approval {
				t.Fatalf("is_customer_approved at the end = %v, want %v", approved, tc.approval)
			}
			// The two internal approval stages (peer, CAB) are untouched by the
			// customer gates: Customer Approval is a state, not an approval stage.
			if n := len(f.stages(id)); n != 3 {
				t.Fatalf("stages at the end = %d, want 3 (peer, CAB, review)", n)
			}
		})
	}
}

// Emergency with Customer Approval ticked: ECAB approval moves the change to
// Customer Approval (not Scheduled); recording the customer's approval then
// schedules it. Unticked is covered by EmergencyLifecycle.
func TestChangeRequestFlowIntegration_EmergencyCustomerApprovalLifecycle(t *testing.T) {
	f := newCRFlow(t)
	f.seedAssignedGroup()
	seedApprovalGroupMembers(t, f.scoped, crECABGroupID, crECABMemberUserID)
	id := f.createGated(domain.ChangeRequestTypeEmergency, crFlowGroupID, boolp(true), nil)

	f.expect(id, "after create", "NEW", "assess", "canceled")
	f.requestApproval(id)
	f.expect(id, "after Request Approval", "AUTHORIZE", "canceled")
	if stages := f.stages(id); len(stages) != 1 || stages[0].label != "ECAB Approval" {
		t.Fatalf("stages = %+v, want only the ECAB stage", stages)
	}

	// The gate is NOT reached by Request Approval for an Emergency change: the
	// ECAB approval still comes first.
	if err := f.decide(id, crECABMemberUserID, "approved"); err != nil {
		t.Fatalf("ECAB approval: %v", err)
	}
	f.expect(id, "after ECAB approval", "CUSTOMER_APPROVAL", "scheduled", "canceled")
	if approved, _ := f.customerOutcome(id); approved {
		t.Fatal("is_customer_approved is true before the customer's approval was recorded")
	}

	f.step(id, domain.ChangeRequestStateScheduled, "SCHEDULED", "implement", "canceled")
	if approved, _ := f.customerOutcome(id); !approved {
		t.Fatal("is_customer_approved = false after recording the customer's approval")
	}
	f.step(id, domain.ChangeRequestStateImplement, "IMPLEMENT", "review", "canceled")
	f.step(id, domain.ChangeRequestStateReview, "REVIEW", "closed", "canceled") // review not ticked: straight to Closed
	f.step(id, domain.ChangeRequestStateClosed, "CLOSED")
}

// Standard with Customer Approval ticked: it has no internal approval to wait
// for, so Request Approval itself lands in Customer Approval (the assumption
// recorded in the PR), with no approval stage at all.
func TestChangeRequestFlowIntegration_StandardCustomerApprovalLifecycle(t *testing.T) {
	f := newCRFlow(t)
	f.seedAssignedGroup()
	id := f.createGated(domain.ChangeRequestTypeStandard, crFlowGroupID, boolp(true), boolp(true))

	f.expect(id, "after create", "NEW", "assess", "canceled")
	f.requestApproval(id)
	f.expect(id, "after Request Approval", "CUSTOMER_APPROVAL", "scheduled", "canceled")
	if n := len(f.stages(id)); n != 0 {
		t.Fatalf("standard change has %d approval stages, want none", n)
	}
	// A resend of Request Approval while it waits for the customer is idempotent.
	if _, err := f.patchState(id, domain.ChangeRequestStateAssess); err != nil {
		t.Fatalf("resent Request Approval: %v", err)
	}
	f.expect(id, "after resent Request Approval", "CUSTOMER_APPROVAL", "scheduled", "canceled")

	f.step(id, domain.ChangeRequestStateScheduled, "SCHEDULED", "implement", "canceled")
	if approved, _ := f.customerOutcome(id); !approved {
		t.Fatal("is_customer_approved = false after recording the customer's approval")
	}
	f.step(id, domain.ChangeRequestStateImplement, "IMPLEMENT", "review", "canceled")
	f.step(id, domain.ChangeRequestStateReview, "REVIEW", "customer_review", "canceled")
	f.step(id, domain.ChangeRequestStateCustomerReview, "CUSTOMER_REVIEW", "closed", "canceled")
	f.step(id, domain.ChangeRequestStateClosed, "CLOSED")
	if _, reviewed := f.customerOutcome(id); !reviewed {
		t.Fatal("is_customer_reviewed = false after closing from customer_review")
	}
}

// Ticking Customer Approval together with Request Approval (one PATCH) on a
// Standard change routes it to Customer Approval; so does ticking it in a
// separate PATCH before Request Approval.
func TestChangeRequestFlowIntegration_StandardCustomerApprovalTickedWithRequestApproval(t *testing.T) {
	f := newCRFlow(t)
	f.seedAssignedGroup()

	together := f.create(domain.ChangeRequestTypeStandard, crFlowGroupID)
	st := domain.ChangeRequestStateAssess
	if _, err := f.patch(together, domain.PatchChangeRequestRequest{State: &st, CustomerApprovalRequired: boolp(true)}); err != nil {
		t.Fatalf("PATCH {state: assess, customerApprovalRequired: true}: %v", err)
	}
	f.expect(together, "after the combined PATCH", "CUSTOMER_APPROVAL", "scheduled", "canceled")

	separate := f.create(domain.ChangeRequestTypeStandard, crFlowGroupID)
	if _, err := f.patch(separate, domain.PatchChangeRequestRequest{CustomerApprovalRequired: boolp(true)}); err != nil {
		t.Fatalf("PATCH {customerApprovalRequired: true}: %v", err)
	}
	f.expect(separate, "after ticking the box", "NEW", "assess", "canceled")
	f.requestApproval(separate)
	f.expect(separate, "after Request Approval", "CUSTOMER_APPROVAL", "scheduled", "canceled")
}

// The checkboxes are accepted on create (both create paths) and PATCH, and
// returned on the detail response.
func TestChangeRequestFlowIntegration_CustomerGateFlagsAcceptedAndReturned(t *testing.T) {
	f := newCRFlow(t)
	f.seedAssignedGroup()

	// Omitted on create: both false.
	plain := f.create(domain.ChangeRequestTypeNormal, crFlowGroupID)
	if cr := f.get(plain); cr.CustomerApprovalRequired || cr.CustomerReviewRequired {
		t.Fatalf("flags after a create that omitted them = %v/%v, want false/false", cr.CustomerApprovalRequired, cr.CustomerReviewRequired)
	}

	// Ticked on the portal create path.
	both := f.createGated(domain.ChangeRequestTypeNormal, crFlowGroupID, boolp(true), boolp(true))
	if cr := f.get(both); !cr.CustomerApprovalRequired || !cr.CustomerReviewRequired {
		t.Fatalf("flags after a create that ticked both = %v/%v, want true/true", cr.CustomerApprovalRequired, cr.CustomerReviewRequired)
	}

	// Ticked on the ServiceNow-first create path.
	typ := domain.ChangeRequestTypeNormal
	resp, err := f.repo.CreateChangeRequestFromServiceNow(f.sys, domain.CreateChangeRequestRequest{
		Subject: crFlowSubject, Type: &typ, CustomerApprovalRequired: boolp(true), CustomerReviewRequired: boolp(false),
	}, "3aaaaaaa-0000-0000-0000-0000000000f2", "CRFLOWSN002", "x@example.com")
	if err != nil {
		t.Fatalf("CreateChangeRequestFromServiceNow: %v", err)
	}
	if cr := f.get(resp.ChangeRequest.ID); !cr.CustomerApprovalRequired || cr.CustomerReviewRequired {
		t.Fatalf("flags after a ServiceNow-first create = %v/%v, want true/false", cr.CustomerApprovalRequired, cr.CustomerReviewRequired)
	}

	// PATCH sets them, returns them in the receipt, and a PATCH touching only
	// one leaves the other alone.
	cr, err := f.patch(plain, domain.PatchChangeRequestRequest{CustomerApprovalRequired: boolp(true)})
	if err != nil {
		t.Fatalf("PATCH customerApprovalRequired: %v", err)
	}
	if !cr.CustomerApprovalRequired || cr.CustomerReviewRequired {
		t.Fatalf("receipt flags = %v/%v, want true/false", cr.CustomerApprovalRequired, cr.CustomerReviewRequired)
	}
	cr, err = f.patch(plain, domain.PatchChangeRequestRequest{CustomerReviewRequired: boolp(true)})
	if err != nil {
		t.Fatalf("PATCH customerReviewRequired: %v", err)
	}
	if !cr.CustomerApprovalRequired || !cr.CustomerReviewRequired {
		t.Fatalf("receipt flags = %v/%v, want true/true", cr.CustomerApprovalRequired, cr.CustomerReviewRequired)
	}
	cr, err = f.patch(plain, domain.PatchChangeRequestRequest{CustomerApprovalRequired: boolp(false), CustomerReviewRequired: boolp(false)})
	if err != nil {
		t.Fatalf("PATCH both off: %v", err)
	}
	if cr.CustomerApprovalRequired || cr.CustomerReviewRequired {
		t.Fatalf("receipt flags = %v/%v, want false/false", cr.CustomerApprovalRequired, cr.CustomerReviewRequired)
	}
	if got := f.get(plain); got.CustomerApprovalRequired || got.CustomerReviewRequired {
		t.Fatalf("stored flags = %v/%v, want false/false", got.CustomerApprovalRequired, got.CustomerReviewRequired)
	}
}

// customerApprovalRequired is editable while the change is New, Assess or
// Authorize -- and what it is at CAB approval decides the cascade. After that
// the gate has been passed and an edit is refused with a clear 400.
func TestChangeRequestFlowIntegration_CustomerApprovalRequiredEditableUntilGatePassed(t *testing.T) {
	f := newCRFlow(t)
	f.seedAssignedGroup()
	seedApprovalGroupMembers(t, f.scoped, crCABGroupID, crCABMemberUserID1)
	id := f.create(domain.ChangeRequestTypeNormal, crFlowGroupID)

	set := func(v bool) error {
		_, err := f.patch(id, domain.PatchChangeRequestRequest{CustomerApprovalRequired: boolp(v)})
		return err
	}
	if err := set(true); err != nil {
		t.Fatalf("tick in New: %v", err)
	}
	f.requestApproval(id)
	if err := set(false); err != nil {
		t.Fatalf("untick in Assess: %v", err)
	}
	if err := set(true); err != nil {
		t.Fatalf("tick in Assess: %v", err)
	}
	if err := f.decide(id, crFlowPeerAID, "approved"); err != nil {
		t.Fatalf("peer approval: %v", err)
	}
	f.expect(id, "after peer approval", "AUTHORIZE", "canceled")
	if err := set(false); err != nil {
		t.Fatalf("untick in Authorize: %v", err)
	}
	if err := set(true); err != nil {
		t.Fatalf("tick in Authorize: %v", err)
	}

	// The value at CAB approval decides: ticked -> Customer Approval.
	if err := f.decide(id, crCABMemberUserID1, "approved"); err != nil {
		t.Fatalf("CAB approval: %v", err)
	}
	f.expect(id, "after CAB approval", "CUSTOMER_APPROVAL", "scheduled", "canceled")

	// The gate has been passed: refused, state and flag unchanged.
	f.wantValidationError("untick in Customer Approval", set(false), "customerApprovalRequired can no longer be changed")
	if !f.get(id).CustomerApprovalRequired {
		t.Fatal("customerApprovalRequired changed although the edit was refused")
	}
	f.step(id, domain.ChangeRequestStateScheduled, "SCHEDULED", "implement", "canceled")
	f.wantValidationError("untick in Scheduled", set(false), "customerApprovalRequired can no longer be changed")
	// Resending the stored value is not an edit.
	if err := set(true); err != nil {
		t.Fatalf("resending the stored value after the gate: %v", err)
	}
	// ... and the refusal is atomic: nothing else in the same PATCH was written.
	title := "should not be written"
	_, err := f.patch(id, domain.PatchChangeRequestRequest{Title: &title, CustomerApprovalRequired: boolp(false)})
	f.wantValidationError("title + untick in Scheduled", err, "can no longer be changed")
	var subject string
	if err := f.scoped.QueryRow(f.sys, `SELECT subject FROM work_item WHERE id = $1`, id).Scan(&subject); err != nil {
		t.Fatalf("read subject: %v", err)
	}
	if subject != crFlowSubject {
		t.Fatalf("subject = %q after a refused PATCH, want it untouched (%q)", subject, crFlowSubject)
	}

	// Unticked at CAB approval: straight to Scheduled.
	seedApprovalGroupMembers(t, f.scoped, crCABGroupID, crCABMemberUserID1)
	id2 := f.create(domain.ChangeRequestTypeNormal, crFlowGroupID)
	if err := func() error {
		_, err := f.patch(id2, domain.PatchChangeRequestRequest{CustomerApprovalRequired: boolp(true)})
		return err
	}(); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if _, err := f.patch(id2, domain.PatchChangeRequestRequest{CustomerApprovalRequired: boolp(false)}); err != nil {
		t.Fatalf("untick: %v", err)
	}
	f.requestApproval(id2)
	f.approvePeerAndCAB(id2, "SCHEDULED", "implement", "canceled")
}

// customerReviewRequired is editable until the change leaves Review -- it
// decides what Review offers -- and refused afterwards.
func TestChangeRequestFlowIntegration_CustomerReviewRequiredEditableUntilReviewLeft(t *testing.T) {
	f := newCRFlow(t)
	f.seedAssignedGroup()
	id := f.create(domain.ChangeRequestTypeStandard, crFlowGroupID)
	set := func(v bool) error {
		_, err := f.patch(id, domain.PatchChangeRequestRequest{CustomerReviewRequired: boolp(v)})
		return err
	}
	for _, step := range []struct {
		state domain.ChangeRequestState // "" = stay
	}{{""}, {domain.ChangeRequestStateAssess}, {domain.ChangeRequestStateImplement}} {
		if step.state != "" {
			if _, err := f.patchState(id, step.state); err != nil {
				t.Fatalf("PATCH state %s: %v", step.state, err)
			}
		}
		if err := set(true); err != nil {
			t.Fatalf("tick in %s: %v", f.state(id), err)
		}
		if err := set(false); err != nil {
			t.Fatalf("untick in %s: %v", f.state(id), err)
		}
	}

	f.step(id, domain.ChangeRequestStateReview, "REVIEW", "closed", "canceled")
	// Still editable in Review, and it flips what Review offers.
	if err := set(true); err != nil {
		t.Fatalf("tick in Review: %v", err)
	}
	f.expect(id, "after ticking in Review", "REVIEW", "customer_review", "canceled")
	if err := set(false); err != nil {
		t.Fatalf("untick in Review: %v", err)
	}
	f.expect(id, "after unticking in Review", "REVIEW", "closed", "canceled")
	if err := set(true); err != nil {
		t.Fatalf("tick in Review (again): %v", err)
	}

	f.step(id, domain.ChangeRequestStateCustomerReview, "CUSTOMER_REVIEW", "closed", "canceled")
	f.wantValidationError("untick in Customer Review", set(false), "customerReviewRequired can no longer be changed")
	if err := set(true); err != nil {
		t.Fatalf("resending the stored value: %v", err)
	}
	f.step(id, domain.ChangeRequestStateClosed, "CLOSED")
	f.wantValidationError("untick in Closed", set(false), "customerReviewRequired can no longer be changed")
	f.wantValidationError("tick approval in Closed", func() error {
		_, err := f.patch(id, domain.PatchChangeRequestRequest{CustomerApprovalRequired: boolp(true)})
		return err
	}(), "customerApprovalRequired can no longer be changed")
	if !f.get(id).CustomerReviewRequired {
		t.Fatal("customerReviewRequired changed although the edit was refused")
	}
}

// Unticking Customer Review in the same PATCH that closes a Review lets the
// close through; closing a required Review without unticking is refused.
func TestChangeRequestFlowIntegration_CloseFromReviewHonoursTheFlagInTheSamePatch(t *testing.T) {
	f := newCRFlow(t)
	f.seedAssignedGroup()
	id := f.createGated(domain.ChangeRequestTypeStandard, crFlowGroupID, nil, boolp(true))
	f.requestApproval(id)
	f.step(id, domain.ChangeRequestStateImplement, "IMPLEMENT", "review", "canceled")
	f.step(id, domain.ChangeRequestStateReview, "REVIEW", "customer_review", "canceled")

	closed := domain.ChangeRequestStateClosed
	_, err := f.patch(id, domain.PatchChangeRequestRequest{State: &closed})
	f.wantValidationError("close a review that requires the customer", err, "customer review is required")
	f.expect(id, "after the refused close", "REVIEW", "customer_review", "canceled")

	if _, err := f.patch(id, domain.PatchChangeRequestRequest{State: &closed, CustomerReviewRequired: boolp(false)}); err != nil {
		t.Fatalf("PATCH {state: closed, customerReviewRequired: false}: %v", err)
	}
	f.expect(id, "after closing with the box unticked", "CLOSED")
	if _, reviewed := f.customerOutcome(id); reviewed {
		t.Fatal("is_customer_reviewed = true although the change closed straight from Review")
	}
}

// Manual {state: scheduled} is legal ONLY from Customer Approval; from every
// other state it is refused and nothing changes.
func TestChangeRequestFlowIntegration_ManualScheduledOnlyFromCustomerApproval(t *testing.T) {
	f := newCRFlow(t)
	f.seedAssignedGroup()
	seedApprovalGroupMembers(t, f.scoped, crCABGroupID, crCABMemberUserID1)
	normal := f.create(domain.ChangeRequestTypeNormal, crFlowGroupID)
	std := f.create(domain.ChangeRequestTypeStandard, crFlowGroupID)

	refused := func(id, from string) {
		t.Helper()
		_, err := f.patchState(id, domain.ChangeRequestStateScheduled)
		f.wantValidationError("manual scheduled from "+from, err, "cannot be set manually")
		if got := f.state(id); got != from {
			t.Fatalf("state after refused scheduled from %s = %q", from, got)
		}
		if approved, _ := f.customerOutcome(id); approved {
			t.Fatalf("a refused scheduled from %s still stamped is_customer_approved", from)
		}
	}
	refused(normal, "NEW")
	f.requestApproval(normal)
	refused(normal, "ASSESS")
	if err := f.decide(normal, crFlowPeerAID, "approved"); err != nil {
		t.Fatalf("peer approval: %v", err)
	}
	refused(normal, "AUTHORIZE")
	if err := f.decide(normal, crCABMemberUserID1, "approved"); err != nil {
		t.Fatalf("CAB approval: %v", err)
	}
	f.expect(normal, "after CAB approval (not ticked)", "SCHEDULED", "implement", "canceled")
	// Scheduled itself: a resent scheduled is not "recording the customer's approval".
	refused(normal, "SCHEDULED")
	f.step(normal, domain.ChangeRequestStateImplement, "IMPLEMENT", "review", "canceled")
	refused(normal, "IMPLEMENT")
	f.step(normal, domain.ChangeRequestStateReview, "REVIEW", "closed", "canceled")
	refused(normal, "REVIEW")

	// authorize / customer_approval stay unreachable by hand even when required.
	if _, err := f.patch(std, domain.PatchChangeRequestRequest{CustomerApprovalRequired: boolp(true)}); err != nil {
		t.Fatalf("tick: %v", err)
	}
	for _, target := range []domain.ChangeRequestState{domain.ChangeRequestStateCustomerApproval, domain.ChangeRequestStateAuthorize} {
		_, err := f.patchState(std, target)
		f.wantValidationError("manual "+string(target), err, "cannot be set manually")
		f.expect(std, "after refused manual "+string(target), "NEW", "assess", "canceled")
	}
}

// Recording the customer's approval: stamps is_customer_approved through the
// same one-way lock as a direct isCustomerApproved write, refuses a
// contradictory isCustomerApproved:false, and is blocked while on hold.
func TestChangeRequestFlowIntegration_CustomerApprovalRecordsTheApproval(t *testing.T) {
	f := newCRFlow(t)
	f.seedAssignedGroup()
	id := f.createGated(domain.ChangeRequestTypeStandard, crFlowGroupID, boolp(true), nil)
	f.requestApproval(id)
	f.expect(id, "at Customer Approval", "CUSTOMER_APPROVAL", "scheduled", "canceled")

	// Contradictory flag in the same PATCH.
	sched := domain.ChangeRequestStateScheduled
	_, err := f.patch(id, domain.PatchChangeRequestRequest{State: &sched, IsCustomerApproved: boolp(false)})
	f.wantValidationError("scheduled + isCustomerApproved false", err, "isCustomerApproved cannot be false")
	f.expect(id, "after the contradictory PATCH", "CUSTOMER_APPROVAL", "scheduled", "canceled")

	// On hold blocks the state change like any other.
	if _, err := f.patch(id, domain.PatchChangeRequestRequest{OnHold: boolp(true)}); err != nil {
		t.Fatalf("put on hold: %v", err)
	}
	_, err = f.patchState(id, domain.ChangeRequestStateScheduled)
	f.wantValidationError("scheduled while on hold", err, "on hold")
	f.expect(id, "while on hold", "CUSTOMER_APPROVAL", "scheduled", "canceled")
	if approved, _ := f.customerOutcome(id); approved {
		t.Fatal("is_customer_approved stamped by a PATCH that was refused")
	}
	// Off hold and approve in one call.
	if _, err := f.patch(id, domain.PatchChangeRequestRequest{State: &sched, OnHold: boolp(false)}); err != nil {
		t.Fatalf("take off hold and record the approval: %v", err)
	}
	f.expect(id, "after recording the approval", "SCHEDULED", "implement", "canceled")
	if approved, _ := f.customerOutcome(id); !approved {
		t.Fatal("is_customer_approved = false after recording the customer's approval")
	}

	// With the explicit flag alongside (true), also fine; a second record is refused as manual scheduled.
	id2 := f.createGated(domain.ChangeRequestTypeStandard, crFlowGroupID, boolp(true), nil)
	f.requestApproval(id2)
	if _, err := f.patch(id2, domain.PatchChangeRequestRequest{State: &sched, IsCustomerApproved: boolp(true)}); err != nil {
		t.Fatalf("scheduled + isCustomerApproved true: %v", err)
	}
	f.expect(id2, "after scheduled + isCustomerApproved", "SCHEDULED", "implement", "canceled")
}

// The customer declining: Cancel is offered at Customer Approval and works.
func TestChangeRequestFlowIntegration_CustomerApprovalCanBeCancelled(t *testing.T) {
	f := newCRFlow(t)
	f.seedAssignedGroup()
	id := f.createGated(domain.ChangeRequestTypeStandard, crFlowGroupID, boolp(true), nil)
	f.requestApproval(id)
	f.step(id, domain.ChangeRequestStateCanceled, "CANCELED")
	if approved, _ := f.customerOutcome(id); approved {
		t.Fatal("is_customer_approved = true on a change the customer declined")
	}
}

// Creator / SRE approval rules from the CAB flow still hold when the customer
// gates are ticked.
func TestChangeRequestFlowIntegration_ApproverRulesHoldWithCustomerGates(t *testing.T) {
	f := newCRFlow(t)
	f.seedAssignedGroup()
	seedApprovalGroupMembers(t, f.scoped, crCABGroupID, crCABMemberUserID1)
	id := f.createGated(domain.ChangeRequestTypeNormal, crFlowGroupID, boolp(true), boolp(true))
	f.requestApproval(id)

	// The creator cannot approve the peer stage; an SRE-team member is not even
	// a peer approver (not provisioned).
	var fe *apierror.ForbiddenError
	if err := f.decide(id, crFlowCreatorID, "approved"); !errors.As(err, &fe) {
		t.Fatalf("creator approving the peer stage err = %v (%T), want ForbiddenError", err, err)
	}
	if stages := f.stages(id); len(stages) != 1 {
		t.Fatalf("stages = %d, want 1", len(stages))
	} else if _, ok := stages[0].approvers[crFlowSREID]; ok {
		t.Fatal("the SRE-team member was provisioned as a peer approver")
	}
	if err := f.decide(id, crFlowPeerAID, "approved"); err != nil {
		t.Fatalf("peer approval: %v", err)
	}
	f.expect(id, "after peer approval", "AUTHORIZE", "canceled")

	// Creator on the CAB stage by force: still refused, and the change does not
	// reach the customer gate.
	if _, err := f.scoped.Exec(f.sys,
		`INSERT INTO approval_stage_approver (id, created_on, updated_on, created_by, updated_by, stage_id, work_item_id, approver_user_id, status)
		 SELECT gen_random_uuid(), now(), now(), 'cr-flow-test', 'cr-flow-test', s.id, s.work_item_id, $2::uuid, 'requested'
		 FROM approval_stage s WHERE s.work_item_id = $1 AND s.checkpoint_label = 'CAB Approval'`, id, crFlowCreatorID); err != nil {
		t.Fatalf("force creator onto the CAB stage: %v", err)
	}
	if err := f.decide(id, crFlowCreatorID, "approved"); !errors.As(err, &fe) {
		t.Fatalf("creator approving the CAB stage err = %v (%T), want ForbiddenError", err, err)
	}
	f.expect(id, "after the refused creator approval", "AUTHORIZE", "canceled")

	// A CAB rejection does not move the change to either gate.
	if err := f.decide(id, crCABMemberUserID1, "rejected"); err != nil {
		t.Fatalf("CAB rejection: %v", err)
	}
	f.expect(id, "after CAB rejection", "AUTHORIZE", "canceled")

	// The creator may still cancel.
	f.step(id, domain.ChangeRequestStateCanceled, "CANCELED")
}

// Rows that predate the checkboxes (false/false): approval behaves as before
// (no customer step), a Review offers Closed directly, and a row already
// sitting in Customer Approval / Customer Review gets their outgoing moves.
func TestChangeRequestFlowIntegration_LegacyRowsDefaultToNoCustomerSteps(t *testing.T) {
	f := newCRFlow(t)
	f.seedAssignedGroup()
	id := f.create(domain.ChangeRequestTypeNormal, crFlowGroupID)
	cr := f.get(id)
	if cr.CustomerApprovalRequired || cr.CustomerReviewRequired {
		t.Fatalf("defaults = %v/%v, want false/false", cr.CustomerApprovalRequired, cr.CustomerReviewRequired)
	}
	for _, tc := range []struct {
		state string
		legal []string
	}{
		{"REVIEW", []string{"closed", "canceled"}},
		{"CUSTOMER_APPROVAL", []string{"scheduled", "canceled"}},
		{"CUSTOMER_REVIEW", []string{"closed", "canceled"}},
		{"SCHEDULED", []string{"implement", "canceled"}},
	} {
		if _, err := f.scoped.Exec(f.sys, `UPDATE change_request SET state = $2::change_request_state_enum WHERE id = $1`, id, tc.state); err != nil {
			t.Fatalf("seed state %s: %v", tc.state, err)
		}
		f.expect(id, "for a legacy row in "+tc.state, tc.state, tc.legal...)
	}
	// A legacy row already in customer_approval can have its approval recorded.
	if _, err := f.scoped.Exec(f.sys, `UPDATE change_request SET state = 'CUSTOMER_APPROVAL' WHERE id = $1`, id); err != nil {
		t.Fatalf("seed state: %v", err)
	}
	f.step(id, domain.ChangeRequestStateScheduled, "SCHEDULED", "implement", "canceled")
}

// Migration 0189 is idempotent: re-running it changes neither the columns nor
// any stored value, and the columns are NOT NULL DEFAULT false.
func TestChangeRequestFlowIntegration_CustomerGateMigrationIsIdempotent(t *testing.T) {
	f := newCRFlow(t)
	f.seedAssignedGroup()
	id := f.createGated(domain.ChangeRequestTypeNormal, crFlowGroupID, boolp(true), boolp(true))

	for _, col := range []string{"customer_approval_required", "customer_review_required"} {
		var nullable, def string
		if err := f.scoped.QueryRow(f.sys,
			`SELECT is_nullable, COALESCE(column_default, '') FROM information_schema.columns WHERE table_name = 'change_request' AND column_name = $1`, col).Scan(&nullable, &def); err != nil {
			t.Fatalf("describe %s: %v", col, err)
		}
		if nullable != "NO" || def != "false" {
			t.Fatalf("%s: nullable=%q default=%q, want NOT NULL DEFAULT false", col, nullable, def)
		}
	}

	sqlBytes, err := os.ReadFile("../../migrations/0189_change_request_customer_gates.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := f.pool.Exec(context.Background(), string(sqlBytes)); err != nil {
			t.Fatalf("re-running migration 0189 (pass %d): %v", i+1, err)
		}
	}
	cr := f.get(id)
	if !cr.CustomerApprovalRequired || !cr.CustomerReviewRequired {
		t.Fatalf("flags after re-running the migration = %v/%v, want the stored true/true", cr.CustomerApprovalRequired, cr.CustomerReviewRequired)
	}
}
