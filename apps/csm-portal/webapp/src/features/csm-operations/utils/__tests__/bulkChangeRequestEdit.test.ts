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

import { describe, expect, it } from "vitest";
import {
  APPROVED_ASSIGNEE_SKIPPED,
  APPROVED_FIELDS_SKIPPED,
  assignChangeRequestToMePlan,
  bulkChangeRequestEditProblem,
  bulkChangeRequestPlan,
  bulkEditTouchesContent,
} from "@features/csm-operations/utils/bulkChangeRequestEdit";

const OPEN_STATES = ["new", "assess", "authorize"];
const APPROVED_STATES = ["customer_approval", "scheduled", "implement", "review", "customer_review"];
const FINAL_STATES = ["closed", "canceled", "rollback"];

describe("bulkChangeRequestPlan", () => {
  const edit = { state: "canceled", impact: "high" as const, category: "devops" as const, workNote: "  why  " };

  it.each(OPEN_STATES)("sends every field to a change request in %s", (state) => {
    expect(bulkChangeRequestPlan({ state }, edit)).toEqual({
      patch: { state: "canceled", impact: "high", category: "devops", workNote: "why" },
    });
  });

  it.each(APPROVED_STATES)("sends only the state and the work note to an approved change request (%s)", (state) => {
    expect(bulkChangeRequestPlan({ state }, edit)).toEqual({ patch: { state: "canceled", workNote: "why" } });
  });

  it.each(APPROVED_STATES)("skips an approved change request (%s) asked only to change its fields", (state) => {
    expect(bulkChangeRequestPlan({ state }, { priority: "critical" })).toEqual({ skip: APPROVED_FIELDS_SKIPPED });
  });

  it.each(FINAL_STATES)("never edits a %s change request", (state) => {
    expect(bulkChangeRequestPlan({ state }, edit)).toEqual({
      skip: "Closed, canceled or rolled back: it can't be edited.",
    });
  });

  it("sends the assignment group and the assignee as ids", () => {
    expect(bulkChangeRequestPlan({ state: "new" }, { assignedTeamId: "g-1", assignedEngineerId: "u-1" })).toEqual({
      patch: { assignedTeamId: "g-1", assignedEngineerId: "u-1" },
    });
  });

  it("adds a work note to an approved change request on its own", () => {
    expect(bulkChangeRequestPlan({ state: "implement" }, { workNote: "deployed to staging" })).toEqual({
      patch: { workNote: "deployed to staging" },
    });
  });

  it("skips when there is nothing to send", () => {
    expect(bulkChangeRequestPlan({ state: "new" }, { workNote: "   " })).toEqual({ skip: "Nothing to change." });
  });
});

describe("bulkChangeRequestEditProblem", () => {
  it("asks for a reason to cancel or roll back, and for nothing else", () => {
    expect(bulkChangeRequestEditProblem({ state: "canceled" })).toMatch(/A reason is required/);
    expect(bulkChangeRequestEditProblem({ state: "rollback", workNote: " " })).toMatch(/A reason is required/);
    expect(bulkChangeRequestEditProblem({ state: "rollback", workNote: "failed checks" })).toBeNull();
    expect(bulkChangeRequestEditProblem({ state: "implement" })).toBeNull();
    expect(bulkChangeRequestEditProblem({ impact: "low" })).toBeNull();
  });
});

describe("bulkEditTouchesContent", () => {
  it("is true only for the change request's own fields", () => {
    expect(bulkEditTouchesContent({ state: "implement", workNote: "x" })).toBe(false);
    expect(bulkEditTouchesContent({ priority: "low" })).toBe(true);
    expect(bulkEditTouchesContent({ assignedEngineerId: "u" })).toBe(true);
  });
});

describe("assignChangeRequestToMePlan", () => {
  it("assigns an open change request to the signed-in user", () => {
    expect(assignChangeRequestToMePlan({ state: "assess" }, "me")).toEqual({ patch: { assignedEngineerId: "me" } });
  });

  it.each(APPROVED_STATES)("skips an approved change request (%s): its assignee is read-only", (state) => {
    expect(assignChangeRequestToMePlan({ state }, "me")).toEqual({ skip: APPROVED_ASSIGNEE_SKIPPED });
  });

  it("skips a change request already assigned to the user, and a final one", () => {
    expect(assignChangeRequestToMePlan({ state: "new", assignedEngineer: { id: "me" } }, "me")).toEqual({
      skip: "Already assigned to you.",
    });
    expect(assignChangeRequestToMePlan({ state: "closed" }, "me")).toEqual({
      skip: "Closed, canceled or rolled back: it can't be edited.",
    });
  });
});
