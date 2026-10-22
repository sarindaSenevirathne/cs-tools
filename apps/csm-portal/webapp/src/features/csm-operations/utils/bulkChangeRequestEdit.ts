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

import {
  changeRequestApprovedReadOnlyReason,
  changeRequestTransitionRequiresReason,
  isChangeRequestTerminal,
} from "@features/csm-operations/utils/changeRequests";
import type {
  BeChangeRequestCategory,
  BeChangeRequestImpact,
  BeChangeRequestPriority,
  BePatchChangeRequestPayload,
} from "@api/backend/types";

/**
 * The one change a bulk edit applies to every selected change request: only
 * the fields set here are sent, so each change request keeps its own value for
 * the rest.
 */
export interface BulkChangeRequestEdit {
  /** A move every selected change request can make (`bulkChangeRequestTargets`). */
  state?: string;
  impact?: BeChangeRequestImpact;
  priority?: BeChangeRequestPriority;
  category?: BeChangeRequestCategory;
  assignedTeamId?: string;
  assignedEngineerId?: string;
  /**
   * An internal work note added to every selected change request. Required
   * with a state that needs a stated reason (Roll back, Cancel change): it is
   * that reason, and it rides in the same request as the state, so a refused
   * move records no reason either.
   */
  workNote?: string;
}

/** What is sent to one change request, or why nothing is. */
export type BulkChangeRequestPlan = { patch: BePatchChangeRequestPayload } | { skip: string };

/** The fields of a bulk edit that are the change request's own content (read-only once it is approved). */
const CONTENT_FIELDS = ["impact", "priority", "category", "assignedTeamId", "assignedEngineerId"] as const;

/** Whether `edit` changes any of the change request's own fields (not just its state or journal). */
export function bulkEditTouchesContent(edit: BulkChangeRequestEdit): boolean {
  return CONTENT_FIELDS.some((field) => !!edit[field]);
}

/** Why a bulk edit cannot be applied as it stands (a reason is missing), or `null`. */
export function bulkChangeRequestEditProblem(edit: BulkChangeRequestEdit): string | null {
  if (edit.state && changeRequestTransitionRequiresReason(edit.state) && !edit.workNote?.trim()) {
    return "A reason is required for this state change. It is added to every selected change request as an internal work note.";
  }
  return null;
}

/** The words for a change request that has been approved and was asked to change a field. */
export const APPROVED_FIELDS_SKIPPED =
  "Approved, so its fields are read-only; only the state and work notes can be changed.";

/**
 * The PATCH for one change request in a bulk edit, from its current state.
 * Once a change request has been approved (Customer Approval and later) its
 * fields are read-only and only the state and a work note are sent; a change
 * request left with nothing to send is skipped with the reason. A closed,
 * canceled or rolled-back one is never edited. The backend stays the authority:
 * a move it refuses for one change request (on hold, nobody to ask, no
 * assignment group) is reported for that one and the rest go through.
 */
export function bulkChangeRequestPlan(
  cr: { state?: string | null },
  edit: BulkChangeRequestEdit,
): BulkChangeRequestPlan {
  if (isChangeRequestTerminal(cr.state)) {
    return { skip: "Closed, canceled or rolled back: it can't be edited." };
  }
  const approved = !!changeRequestApprovedReadOnlyReason(cr.state);
  const patch: BePatchChangeRequestPayload = {};
  if (edit.state) patch.state = edit.state;
  if (!approved) {
    if (edit.impact) patch.impact = edit.impact;
    if (edit.priority) patch.priority = edit.priority;
    if (edit.category) patch.category = edit.category;
    if (edit.assignedTeamId) patch.assignedTeamId = edit.assignedTeamId;
    if (edit.assignedEngineerId) patch.assignedEngineerId = edit.assignedEngineerId;
  }
  const note = edit.workNote?.trim();
  if (note) patch.workNote = note;
  if (Object.keys(patch).length === 0) {
    return { skip: approved && bulkEditTouchesContent(edit) ? APPROVED_FIELDS_SKIPPED : "Nothing to change." };
  }
  return { patch };
}

/** The words for an approved change request in "Assign to me". */
export const APPROVED_ASSIGNEE_SKIPPED = "Approved, so its assignee can no longer be changed.";

/** "Assign to me" for one change request: the signed-in user as its assignee, unless it has been approved. */
export function assignChangeRequestToMePlan(
  cr: { state?: string | null; assignedEngineer?: { id?: string | null } | null },
  userId: string,
): BulkChangeRequestPlan {
  if (isChangeRequestTerminal(cr.state)) {
    return { skip: "Closed, canceled or rolled back: it can't be edited." };
  }
  if (changeRequestApprovedReadOnlyReason(cr.state)) return { skip: APPROVED_ASSIGNEE_SKIPPED };
  if (cr.assignedEngineer?.id === userId) return { skip: "Already assigned to you." };
  return { patch: { assignedEngineerId: userId } };
}
