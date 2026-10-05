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

import { type Locator, type Page, expect } from "@playwright/test";

/**
 * Page object for `/operations/change-requests/:id`
 * (`CsmChangeRequestDetailPage.tsx`). "Request Approval" is only rendered when
 * the CR's `legalNextStates` includes `"assess"` (data-driven, via
 * `ChangeRequestActionBar`'s own `legalNextStates` filtering); it will be
 * absent for a CR already past that stage. It is a plain, direct state PATCH
 * (`{state: "assess"}`) that starts the CR's approval flow (Peer -> CAB for
 * Normal, ECAB for Emergency, straight to Scheduled for Standard).
 * There is deliberately no "Schedule" button: a CR is moved to Scheduled
 * automatically by its CAB/ECAB approval (or, when it requires customer
 * approval, by "Record customer approval" from the Customer Approval step) --
 * see `scheduleButton()`, which exists only so specs can assert its absence.
 */
export class ChangeRequestDetailPage {
  constructor(readonly page: Page) {}

  /**
   * A freshly-created CR isn't always retrievable the instant we navigate to
   * it — the real DEV-SN backend can lag between the create write and the
   * record becoming readable. Retry the navigation (full reload) until the
   * page has genuinely loaded the record, rather than failing on the first
   * attempt. The lifecycle stepper (always rendered once a CR resolves,
   * regardless of its number/state) is the readiness signal — there is no
   * "Back to change requests" control on this page (it opens as a tab
   * alongside the dashboard, like a case does; confirmed absent anywhere in
   * CsmChangeRequestDetailPage.tsx — an earlier version of this page object
   * waited on one that never existed in the current tab-strip UI, which
   * silently turned every `goto()` call into a 45s timeout).
   */
  async goto(id: string): Promise<void> {
    await expect(async () => {
      await this.page.goto(`/operations/change-requests/${id}`);
      await expect(this.lifecycleStepper()).toBeVisible({ timeout: 3_000 });
    }).toPass({ timeout: 45_000, intervals: [1_000, 2_000, 3_000, 5_000] });
  }

  lifecycleStepper(): Locator {
    return this.page.getByRole("list", { name: "Change request lifecycle" });
  }

  requestApprovalButton(): Locator {
    return this.page.getByRole("button", { name: "Request Approval" });
  }

  async requestApproval(): Promise<void> {
    await this.requestApprovalButton().click();
  }

  /** Never expected to be visible: Scheduled is reached by approval, not by a
   * manual action. Matches a button or a menu entry containing "Schedule". */
  scheduleButton(): Locator {
    return this.page
      .getByRole("button", { name: /^schedule/i })
      .or(this.page.getByRole("menuitem", { name: /^schedule/i }));
  }

  /** The lifecycle stepper's current step (`aria-current="step"`). */
  currentStep(): Locator {
    return this.lifecycleStepper().locator('[aria-current="step"]');
  }

  /** Header note while a stage is waiting, e.g. "Awaiting CAB Approval",
   * "Awaiting customer approval" or "Awaiting customer review". */
  blockingReason(): Locator {
    return this.page.getByText(/^Awaiting .+ (Approval|review)$/i);
  }

  /** "Record customer approval" -- the only place `scheduled` is a manual action. */
  recordCustomerApprovalButton(): Locator {
    return this.page.getByRole("button", { name: "Record customer approval" });
  }

  async recordCustomerApproval(): Promise<void> {
    await this.recordCustomerApprovalButton().click();
  }

  /** Review's forward move when the CR requires customer review. */
  sendForCustomerReviewButton(): Locator {
    return this.page.getByRole("button", { name: "Send for customer review" });
  }

  /** The primary "Close" button (as opposed to the overflow menu entry). */
  closeButton(): Locator {
    return this.page.getByRole("button", { name: "Close", exact: true });
  }

  /** Read-only Yes/No on the Approval tab beside a label such as
   * "Customer approval required" / "Customer review required". */
  flagValue(label: string): Locator {
    return this.page
      .getByText(label, { exact: true })
      .locator("xpath=..")
      .getByText(/^(Yes|No)$/);
  }

  /** The lifecycle stepper's step labels, in order. */
  stepLabels(): Locator {
    return this.lifecycleStepper().getByRole("listitem");
  }

  /** The Customer Approval / Customer Review checkboxes in the edit dialog. */
  editCustomerApprovalCheckbox(): Locator {
    return this.editDialog().getByRole("checkbox", { name: "Customer Approval" });
  }

  editCustomerReviewCheckbox(): Locator {
    return this.editDialog().getByRole("checkbox", { name: "Customer Review" });
  }

  /** Explanatory notice shown to the CR's creator in the Approvals card. */
  creatorApprovalNotice(): Locator {
    return this.page.getByRole("alert").filter({ hasText: /you created this change request/i });
  }

  /** The "Stage" cell of the approvals table row for a named approver
   * ("Peer Approval" | "CAB Approval" | "ECAB Approval"). */
  approverStage(approverName: string): Locator {
    return this.approverRow(approverName).getByRole("cell").first();
  }

  /** The overflow ("Change state") menu trigger, which holds Cancel change. */
  changeStateButton(): Locator {
    return this.page.getByRole("button", { name: "Change state" });
  }

  cancelChangeMenuItem(): Locator {
    return this.page.getByRole("menuitem", { name: "Cancel change" });
  }

  editButton(): Locator {
    return this.page.getByRole("button", { name: "Edit", exact: true });
  }

  async openEditDialog(): Promise<void> {
    await this.editButton().click();
    await expect(
      this.page.getByRole("dialog").getByRole("heading", { name: "Edit change request" }),
    ).toBeVisible();
  }

  editDialog(): Locator {
    return this.page.getByRole("dialog");
  }

  /**
   * The "Planned start" `DateTimePicker` field group inside the edit dialog.
   * MUI X renders these as a sectioned `role="group"`, not a plain `<input>`
   * — see `fillDateTimeField` on `CaseDetailPage` for the fill approach (not
   * duplicated here; callers needing to set this should reuse that pattern
   * or drive it directly via keyboard input on this locator).
   */
  plannedStartGroup(): Locator {
    return this.editDialog().getByRole("group", { name: /^Planned start/ });
  }

  // "Customer approved"/"Customer reviewed" are deliberately not editable
  // controls in this dialog — see EditChangeRequestDialog.tsx's doc comment.

  saveButton(): Locator {
    return this.editDialog().getByRole("button", { name: /^(Save|Saving…)$/ });
  }

  async saveEdit(): Promise<void> {
    await this.saveButton().click();
  }

  // ── Approvals ────────────────────────────────────────────────────────────
  //
  // ChangeRequestApprovals.tsx renders one flat table (Stage/State/Approver/
  // Assignment group/Comments/Created/Approved on/Actions) with every
  // approver from every stage shown together — not the collapsible
  // per-stage accordion cards an earlier UI revision used (see that
  // component's own history in entity-service's CLAUDE.md, "Change
  // requests" section: "a full UI redesign ... now renders as one flat
  // table"). Scope by table row, not an accordion class.

  /** The approvals table row for a named approver (e.g. "Jane Doe"). The
   * same person can sit on more than one stage (Peer Approval and CAB
   * Approval both list the seeded users), so pass `stage` ("Peer Approval" |
   * "CAB Approval" | "ECAB Approval") to pick one stage's row. */
  approverRow(approverName: string, stage?: string): Locator {
    const rows = this.page.getByRole("row", { name: approverName });
    return stage ? rows.filter({ has: this.page.getByRole("cell", { name: stage, exact: true }) }) : rows;
  }

  /** That approver's status chip text ("Requested" | "Approved" |
   * "Rejected" | "Cancelled" | ...). */
  approverStatus(approverName: string, stage?: string): Locator {
    return this.approverRow(approverName, stage).locator(".MuiChip-label");
  }

  /** Approve/Reject buttons only render for the signed-in user's own
   * pending ("REQUESTED") approval row — scope by the approver's own display
   * name when more than one row is on the page at once. */
  approveButton(approverName?: string, stage?: string): Locator {
    const scope = approverName ? this.approverRow(approverName, stage) : this.page;
    return scope.getByRole("button", { name: "Approve" });
  }

  rejectButton(approverName?: string, stage?: string): Locator {
    const scope = approverName ? this.approverRow(approverName, stage) : this.page;
    return scope.getByRole("button", { name: "Reject" });
  }

  async approve(approverName?: string, stage?: string): Promise<void> {
    await this.approveButton(approverName, stage).click();
  }

  async reject(approverName?: string): Promise<void> {
    await this.rejectButton(approverName).click();
  }

  // ── Comments ─────────────────────────────────────────────────────────────

  async openComposer(): Promise<void> {
    const opener = this.page.getByRole("button", { name: "Add a comment…" });
    if (await opener.isVisible().catch(() => false)) await opener.click();
  }

  internalNoteSwitch(): Locator {
    return this.page.getByRole("switch", { name: "Internal note" });
  }

  commentEditor(): Locator {
    return this.page.getByTestId("case-description-editor");
  }

  commentSubmitButton(): Locator {
    return this.page.getByRole("button", { name: /Send to customer|Save work note/ });
  }

  async addComment(text: string, opts: { internal?: boolean } = {}): Promise<void> {
    await this.openComposer();
    const wantInternal = !!opts.internal;
    const isChecked = await this.internalNoteSwitch().isChecked();
    if (isChecked !== wantInternal) await this.internalNoteSwitch().click();
    await this.commentEditor().click();
    await this.commentEditor().fill(text);
    await this.commentSubmitButton().click();
  }

  // ── Attachments ──────────────────────────────────────────────────────────

  async uploadAttachment(filePath: string): Promise<void> {
    await this.page.locator('input[type="file"]').first().setInputFiles(filePath);
  }

  async downloadAttachment(filename: string): Promise<void> {
    await this.page.getByRole("button", { name: `Download ${filename}` }).click();
  }
}
