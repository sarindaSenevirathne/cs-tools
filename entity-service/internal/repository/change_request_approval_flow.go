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

package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
)

// This file owns the type-dependent change request approval flow of the
// PostgreSQL data source. The three change types behave as follows
// (ServiceNow's own semantics: Normal "requires one or more approvals",
// Standard "does not require approval", Emergency "must be implemented as
// soon as possible"):
//
//	Normal:    New --(Request Approval)--> Assess [Peer Approval stage]
//	               --> Authorize [CAB Approval stage]
//	               --> Scheduled (automatically, on CAB approval)
//	               --> Implement --> Review --> Closed
//	Emergency: New --(Request Approval)--> Authorize [ECAB Approval stage only,
//	               no peer approval] --> Scheduled (automatically, on ECAB
//	               approval) --> Implement --> Review --> Closed
//	Standard:  New --(Request Approval)--> Scheduled (no approval stages at
//	               all) --> Implement --> Review --> Closed
//
// "Request Approval" is the one human action out of New and is always sent as
// {state: "assess"} -- the contract the webapp already has (legalNextStates of
// a New change request is ["assess", "canceled"] for every type). The
// resulting state is chosen here from the change's type, never by the caller.
// Scheduled is never a manual transition: it is reached only by the CAB/ECAB
// approval cascade in DecideChangeRequestApproval, or by Request Approval on
// a Standard change, which has nothing to wait for.
//
// A change request whose type is NULL or not one of the three (a legacy or
// ServiceNow-synced row: azure, infra, ...) follows the Normal flow.
//
// The creation form's two checkboxes add an optional customer step on each
// side of the implementation (change_request.customer_approval_required /
// customer_review_required, migration 0189):
//
//	approval gate: wherever the flow above would move the change to Scheduled
//	    (CAB / ECAB approval, or Request Approval on a Standard change -- an
//	    assumption, Standard has no internal approvals to put the gate after) it
//	    moves it to Customer Approval instead when customer_approval_required.
//	    A human then records the customer's approval ({state: "scheduled"},
//	    legal ONLY from Customer Approval), which stamps is_customer_approved
//	    and schedules the change.
//	review gate: Review -> Customer Review -> Closed when
//	    customer_review_required, Review -> Closed otherwise. Closing from
//	    Customer Review records the customer's review (is_customer_reviewed).

// Stage labels written to approval_stage.checkpoint_label by this file's
// provisioning. LegacyAssessLabel/LegacyAuthorizeLabel are what stages created
// before the CAB flow were written with; they are still recognised when a
// stage is classified (see classifyApprovalStage).
const (
	approvalStageLabelPeer      = "Peer Approval"
	approvalStageLabelCAB       = "CAB Approval"
	approvalStageLabelECAB      = "ECAB Approval"
	approvalStageLabelReview    = "Review"
	approvalStageLabelLegacyAss = "Assess"
	approvalStageLabelLegacyAut = "Authorize"
)

// approvalPoolKind selects where a checkpoint's approvers come from.
type approvalPoolKind int

const (
	// poolAssignedGroup: members of the change request's own assigned group
	// (work_item.assignment_group_id). Used by the Review checkpoint.
	poolAssignedGroup approvalPoolKind = iota
	// poolPeer: the peer approval pool -- see resolvePeerPool.
	poolPeer
	// poolNamedGroup: members of the group named checkpoint.GroupName (the
	// CAB / ECAB groups).
	poolNamedGroup
)

// changeRequestFlow is what Request Approval does for one change type.
type changeRequestFlow struct {
	// requestState is the state Request Approval moves a New change to.
	requestState domain.ChangeRequestState
	// checkpoint is the approval stage provisioned on entry, nil when the
	// type has no approval at all (Standard).
	checkpoint *changeRequestApprovalCheckpoint
}

// changeRequestFlowForModel resolves the flow from change_request.change_model
// (the upper-case enum label; "" or unknown follows the Normal flow).
func changeRequestFlowForModel(model string) changeRequestFlow {
	switch strings.ToUpper(model) {
	case "EMERGENCY":
		return changeRequestFlow{requestState: domain.ChangeRequestStateAuthorize, checkpoint: &changeRequestECABCheckpoint}
	case "STANDARD":
		return changeRequestFlow{requestState: domain.ChangeRequestStateScheduled}
	default:
		return changeRequestFlow{requestState: domain.ChangeRequestStateAssess, checkpoint: &changeRequestPeerCheckpoint}
	}
}

// crQuerier is the sliver of *Scoped and pgx.Tx the approval-flow helpers
// share, so they read the same inside a transaction and standalone.
type crQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// approvalStageKind is the role of an approval stage in the flow.
type approvalStageKind int

const (
	stageKindOther approvalStageKind = iota
	stageKindPeer
	stageKindCAB
	stageKindECAB
	stageKindReview
)

// classifyApprovalStage resolves a stage's role from its explicit
// checkpoint_label, falling back to its zero-based ordinal position for a
// stage with no label (a ServiceNow-synced stage, or one written before
// migration 0179) -- the historical Assess (0) / Authorize (1) convention.
func classifyApprovalStage(label *string, position int) approvalStageKind {
	if label != nil && *label != "" {
		switch *label {
		case approvalStageLabelPeer, approvalStageLabelLegacyAss:
			return stageKindPeer
		case approvalStageLabelCAB, approvalStageLabelLegacyAut:
			return stageKindCAB
		case approvalStageLabelECAB:
			return stageKindECAB
		case approvalStageLabelReview:
			return stageKindReview
		default:
			return stageKindOther
		}
	}
	switch position {
	case 0:
		return stageKindPeer
	case 1:
		return stageKindCAB
	default:
		return stageKindOther
	}
}

// approvalStageInfo reads stageID's label and ordinal position among the work
// item's stages (ordered by created_on, id -- the same ordering
// changeRequestApprovalStagesQuery uses).
func approvalStageInfo(ctx context.Context, q crQuerier, workItemID, stageID string) (approvalStageKind, error) {
	var label *string
	var pos int
	err := q.QueryRow(ctx, `
		SELECT ast.checkpoint_label,
		       (SELECT COUNT(*) FROM approval_stage earlier
		         WHERE earlier.work_item_id = $1
		           AND (earlier.created_on, earlier.id) < (ast.created_on, ast.id))
		FROM approval_stage ast WHERE ast.id = $2`, workItemID, stageID).Scan(&label, &pos)
	if err != nil {
		return stageKindOther, fmt.Errorf("read approval stage: %w", err)
	}
	return classifyApprovalStage(label, pos), nil
}

// changeRequestCreatorUserIDs returns the (lower-cased) ids of every user who
// counts as the creator of the change request and therefore may never approve
// it at any stage: the user whose email is work_item.created_by, and
// change_request.requested_by_user_id (the existing self-approval-exclusion
// identity). work_item.opened_by_user_id is deliberately not part of this:
// it is a ServiceNow "opened by" passthrough that can name someone other than
// who raised the change here. An unreadable/missing change request yields an
// empty set, not an error.
func changeRequestCreatorUserIDs(ctx context.Context, q crQuerier, workItemID string) (map[string]bool, error) {
	ids := map[string]bool{}
	var requestedBy, createdBy *string
	err := q.QueryRow(ctx, `
		SELECT cr.requested_by_user_id::text, wi.created_by
		FROM work_item wi
		LEFT JOIN change_request cr ON cr.id = wi.id
		WHERE wi.id = $1`, workItemID).Scan(&requestedBy, &createdBy)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ids, nil
		}
		return nil, fmt.Errorf("read change request creator: %w", err)
	}
	if requestedBy != nil && *requestedBy != "" {
		ids[strings.ToLower(*requestedBy)] = true
	}
	if createdBy != nil && strings.TrimSpace(*createdBy) != "" {
		rows, err := q.Query(ctx, `SELECT id::text FROM "user" WHERE LOWER(email) = LOWER($1)`, strings.TrimSpace(*createdBy))
		if err != nil {
			return nil, fmt.Errorf("resolve change request creator: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return nil, fmt.Errorf("scan change request creator: %w", err)
			}
			ids[strings.ToLower(id)] = true
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("change request creator rows: %w", err)
		}
	}
	return ids, nil
}

// sreMemberIDs returns which of userIDs belong to an SRE team (team.type
// starting with "sre", e.g. "sre-abt": Apollo, Artemis, ...). Membership is
// recognised however the mirror recorded it: a team_member row whose team_id
// is an SRE team, whose group_id is an SRE team's own id, or whose group_id
// is a "group" sharing an SRE team's name. People in an SRE group are not
// "experienced engineers" for the purposes of peer approval.
func sreMemberIDs(ctx context.Context, q crQuerier, userIDs []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(userIDs) == 0 {
		return out, nil
	}
	rows, err := q.Query(ctx, `
		SELECT DISTINCT tm.user_id::text
		FROM team_member tm
		WHERE tm.user_id = ANY($1::uuid[])
		  AND (
		        EXISTS (SELECT 1 FROM team t WHERE t.id = tm.team_id AND LOWER(t.type) LIKE 'sre%')
		     OR EXISTS (SELECT 1 FROM team t WHERE t.id = tm.group_id AND LOWER(t.type) LIKE 'sre%')
		     OR EXISTS (SELECT 1 FROM "group" g JOIN team t ON LOWER(t.name) = LOWER(g.name)
		                 WHERE g.id = tm.group_id AND LOWER(t.type) LIKE 'sre%')
		      )`, userIDs)
	if err != nil {
		return nil, fmt.Errorf("check sre membership: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan sre membership: %w", err)
		}
		out[strings.ToLower(id)] = true
	}
	return out, rows.Err()
}

// groupMemberIDs lists the distinct user ids in the "group" identified by
// groupID (team_member.group_id -- see CLAUDE.md on why group_id, not team_id).
func groupMemberIDs(ctx context.Context, q crQuerier, groupID string) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT DISTINCT user_id::text FROM team_member WHERE group_id = $1::uuid`, groupID)
	if err != nil {
		return nil, fmt.Errorf("list group members: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan group member: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// namedGroup resolves a group by name: its id (preferring, when the mirror
// produced several same-named rows, the one that actually has members) and
// its distinct members. A member is anyone with team_member.group_id pointing
// at a group of that name, or team_member.team_id pointing at a team of that
// name (the CR-notice flow addresses these audiences by team name). exists is
// false when no such group row exists at all.
func namedGroup(ctx context.Context, q crQuerier, name string) (groupID string, members []string, exists bool, err error) {
	err = q.QueryRow(ctx, `
		SELECT g.id::text FROM "group" g WHERE g.name = $1
		ORDER BY (SELECT COUNT(*) FROM team_member tm WHERE tm.group_id = g.id) DESC, g.created_on ASC, g.id ASC
		LIMIT 1`, name).Scan(&groupID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil, false, nil
		}
		return "", nil, false, fmt.Errorf("resolve group %q: %w", name, err)
	}
	rows, err := q.Query(ctx, `
		SELECT DISTINCT tm.user_id::text FROM team_member tm
		WHERE tm.group_id IN (SELECT id FROM "group" WHERE name = $1)
		   OR tm.team_id  IN (SELECT id FROM team WHERE name = $1)`, name)
	if err != nil {
		return "", nil, true, fmt.Errorf("list members of group %q: %w", name, err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", nil, true, fmt.Errorf("scan member of group %q: %w", name, err)
		}
		members = append(members, id)
	}
	return groupID, members, true, rows.Err()
}

// approvalPool is a resolved approver pool: the group the stage is recorded
// against and the users to seed as approvers.
type approvalPool struct {
	groupID string
	members []string
}

// resolvePeerPool resolves the peer approval pool of a Normal change.
//
// Only experienced engineers qualify to approve a peer: nobody who belongs to
// an SRE team (Apollo, Artemis, any other) is ever placed in the pool. The
// primary pool is the change's assigned group; when that group is an SRE group
// (every member excluded) or has nobody left once the creator is excluded, the
// PeerApprovalFallbackGroupName group is used instead -- it is the group of
// experienced engineers that peer approval is drawn from in the ServiceNow
// flow. creatorIDs are never counted as a way to satisfy the pool (they are
// still listed, as cancelled, by the caller).
func resolvePeerPool(ctx context.Context, q crQuerier, assignedTeamID *string, creatorIDs map[string]bool) (approvalPool, error) {
	eligible := func(members []string) ([]string, bool, error) {
		sre, err := sreMemberIDs(ctx, q, members)
		if err != nil {
			return nil, false, err
		}
		var kept []string
		requestable := false
		for _, m := range members {
			if sre[strings.ToLower(m)] {
				continue
			}
			kept = append(kept, m)
			if !creatorIDs[strings.ToLower(m)] {
				requestable = true
			}
		}
		return kept, requestable, nil
	}

	if assignedTeamID != nil && *assignedTeamID != "" {
		members, err := groupMemberIDs(ctx, q, *assignedTeamID)
		if err != nil {
			return approvalPool{}, err
		}
		kept, ok, err := eligible(members)
		if err != nil {
			return approvalPool{}, err
		}
		if ok {
			return approvalPool{groupID: *assignedTeamID, members: kept}, nil
		}
	}

	gid, members, exists, err := namedGroup(ctx, q, domain.PeerApprovalFallbackGroupName)
	if err != nil {
		return approvalPool{}, err
	}
	if exists {
		kept, ok, err := eligible(members)
		if err != nil {
			return approvalPool{}, err
		}
		if ok {
			return approvalPool{groupID: gid, members: kept}, nil
		}
	}
	return approvalPool{}, &apierror.ValidationError{Msg: fmt.Sprintf(
		"no eligible peer approvers: the assigned group has no members who can approve (SRE team members and the change's creator cannot, and a peer approval group is only used when its members qualify), and the %q group has none either",
		domain.PeerApprovalFallbackGroupName)}
}

// resolveApprovalPool resolves checkpoint's approver pool for the change
// request. Its errors are caller-actionable ValidationErrors with no stage
// created yet.
func resolveApprovalPool(ctx context.Context, q crQuerier, cp changeRequestApprovalCheckpoint, assignedTeamID *string, creatorIDs map[string]bool) (approvalPool, error) {
	switch cp.Pool {
	case poolPeer:
		return resolvePeerPool(ctx, q, assignedTeamID, creatorIDs)
	case poolNamedGroup:
		gid, members, exists, err := namedGroup(ctx, q, cp.GroupName)
		if err != nil {
			return approvalPool{}, err
		}
		if !exists {
			return approvalPool{}, &apierror.ValidationError{Msg: fmt.Sprintf("the %q group does not exist, so a %s stage cannot be provisioned", cp.GroupName, cp.Label)}
		}
		if len(members) == 0 {
			return approvalPool{}, &apierror.ValidationError{Msg: fmt.Sprintf("the %q group has no members to provision as %s approvers", cp.GroupName, cp.Label)}
		}
		requestable := false
		for _, m := range members {
			if !creatorIDs[strings.ToLower(m)] {
				requestable = true
				break
			}
		}
		if !requestable {
			return approvalPool{}, &apierror.ValidationError{Msg: fmt.Sprintf("the %q group has no members other than the creator to provision as %s approvers", cp.GroupName, cp.Label)}
		}
		return approvalPool{groupID: gid, members: members}, nil
	default: // poolAssignedGroup
		if assignedTeamID == nil || *assignedTeamID == "" {
			return approvalPool{}, &apierror.ValidationError{Msg: fmt.Sprintf("the assigned team has no members to provision as %s approvers", cp.Label)}
		}
		members, err := groupMemberIDs(ctx, q, *assignedTeamID)
		if err != nil {
			return approvalPool{}, err
		}
		if len(members) == 0 {
			return approvalPool{}, &apierror.ValidationError{Msg: fmt.Sprintf("the assigned team has no members to provision as %s approvers", cp.Label)}
		}
		requestable := false
		for _, m := range members {
			if !creatorIDs[strings.ToLower(m)] {
				requestable = true
				break
			}
		}
		if !requestable {
			return approvalPool{}, &apierror.ValidationError{Msg: fmt.Sprintf("the assigned team has no members other than the requester to provision as %s approvers", cp.Label)}
		}
		return approvalPool{groupID: *assignedTeamID, members: members}, nil
	}
}

// approverDecisionBlock reports why userID may not decide an approval on the
// change request right now, or nil when nothing blocks them. Only the rules
// that depend on WHO the person is are checked here (the creator rule, and the
// SRE rule on the peer stage); that they hold a REQUESTED row is decided by
// the caller's own UPDATE.
//
//   - The creator of a change request (see changeRequestCreatorUserIDs) may
//     not approve it at any stage. They may still cancel it.
//   - An SRE team member may not decide a PEER approval, even if a row for
//     them exists (membership can change after provisioning, and rows
//     can predate the rule).
//
// stageKind is the kind of the stage the caller's pending row belongs to; pass
// stageKindOther when it is not known.
func approverDecisionBlock(ctx context.Context, q crQuerier, userID string, creatorIDs map[string]bool, kind approvalStageKind) error {
	if creatorIDs[strings.ToLower(userID)] {
		return &apierror.ForbiddenError{Msg: "the creator of a change request cannot approve it"}
	}
	if kind == stageKindPeer {
		sre, err := sreMemberIDs(ctx, q, []string{userID})
		if err != nil {
			return err
		}
		if sre[strings.ToLower(userID)] {
			return &apierror.ForbiddenError{Msg: "members of an SRE team cannot give peer approval; only experienced engineers outside the SRE teams may"}
		}
	}
	return nil
}

// changeRequestGateSnapshot is what the customer gates and the approval
// routing need to know about a change request, read once under a row lock.
type changeRequestGateSnapshot struct {
	// state is the upper-case change_request_state_enum label, "" when NULL.
	state string
	// model is the upper-case change_model label, "" when NULL.
	model            string
	approvalRequired bool
	reviewRequired   bool
}

// lockChangeRequestGateSnapshot reads (and locks, FOR UPDATE) the fields the
// customer gates depend on. Locking keeps a concurrent approval decision
// (which takes the same lock) from moving the change past a gate between this
// read and the write that depends on it.
func lockChangeRequestGateSnapshot(ctx context.Context, tx pgx.Tx, id string) (changeRequestGateSnapshot, error) {
	var state, model *string
	var snap changeRequestGateSnapshot
	err := tx.QueryRow(ctx,
		`SELECT state::text, change_model::text, customer_approval_required, customer_review_required
		 FROM change_request WHERE id = $1 FOR UPDATE`, id,
	).Scan(&state, &model, &snap.approvalRequired, &snap.reviewRequired)
	if errors.Is(err, pgx.ErrNoRows) {
		return snap, &apierror.NotFoundError{Msg: "change request not found"}
	}
	if err != nil {
		return snap, fmt.Errorf("patch change request: read approval gates: %w", err)
	}
	snap.state = strings.ToUpper(stringOrEmpty(state))
	snap.model = strings.ToUpper(stringOrEmpty(model))
	return snap, nil
}

// requestApprovalDestination is the state Request Approval writes: the flow's
// own (Assess / Authorize / Scheduled), except that a flow with no internal
// approval to wait for (Standard) goes to Customer Approval instead of
// Scheduled when the customer's approval is required. Normal and Emergency
// reach the customer gate later, when CAB / ECAB approves
// (approvalGateTarget).
func requestApprovalDestination(flow changeRequestFlow, customerApprovalRequired bool) domain.ChangeRequestState {
	if flow.checkpoint == nil && flow.requestState == domain.ChangeRequestStateScheduled && customerApprovalRequired {
		return domain.ChangeRequestStateCustomerApproval
	}
	return flow.requestState
}

// approvalGateTarget is the upper-case state a CAB / ECAB approval moves the
// change to: Customer Approval when the customer's approval is required,
// Scheduled otherwise.
func approvalGateTarget(customerApprovalRequired bool) string {
	if customerApprovalRequired {
		return "CUSTOMER_APPROVAL"
	}
	return "SCHEDULED"
}

// approvalRequirementEditable reports whether customer_approval_required may
// still be changed in the given (upper-case) state: until the approval gate it
// controls has been passed, i.e. while the change is New, Assess or Authorize
// (a NULL state is a pre-lifecycle legacy row and counts as New).
func approvalRequirementEditable(state string) bool {
	switch state {
	case "", "NEW", "ASSESS", "AUTHORIZE":
		return true
	}
	return false
}

// reviewRequirementEditable reports whether customer_review_required may still
// be changed in the given (upper-case) state: until the change leaves Review,
// the step whose next move it decides. Customer Review and every terminal
// state are past it.
func reviewRequirementEditable(state string) bool {
	switch state {
	case "CUSTOMER_REVIEW", "ROLLBACK", "CLOSED", "CANCELED":
		return false
	}
	return true
}

// validateCustomerGateEdits refuses an edit of customer_approval_required /
// customer_review_required once the gate it controls has been passed. A write
// of the value already stored is a no-op and is always accepted, so a client
// that resends the whole form is not punished for fields it did not touch.
func validateCustomerGateEdits(snap changeRequestGateSnapshot, approvalRequired, reviewRequired *bool) error {
	state := strings.ToLower(snap.state)
	if state == "" {
		state = "new"
	}
	if approvalRequired != nil && *approvalRequired != snap.approvalRequired && !approvalRequirementEditable(snap.state) {
		return &apierror.ValidationError{Msg: fmt.Sprintf(
			"customerApprovalRequired can no longer be changed: the change request has already passed the approval stage (current state: %s)", state)}
	}
	if reviewRequired != nil && *reviewRequired != snap.reviewRequired && !reviewRequirementEditable(snap.state) {
		return &apierror.ValidationError{Msg: fmt.Sprintf(
			"customerReviewRequired can no longer be changed: the change request has already left the review stage (current state: %s)", state)}
	}
	return nil
}
