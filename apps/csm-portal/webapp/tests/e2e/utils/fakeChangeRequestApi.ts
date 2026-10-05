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

//
// In-browser fake of the change-request slice of the backend contract, for
// specs that must walk an approval flow deterministically without creating
// permanent ServiceNow records. Installed with `page.route`, so only the
// change-request endpoints (and the user id returned by `/users/me`) are
// faked -- everything else still reaches the real backend the rest of the
// suite runs against, and the browser session is still the captured one.
//
// The contract encoded here is the one the UI is built against:
//   - `legalNextStates` offers no manual way to `scheduled` -- except from
//     `customer_approval`, where `scheduled` means "record the customer's
//     approval" -- and `authorize` (listed from Assess) is the approval path,
//     never a button;
//   - Request Approval (`PATCH {state:"assess"}`) on a Normal CR enters Assess
//     with a "Peer Approval" stage; on an Emergency CR it enters Authorize with
//     an "ECAB Approval" stage only; on a Standard CR it goes straight to
//     the post-approval state with no approvals;
//   - approving Peer Approval adds a "CAB Approval" stage and moves to
//     Authorize; approving CAB/ECAB moves the CR on by itself;
//   - "the post-approval state" is `customer_approval` when the CR has
//     `customerApprovalRequired`, else `scheduled`;
//   - from `customer_approval` legalNextStates = [scheduled, canceled];
//   - Review offers [customer_review, canceled] when `customerReviewRequired`,
//     else [closed, canceled]; `customer_review` -> [closed, canceled];
//   - `customerApprovalRequired` / `customerReviewRequired` are on the detail
//     response and editable via PATCH until their gate passes; a late edit is
//     refused with a 400 and a readable message;
//   - the CR's creator can never approve.
//

import type { Page, Route } from "@playwright/test";

export type FakeCrType = "normal" | "standard" | "emergency";

export interface FakeUser {
  id: string;
  name: string;
  email: string;
}

export const FAKE_CREATOR: FakeUser = { id: "00000000-0000-0000-0000-00000000e001", name: "Casey Creator", email: "casey.creator@example.com" };
export const FAKE_PEER: FakeUser = { id: "00000000-0000-0000-0000-00000000e002", name: "Pat Peer", email: "pat.peer@example.com" };
export const FAKE_CAB: FakeUser = { id: "00000000-0000-0000-0000-00000000e003", name: "Cam Cab", email: "cam.cab@example.com" };
export const FAKE_ECAB: FakeUser = { id: "00000000-0000-0000-0000-00000000e004", name: "Eli Ecab", email: "eli.ecab@example.com" };

export const FAKE_CR_ID = "00000000-0000-0000-0000-00000000c001";

interface Approver {
  id: string;
  name: string;
  status: string;
}
interface Stage {
  stage: string;
  approverType: "STATIC_GROUP";
  approverName: string;
  status: string;
  approvers: Approver[];
}

/** The two ServiceNow-style creation checkboxes the CR carries. */
export interface FakeCustomerFlags {
  customerApprovalRequired: boolean;
  customerReviewRequired: boolean;
}

/** States from which each checkbox can no longer be changed (backend refuses with 400). */
const APPROVAL_FLAG_LOCKED = ["customer_approval", "scheduled", "implement", "review", "customer_review", "closed", "rollback", "canceled"];
const REVIEW_FLAG_LOCKED = ["customer_review", "closed", "rollback", "canceled"];

export interface FakeChangeRequestApi {
  /** Who the app believes is signed in (applied on the next page load). */
  setViewer(user: FakeUser): void;
  /** The CR's current lifecycle state, as the fake backend holds it. */
  state(): string;
  /** The CR's current checkbox settings, as the fake backend holds them. */
  flags(): FakeCustomerFlags;
  /** Moves the fake CR to `next` out-of-band (e.g. while an edit dialog is
   * still open on a stale copy), without touching its approval stages. */
  setState(next: string): void;
  /** Every request the fake served, as "METHOD /path". */
  requests(): string[];
}

function legalNextStates(state: string, flags: FakeCustomerFlags): string[] {
  switch (state) {
    case "new":
      return ["assess", "canceled"];
    case "assess":
      return ["authorize", "canceled"]; // authorize = the approval path, never a button
    case "authorize":
      return ["canceled"];
    case "customer_approval":
      return ["scheduled", "canceled"]; // scheduled = "Record customer approval"
    case "scheduled":
      return ["implement", "canceled"];
    case "implement":
      return ["review", "canceled"];
    case "review":
      return flags.customerReviewRequired ? ["customer_review", "canceled"] : ["closed", "canceled"];
    case "customer_review":
      return ["closed", "canceled"];
    default:
      return [];
  }
}

const nextStage = (name: string, group: string, who: FakeUser): Stage => ({
  stage: name,
  approverType: "STATIC_GROUP",
  approverName: group,
  status: "REQUESTED",
  approvers: [{ id: who.id, name: who.name, status: "REQUESTED" }],
});

export async function installFakeChangeRequestApi(
  page: Page,
  type: FakeCrType,
  viewer: FakeUser = FAKE_CREATOR,
  initialFlags: Partial<FakeCustomerFlags> = {},
): Promise<FakeChangeRequestApi> {
  let currentViewer = viewer;
  let state = "new";
  const flags: FakeCustomerFlags = {
    customerApprovalRequired: initialFlags.customerApprovalRequired ?? false,
    customerReviewRequired: initialFlags.customerReviewRequired ?? false,
  };
  /** Where a CR lands once its internal approval is granted. */
  const afterInternalApproval = (): string => (flags.customerApprovalRequired ? "customer_approval" : "scheduled");
  let stages: Stage[] = [];
  const log: string[] = [];

  const detail = (): Record<string, unknown> => ({
    id: FAKE_CR_ID,
    number: "CHG0099001",
    subject: "[E2E] approval flow (mocked)",
    createdOn: "2026-01-01T00:00:00Z",
    createdBy: FAKE_CREATOR.email,
    state,
    type,
    assignedTeam: { id: "00000000-0000-0000-0000-00000000a001", name: "Platform" },
    requestedBy: { id: FAKE_CREATOR.id, name: FAKE_CREATOR.name },
    customerApprovalRequired: flags.customerApprovalRequired,
    customerReviewRequired: flags.customerReviewRequired,
    legalNextStates: legalNextStates(state, flags),
  });

  const cors = (route: Route): Record<string, string> => ({
    "access-control-allow-origin": route.request().headers()["origin"] ?? "*",
    "access-control-allow-headers": "*",
    "access-control-allow-methods": "GET,POST,PATCH,PUT,DELETE,OPTIONS",
    "access-control-allow-credentials": "true",
  });
  const json = (route: Route, body: unknown, status = 200): Promise<void> =>
    route.fulfill({
      status,
      contentType: "application/json",
      headers: cors(route),
      body: JSON.stringify(body),
    });

  // Same identity swap for /users/me: keep the real profile (roles, time
  // zone, ...) but make the signed-in user one of the fake people above.
  await page.route(
    (url) => url.pathname.endsWith("/users/me"),
    async (route) => {
      const type = route.request().resourceType();
      if (route.request().method() !== "GET" || (type !== "fetch" && type !== "xhr")) return route.fallback();
      const real = await route.fetch();
      const profile = (await real.json()) as Record<string, unknown>;
      const [firstName, ...rest] = currentViewer.name.split(" ");
      await route.fulfill({
        response: real,
        json: { ...profile, id: currentViewer.id, email: currentViewer.email, firstName, lastName: rest.join(" ") },
      });
    },
  );

  await page.route(
    (url) => new RegExp(`/change-requests/${FAKE_CR_ID}(/.*)?$`).test(url.pathname),
    async (route) => {
      const req = route.request();
      const rtype = req.resourceType();
      if (rtype !== "fetch" && rtype !== "xhr") return route.fallback();
      if (req.method() === "OPTIONS") return route.fulfill({ status: 204, headers: cors(route) });

      const path = new URL(req.url()).pathname.replace(/^.*\/change-requests\//, "/change-requests/");
      log.push(`${req.method()} ${path}`);

      if (path.endsWith("/approvals/decision") && req.method() === "POST") {
        const { decision } = req.postDataJSON() as { decision: "approved" | "rejected" };
        const current = stages.find((s) => s.status === "REQUESTED");
        const row = current?.approvers.find((a) => a.id === currentViewer.id && a.status === "REQUESTED");
        if (!current || !row || currentViewer.id === FAKE_CREATOR.id) {
          return json(route, { message: "Access to the requested resource is forbidden!" }, 403);
        }
        row.status = decision === "approved" ? "APPROVED" : "REJECTED";
        current.status = row.status;
        if (decision === "approved") {
          if (current.stage === "Peer Approval") {
            state = "authorize";
            stages = [...stages, nextStage("CAB Approval", "CAB", FAKE_CAB)];
          } else {
            state = afterInternalApproval(); // CAB / ECAB approval moves the CR on itself
          }
        }
        return json(route, { id: FAKE_CR_ID, state });
      }
      if (path.endsWith("/approvals") && req.method() === "GET") {
        // Like the Postgres-backed API: `canDecide` is true only on the
        // caller's own REQUESTED row, and never for the CR's creator.
        return json(route, {
          approvals: stages.map((st) => ({
            ...st,
            approvers: st.approvers.map((a) => ({
              ...a,
              canDecide: a.id === currentViewer.id && a.status === "REQUESTED" && currentViewer.id !== FAKE_CREATOR.id,
            })),
          })),
        });
      }
      if (path.endsWith("/comments/search")) {
        return json(route, { comments: [], hasMore: false, totalRecords: 0 });
      }
      if (req.method() === "PATCH") {
        const body = req.postDataJSON() as {
          state?: string;
          customerApprovalRequired?: boolean;
          customerReviewRequired?: boolean;
        };
        // Checkbox edits: refused once the gate they control has passed.
        if (body.customerApprovalRequired !== undefined) {
          if (APPROVAL_FLAG_LOCKED.includes(state)) {
            return json(route, { message: `customerApprovalRequired cannot be changed once the change request is ${state}` }, 400);
          }
          flags.customerApprovalRequired = body.customerApprovalRequired;
        }
        if (body.customerReviewRequired !== undefined) {
          if (REVIEW_FLAG_LOCKED.includes(state)) {
            return json(route, { message: `customerReviewRequired cannot be changed once the change request is ${state}` }, 400);
          }
          flags.customerReviewRequired = body.customerReviewRequired;
        }
        const target = body.state;
        if (target === undefined) {
          return json(route, { id: FAKE_CR_ID, state });
        }
        if (target === "assess") {
          if (type === "standard") state = afterInternalApproval();
          else if (type === "emergency") {
            state = "authorize";
            stages = [nextStage("ECAB Approval", "ECAB", FAKE_ECAB)];
          } else {
            state = "assess";
            stages = [nextStage("Peer Approval", "Peers", FAKE_PEER)];
          }
        } else if (target === "scheduled" && state === "customer_approval") {
          state = "scheduled"; // the customer's approval was recorded
        } else if (target !== "scheduled" && target !== "authorize" && target !== "customer_approval") {
          if (!legalNextStates(state, flags).includes(target)) {
            return json(route, { message: `Illegal transition from ${state} to ${target}.` }, 400);
          }
          state = target;
        } else {
          return json(route, { message: `Illegal transition to ${String(target)}.` }, 400);
        }
        return json(route, { id: FAKE_CR_ID, state });
      }
      if (req.method() === "GET" && path === `/change-requests/${FAKE_CR_ID}`) {
        return json(route, detail());
      }
      return route.fallback();
    },
  );

  return {
    setViewer: (user) => {
      currentViewer = user;
    },
    state: () => state,
    setState: (next) => {
      state = next;
    },
    flags: () => ({ ...flags }),
    requests: () => [...log],
  };
}
