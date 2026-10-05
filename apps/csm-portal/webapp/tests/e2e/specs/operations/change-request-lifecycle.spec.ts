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
// Deterministic Change Request lifecycle coverage — the compulsory-
// assigned-team gate, Assess-entry approver auto-provisioning, the
// approve/cancel-sibling cascade from Assess to Authorize, and a terminal
// approval display — run against the four fixed-UUID fixtures in
// scripts/csm-compose/seed-entity-service.sql (CR-FIXED-001..004), not a
// freshly self-provisioned CR the way change-request-detail.spec.ts works.
//
// That's a deliberate, necessary difference, not a style choice:
// CreateChangeRequest always 503s against this stack's own Postgres data
// source (work_item.number has no DB sequence — see entity-service's own
// CLAUDE.md, "CreateCase and case numbers"/"Change requests"), so there is
// no "create one, then drive it" path available locally at all. The fixed
// fixtures exist specifically to give this spec something to navigate
// straight to by id.
//
// The flip side of a fixed fixture: CR-FIXED-002/003 each get moved forward
// by exactly the transition this spec exercises (New -> Assess, and an
// approval decision), and that move is NOT reset by re-running the seed
// file — `ON CONFLICT (id) DO NOTHING` only ever applies on first insert, so
// a mutated row stays mutated. Unlike staging (where change-request-detail's
// self-provisioned CRs are simply abandoned, never reused), this spec is
// meant to run before every local push, so its own fixtures have to come
// back to their starting state every time. resetFixtures() below does that
// with plain, idempotent UPDATEs against the already-running local
// docker-compose Postgres (`csm-platform-postgres-1`) before anything else
// runs — CR-FIXED-001/004 are read-only fixtures (nothing here ever mutates
// them) and need no reset.
//
// Runs only against the local stack (E2E_NO_WEBSERVER=1, see
// package.json's "test:e2e:cr-lifecycle") signed in as jane.doe@example.com
// — the approver CR-FIXED-003/004 are seeded with (see
// tests/e2e/auth/generate-session.spec.ts for how that session is minted
// with no human step, as the "crApprover" role).
//

import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { test, expect, withRole } from "../../fixtures/test";
import { ChangeRequestCreatePage } from "../../pages/ChangeRequestCreatePage";
import { ChangeRequestDetailPage } from "../../pages/ChangeRequestDetailPage";
import {
  FAKE_CAB,
  FAKE_CR_ID,
  FAKE_CREATOR,
  FAKE_ECAB,
  FAKE_PEER,
  installFakeChangeRequestApi,
} from "../../utils/fakeChangeRequestApi";

const execFileAsync = promisify(execFile);

const CR_NO_TEAM = "00000000-0000-0000-0000-000000001001";
const CR_WITH_TEAM = "00000000-0000-0000-0000-000000001002";
const CR_PENDING_APPROVAL = "00000000-0000-0000-0000-000000001003";
const CR_RESOLVED = "00000000-0000-0000-0000-000000001004";

const APOLLO_GROUP_ID = "00000000-0000-0000-0000-000000000901"; // "Example Corp ABT" — see seed-entity-service.sql
const STAGE_ID = "00000000-0000-0000-0000-000000001005";
const JANE_APPROVER_ROW = "00000000-0000-0000-0000-000000001006";
const JOHN_APPROVER_ROW = "00000000-0000-0000-0000-000000001007";

/** Restores CR-FIXED-002/003 to the exact starting state documented in
 * seed-entity-service.sql, unconditionally, via the already-running local
 * docker-compose Postgres container — see this file's own top comment for
 * why a fixed fixture needs this instead of the idempotent seed file's own
 * `ON CONFLICT DO NOTHING` inserts. Plain UPDATEs, not re-running the seed
 * file, since the rows already exist after the first ever seed. */
async function resetFixtures(): Promise<void> {
  const sql = `
    UPDATE change_request SET state = 'NEW'::change_request_state_enum, requested_by_user_id = NULL WHERE id = '${CR_WITH_TEAM}';
    UPDATE work_item SET assignment_group_id = '${APOLLO_GROUP_ID}' WHERE id = '${CR_WITH_TEAM}';
    -- Also clears the CAB/ECAB Approval stages the approval flow adds.
    DELETE FROM approval_stage_approver WHERE work_item_id = '${CR_WITH_TEAM}';
    DELETE FROM approval_stage WHERE work_item_id = '${CR_WITH_TEAM}';

    -- requested_by_user_id stays NULL here (not jane.doe/john.smith, both
    -- team members): PatchChangeRequest now auto-cancels the change's own
    -- requester instead of leaving them Requested (mirrors real ServiceNow,
    -- confirmed live against CHG0039122 — see entity-service's own CLAUDE.md).
    -- Either seeded user as requester would turn this fixture's "both
    -- members become pending approvers" demonstration into a demonstration
    -- of that unrelated exclusion instead.
    UPDATE change_request SET state = 'ASSESS'::change_request_state_enum, requested_by_user_id = NULL WHERE id = '${CR_PENDING_APPROVAL}';
    -- The "approve cascades to Authorize" test below approves Jane's row,
    -- which (since entity-service also auto-provisions the CAB Approval
    -- stage now, not just Peer Approval) creates a SECOND approval_stage + a fresh pair
    -- of approver rows for this same work item, under new gen_random_uuid()
    -- ids neither upsert below ever matches. Left alone, those accumulate
    -- across runs -- the Approvals table ends up with two rows per approver,
    -- and Playwright's strict-mode approverRow("Jane Doe") then matches more
    -- than one and fails. Delete anything that isn't this fixture's own
    -- known seeded stage/approvers before re-seeding them.
    DELETE FROM approval_stage_approver WHERE work_item_id = '${CR_PENDING_APPROVAL}'
      AND id NOT IN ('${JANE_APPROVER_ROW}', '${JOHN_APPROVER_ROW}');
    DELETE FROM approval_stage WHERE work_item_id = '${CR_PENDING_APPROVAL}' AND id <> '${STAGE_ID}';
    INSERT INTO approval_stage (id, created_on, updated_on, created_by, updated_by, work_item_id, assignment_group_id, raw_status)
      VALUES ('${STAGE_ID}', now(), now(), 'seed', 'seed', '${CR_PENDING_APPROVAL}', '${APOLLO_GROUP_ID}', 'requested')
      ON CONFLICT (id) DO UPDATE SET raw_status = 'requested';
    INSERT INTO approval_stage_approver (id, created_on, updated_on, created_by, updated_by, stage_id, work_item_id, approver_user_id, status)
      VALUES
        ('${JANE_APPROVER_ROW}', now(), now(), 'seed', 'seed', '${STAGE_ID}', '${CR_PENDING_APPROVAL}', '00000000-0000-0000-0000-000000000001', 'requested'),
        ('${JOHN_APPROVER_ROW}', now(), now(), 'seed', 'seed', '${STAGE_ID}', '${CR_PENDING_APPROVAL}', '00000000-0000-0000-0000-000000000002', 'requested')
      ON CONFLICT (id) DO UPDATE SET status = 'requested';
  `;
  await execFileAsync("docker", [
    "exec",
    "-i",
    process.env.E2E_POSTGRES_CONTAINER ?? "csm-platform-postgres-1",
    "psql",
    "-U",
    "postgres",
    "-d",
    "csm_platform",
    "-v",
    "ON_ERROR_STOP=1",
    "-c",
    sql,
  ]);
}

withRole(test, "crApprover");

// The seeded-fixture describes below need the local docker-compose stack and
// reset its Postgres rows first; scoped to this wrapper so the approval-flow
// describes at the bottom of the file (which run against an in-browser fake of
// the change-request API, see utils/fakeChangeRequestApi.ts) don't need docker.
test.describe("seeded fixtures (local stack)", () => {
test.beforeAll(async () => {
  await resetFixtures();
});

test.describe("change request lifecycle — compulsory team gate", () => {
  test("Request Approval is disabled with no assigned team, and states why", async ({ page }) => {
    const detail = new ChangeRequestDetailPage(page);
    await detail.goto(CR_NO_TEAM);

    const blocked = page.getByLabel(/Request Approval: .*assigned team/i);
    await expect(blocked).toBeVisible();
    await expect(blocked.getByRole("button", { name: "Request Approval" })).toBeDisabled();
    await expect(detail.scheduleButton()).toHaveCount(0);
  });
});

test.describe("change request lifecycle — Assess-entry auto-provisioning", () => {
  test("Request Approval succeeds once a team is assigned, and provisions that team's members as approvers", async ({
    page,
  }) => {
    test.setTimeout(60_000);

    const detail = new ChangeRequestDetailPage(page);
    await detail.goto(CR_WITH_TEAM);

    const requestApprovalButton = detail.requestApprovalButton();
    await expect(requestApprovalButton).toBeEnabled();

    const [response] = await Promise.all([
      page.waitForResponse(
        (r) => new RegExp(`/change-requests/${CR_WITH_TEAM}$`).test(r.url()) && r.request().method() === "PATCH",
        { timeout: 15_000 },
      ),
      detail.requestApproval(),
    ]);
    expect(response.ok(), `Request Approval PATCH failed (${response.status()})`).toBeTruthy();

    await expect(requestApprovalButton).toBeHidden({ timeout: 15_000 });

    // Both of the assigned team's seeded members (Jane Doe, John Smith —
    // see scripts/csm-compose/seed-entity-service.sql) should now appear as
    // Requested approvers, with no manual provisioning step.
    await expect(detail.approverStatus("Jane Doe", "Peer Approval")).toHaveText("Requested");
    await expect(detail.approverStatus("John Smith", "Peer Approval")).toHaveText("Requested");
  });
});

test.describe("change request lifecycle — approve cascades to Authorize", () => {
  test("approving one of two pending approvers cascades the state to Authorize and cancels the other", async ({
    page,
  }) => {
    test.setTimeout(60_000);

    const detail = new ChangeRequestDetailPage(page);
    await detail.goto(CR_PENDING_APPROVAL);

    // Only the signed-in user's (Jane Doe's) own pending row renders an
    // Approve button — John Smith's sibling row has none. Rows are scoped to
    // the "Peer Approval" stage because, once Jane approves, the backend
    // adds a CAB Approval stage listing the same two seeded users.
    const PEER = "Peer Approval";
    await expect(detail.approverStatus("Jane Doe", PEER)).toHaveText("Requested");
    await expect(detail.approverStatus("John Smith", PEER)).toHaveText("Requested");
    await expect(detail.approveButton("John Smith", PEER)).toHaveCount(0);

    const approveButton = detail.approveButton("Jane Doe", PEER);
    await expect(approveButton).toBeVisible();

    const [response] = await Promise.all([
      page.waitForResponse((r) => /\/change-requests\/[^/]+\/approvals?/.test(r.url()), { timeout: 15_000 }),
      approveButton.click(),
    ]);
    expect(response.ok(), `Approve decision failed (${response.status()})`).toBeTruthy();

    await expect(detail.approverStatus("Jane Doe", PEER)).toHaveText("Approved");
    await expect(detail.approverStatus("John Smith", PEER)).toHaveText("Cancelled");

    // Peer approval cascades to the CAB Approval stage (its own group, the
    // next stage), the CR is in Authorize, and there is no Schedule button.
    await expect(detail.currentStep()).toContainText("Authorize");
    await expect(detail.blockingReason()).toHaveText("Awaiting CAB Approval");
    await expect(detail.approverStatus("Jane Doe", "CAB Approval")).toHaveText("Requested");
    await expect(detail.scheduleButton()).toHaveCount(0);
  });
});

test.describe("change request lifecycle — terminal approval display", () => {
  test("an already-decided change request shows its resolved approval state with no pending actions", async ({
    page,
  }) => {
    const detail = new ChangeRequestDetailPage(page);
    await detail.goto(CR_RESOLVED);

    await expect(detail.approverStatus("Jane Doe", "Peer Approval")).toHaveText("Approved");
    await expect(detail.approverStatus("John Smith", "Peer Approval")).toHaveText("Cancelled");
    await expect(detail.approveButton()).toHaveCount(0);
    await expect(detail.rejectButton()).toHaveCount(0);
  });
});
});

//
// Approval-flow lifecycle per change type, against the in-browser fake in
// utils/fakeChangeRequestApi.ts (no records created, no seeded fixtures, no
// docker). Visible state is asserted after every step:
//
//   Normal    New -> Request Approval -> Assess [Peer Approval]
//                 -> Authorize [CAB Approval] -> (auto) Scheduled
//                 -> Implement -> Review -> Closed
//   Emergency New -> Request Approval -> Authorize [ECAB Approval only]
//                 -> (auto) Scheduled
//   Standard  New -> Request Approval -> (auto) Scheduled, no approvals
//
// With "Customer Approval" ticked, every route above stops at Customer
// Approval before Scheduled until "Record customer approval" is clicked; with
// "Customer Review" ticked, Review offers "Send for customer review" instead
// of "Close", then Customer Review offers Close.
//
// Also asserts at every step that there is no "Schedule" button and no
// "Move to Assess" label, and that the CR's creator can Cancel but never
// Approve/Reject. Each "switch user" is a page reload with the faked
// `/users/me` identity changed. The browser is still signed in with the
// captured session so the portal boots normally.
//

async function openDetail(detail: ChangeRequestDetailPage): Promise<void> {
  await detail.goto(FAKE_CR_ID);
}

async function expectNoManualSchedule(detail: ChangeRequestDetailPage): Promise<void> {
  await expect(detail.scheduleButton()).toHaveCount(0);
  await expect(detail.page.getByText(/move to assess/i)).toHaveCount(0);
}

/** Customer Approval / Customer Review appear on the stepper only when ticked. */
async function expectCustomerStepsOnLine(
  detail: ChangeRequestDetailPage,
  flags: { approval: boolean; review: boolean },
): Promise<void> {
  const expected = ["New", "Assess", "Authorize"];
  if (flags.approval) expected.push("Customer Approval");
  expected.push("Scheduled", "Implement", "Review");
  if (flags.review) expected.push("Customer Review");
  expected.push("Closed");
  await expect(detail.stepLabels()).toHaveText(expected);
}

test.describe("change request approval flow — Normal", () => {
  for (const approval of [false, true]) {
    for (const review of [false, true]) {
      test(`Normal, customer approval ${approval ? "on" : "off"}, customer review ${review ? "on" : "off"}: every step shows the right state and actions`, async ({
        page,
      }) => {
        test.setTimeout(120_000);
        const api = await installFakeChangeRequestApi(page, "normal", FAKE_CREATOR, {
          customerApprovalRequired: approval,
          customerReviewRequired: review,
        });
        const detail = new ChangeRequestDetailPage(page);

        // New: the creator requests approval. The flags are shown read-only.
        await openDetail(detail);
        await expect(detail.currentStep()).toContainText("New");
        await expect(detail.flagValue("Customer approval required")).toHaveText(approval ? "Yes" : "No");
        await expect(detail.flagValue("Customer review required")).toHaveText(review ? "Yes" : "No");
        await expectCustomerStepsOnLine(detail, { approval, review });
        await expectNoManualSchedule(detail);
        await detail.requestApproval();

        // Peer Approval is pending; the creator can't decide but can still cancel.
        await expect(detail.currentStep()).toContainText("Assess");
        await expect(detail.blockingReason()).toHaveText("Awaiting Peer Approval");
        await expect(detail.approverStage("Pat Peer")).toHaveText("Peer Approval");
        await expect(detail.approveButton()).toHaveCount(0);
        await expect(detail.rejectButton()).toHaveCount(0);
        await expect(detail.creatorApprovalNotice()).toBeVisible();
        await detail.changeStateButton().click();
        await expect(detail.cancelChangeMenuItem()).toBeEnabled();
        await page.keyboard.press("Escape");
        await expectNoManualSchedule(detail);

        // A peer approves; CAB Approval is the next, separate stage.
        api.setViewer(FAKE_PEER);
        await page.reload();
        await expect(detail.approveButton("Pat Peer")).toBeVisible();
        await detail.approve("Pat Peer");
        await expect(detail.currentStep()).toContainText("Authorize");
        await expect(detail.blockingReason()).toHaveText("Awaiting CAB Approval");
        await expect(detail.approverStage("Cam Cab")).toHaveText("CAB Approval");
        await expect(detail.approverStage("Pat Peer")).toHaveText("Peer Approval");
        await expect(detail.approverStatus("Pat Peer")).toHaveText("Approved");
        await expectNoManualSchedule(detail);

        // A CAB member approves; the page refreshes itself to Customer
        // Approval when that box is ticked, else straight to Scheduled.
        api.setViewer(FAKE_CAB);
        await page.reload();
        await detail.approve("Cam Cab");
        expect(api.requests().some((r) => r === `POST /change-requests/${FAKE_CR_ID}/approvals/decision`)).toBe(true);

        api.setViewer(FAKE_CREATOR);
        await page.reload();
        if (approval) {
          await expect(detail.currentStep()).toContainText("Customer Approval");
          await expect(detail.blockingReason()).toHaveText("Awaiting customer approval");
          await expect(page.getByRole("button", { name: "Start implementation" })).toHaveCount(0);
          await expect(detail.recordCustomerApprovalButton()).toBeVisible();
          await detail.changeStateButton().click();
          await expect(detail.cancelChangeMenuItem()).toBeEnabled();
          await page.keyboard.press("Escape");
          await expectNoManualSchedule(detail);
          await detail.recordCustomerApproval();
        } else {
          await expect(detail.recordCustomerApprovalButton()).toHaveCount(0);
        }

        // Scheduled: nothing awaited, no Schedule button.
        await expect(detail.currentStep()).toContainText("Scheduled");
        await expect(detail.blockingReason()).toHaveCount(0);
        await expect(detail.recordCustomerApprovalButton()).toHaveCount(0);
        await expectNoManualSchedule(detail);

        // The engineer-driven tail.
        await page.getByRole("button", { name: "Start implementation" }).click();
        await expect(detail.currentStep()).toContainText("Implement");
        await page.getByRole("button", { name: "Mark implemented" }).click();
        await expect(detail.currentStep()).toContainText("Review");
        if (review) {
          // Review offers only "Send for customer review" -- no Close.
          await expect(detail.sendForCustomerReviewButton()).toBeVisible();
          await expect(detail.closeButton()).toHaveCount(0);
          await detail.changeStateButton().click();
          await expect(page.getByRole("menuitem", { name: "Close", exact: true })).toHaveCount(0);
          await page.keyboard.press("Escape");
          await detail.sendForCustomerReviewButton().click();
          await expect(detail.currentStep()).toContainText("Customer Review");
          await expect(detail.blockingReason()).toHaveText("Awaiting customer review");
          await expect(detail.sendForCustomerReviewButton()).toHaveCount(0);
        } else {
          // Review offers Close and no customer review.
          await expect(detail.closeButton()).toBeVisible();
          await expect(detail.sendForCustomerReviewButton()).toHaveCount(0);
        }
        await detail.closeButton().click();
        await expect(detail.currentStep()).toContainText("Closed");
        await expect(detail.blockingReason()).toHaveCount(0);
        await expectNoManualSchedule(detail);
        expect(api.state()).toBe("closed");
      });
    }
  }

  test("a non-creator approver sees Approve and Reject, with no creator notice", async ({ page }) => {
    const api = await installFakeChangeRequestApi(page, "normal", FAKE_CREATOR);
    const detail = new ChangeRequestDetailPage(page);
    await openDetail(detail);
    await detail.requestApproval();
    await expect(detail.currentStep()).toContainText("Assess");

    api.setViewer(FAKE_PEER);
    await page.reload();
    await expect(detail.approveButton("Pat Peer")).toBeEnabled();
    await expect(detail.rejectButton("Pat Peer")).toBeEnabled();
    await expect(detail.creatorApprovalNotice()).toHaveCount(0);
  });
});

test.describe("change request approval flow — Emergency", () => {
  test("Request Approval -> ECAB Approval only (no Peer or CAB) -> auto Scheduled", async ({ page }) => {
    test.setTimeout(60_000);
    const api = await installFakeChangeRequestApi(page, "emergency", FAKE_CREATOR);
    const detail = new ChangeRequestDetailPage(page);

    await openDetail(detail);
    await detail.requestApproval();

    await expect(detail.currentStep()).toContainText("Authorize");
    await expect(detail.blockingReason()).toHaveText("Awaiting ECAB Approval");
    await expect(detail.approverStage("Eli Ecab")).toHaveText("ECAB Approval");
    await expect(page.getByRole("cell", { name: "Peer Approval", exact: true })).toHaveCount(0);
    await expect(page.getByRole("cell", { name: "CAB Approval", exact: true })).toHaveCount(0);
    await expect(detail.approveButton()).toHaveCount(0); // creator
    await expectNoManualSchedule(detail);

    api.setViewer(FAKE_ECAB);
    await page.reload();
    await detail.approve("Eli Ecab");
    await expect(detail.currentStep()).toContainText("Scheduled");
    await expectNoManualSchedule(detail);
  });
});

test.describe("change request approval flow — Emergency with Customer Approval", () => {
  test("ECAB approval stops at Customer Approval; only 'Record customer approval' reaches Scheduled", async ({ page }) => {
    test.setTimeout(60_000);
    const api = await installFakeChangeRequestApi(page, "emergency", FAKE_CREATOR, { customerApprovalRequired: true });
    const detail = new ChangeRequestDetailPage(page);

    await openDetail(detail);
    await expect(detail.flagValue("Customer approval required")).toHaveText("Yes");
    await detail.requestApproval();
    await expect(detail.currentStep()).toContainText("Authorize");
    await expect(detail.blockingReason()).toHaveText("Awaiting ECAB Approval");
    await expect(page.getByRole("cell", { name: "Peer Approval", exact: true })).toHaveCount(0);
    await expectNoManualSchedule(detail);

    api.setViewer(FAKE_ECAB);
    await page.reload();
    await detail.approve("Eli Ecab");

    api.setViewer(FAKE_CREATOR);
    await page.reload();
    await expect(detail.currentStep()).toContainText("Customer Approval");
    await expect(detail.blockingReason()).toHaveText("Awaiting customer approval");
    await expect(page.getByRole("button", { name: "Start implementation" })).toHaveCount(0);
    await expectNoManualSchedule(detail);
    await detail.recordCustomerApproval();

    await expect(detail.currentStep()).toContainText("Scheduled");
    await expect(detail.blockingReason()).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Start implementation" })).toBeVisible();
    await expectNoManualSchedule(detail);
  });
});

test.describe("change request approval flow — Standard with Customer Approval", () => {
  test("Request Approval goes to Customer Approval (not Scheduled), then Record customer approval schedules it", async ({
    page,
  }) => {
    await installFakeChangeRequestApi(page, "standard", FAKE_CREATOR, { customerApprovalRequired: true });
    const detail = new ChangeRequestDetailPage(page);

    await openDetail(detail);
    await detail.requestApproval();

    await expect(detail.currentStep()).toContainText("Customer Approval");
    await expect(detail.blockingReason()).toHaveText("Awaiting customer approval");
    await expect(page.getByText(/no approval stages recorded/i)).toBeVisible();
    await expect(page.getByRole("button", { name: "Start implementation" })).toHaveCount(0);
    await expectNoManualSchedule(detail);

    await detail.recordCustomerApproval();
    await expect(detail.currentStep()).toContainText("Scheduled");
    await expect(detail.blockingReason()).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Start implementation" })).toBeVisible();
  });
});

test.describe("change request approval flow — editing the customer checkboxes", () => {
  test("both are editable before their gate, are sent via PATCH, and show on the Approval tab afterwards", async ({
    page,
  }) => {
    const api = await installFakeChangeRequestApi(page, "normal", FAKE_CREATOR);
    const detail = new ChangeRequestDetailPage(page);
    await openDetail(detail);
    await expect(detail.flagValue("Customer approval required")).toHaveText("No");

    await detail.openEditDialog();
    await expect(detail.editCustomerApprovalCheckbox()).not.toBeChecked();
    await expect(detail.editCustomerApprovalCheckbox()).toBeEnabled();
    await expect(detail.editCustomerReviewCheckbox()).toBeEnabled();
    await detail.editCustomerApprovalCheckbox().check();
    await detail.editCustomerReviewCheckbox().check();
    const [request] = await Promise.all([
      page.waitForRequest((r) => r.method() === "PATCH" && r.url().endsWith(`/change-requests/${FAKE_CR_ID}`)),
      detail.saveEdit(),
    ]);
    expect(request.postDataJSON()).toEqual({ customerApprovalRequired: true, customerReviewRequired: true });
    await expect(detail.editDialog()).toHaveCount(0);

    expect(api.flags()).toEqual({ customerApprovalRequired: true, customerReviewRequired: true });
    await expect(detail.flagValue("Customer approval required")).toHaveText("Yes");
    await expect(detail.flagValue("Customer review required")).toHaveText("Yes");
    await expectCustomerStepsOnLine(detail, { approval: true, review: true });
  });

  test("Customer Approval is disabled with an explanation once the CR is scheduled; Customer Review stays editable", async ({
    page,
  }) => {
    await installFakeChangeRequestApi(page, "standard", FAKE_CREATOR);
    const detail = new ChangeRequestDetailPage(page);
    await openDetail(detail);
    await detail.requestApproval();
    await expect(detail.currentStep()).toContainText("Scheduled");

    await detail.openEditDialog();
    await expect(detail.editCustomerApprovalCheckbox()).toBeDisabled();
    await expect(detail.editDialog().getByText(/locked/i).first()).toBeVisible();
    await expect(detail.editCustomerReviewCheckbox()).toBeEnabled();
  });

  test("Customer Review is disabled once the CR has reached customer review", async ({ page }) => {
    const api = await installFakeChangeRequestApi(page, "standard", FAKE_CREATOR, { customerReviewRequired: true });
    const detail = new ChangeRequestDetailPage(page);
    await openDetail(detail);
    api.setState("customer_review");
    await page.reload();
    await expect(detail.currentStep()).toContainText("Customer Review");

    await detail.openEditDialog();
    await expect(detail.editCustomerReviewCheckbox()).toBeDisabled();
    await expect(detail.editCustomerApprovalCheckbox()).toBeDisabled();
  });

  test("shows the backend's refusal when the gate passed while the dialog was open (400)", async ({ page }) => {
    const api = await installFakeChangeRequestApi(page, "normal", FAKE_CREATOR);
    const detail = new ChangeRequestDetailPage(page);
    await openDetail(detail);
    await detail.openEditDialog();
    await detail.editCustomerApprovalCheckbox().check();

    // The CR moves on behind the open dialog's back; the backend refuses.
    api.setState("scheduled");
    await detail.saveEdit();
    await expect(detail.editDialog().getByRole("alert")).toContainText(
      "customerApprovalRequired cannot be changed once the change request is scheduled",
    );
    await expect(detail.editDialog()).toBeVisible();
    expect(api.flags().customerApprovalRequired).toBe(false);
  });
});

test.describe("change request approval flow — Standard", () => {
  test("Request Approval goes straight to Scheduled, with no approval stages", async ({ page }) => {
    await installFakeChangeRequestApi(page, "standard", FAKE_CREATOR);
    const detail = new ChangeRequestDetailPage(page);

    await openDetail(detail);
    await detail.requestApproval();

    await expect(detail.currentStep()).toContainText("Scheduled");
    await expect(page.getByText(/no approval stages recorded/i)).toBeVisible();
    await expect(detail.blockingReason()).toHaveCount(0);
    await expectNoManualSchedule(detail);
    await expect(page.getByRole("button", { name: "Start implementation" })).toBeVisible();
  });
});

test.describe("seeded fixtures (local stack) — create with an assignment group", () => {
  test("a team picked from the Assignment group picker is saved on create (no FK 400)", async ({ page }) => {
    test.setTimeout(60_000);

    const cr = new ChangeRequestCreatePage(page);
    await cr.goto();
    await cr.selectType("Normal");
    await cr.subjectField().fill(`[E2E] local create with assignment group ${new Date().toISOString()}`);

    const group = page.getByRole("combobox", { name: /^Assignment group/ });
    await group.fill("Apollo");
    await page.getByRole("option", { name: /Apollo/ }).first().click();

    const [response] = await Promise.all([
      page.waitForResponse((r) => r.request().method() === "POST" && /\/change-requests$/.test(r.url())),
      cr.createButton().click(),
    ]);
    expect(response.status(), await response.text()).toBe(201);
    await expect(page).toHaveURL(/\/operations\/change-requests\/(?!new(?:[/?#]|$))[^/]+$/, { timeout: 15_000 });
  });
});
