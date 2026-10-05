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
// Change request creation (POST /change-requests). Unlike case creation,
// there's no safe low-severity option to file under — every real submit
// here creates a permanent ServiceNow change request with no delete
// endpoint, so the happy-path test is deliberately the only one that
// actually submits, and its subject is always E2E-tagged (see
// e2eChangeRequestSubject) so it's identifiable in staging afterward.
//

import { test, expect, withRole } from "../../fixtures/test";
import { ChangeRequestCreatePage } from "../../pages/ChangeRequestCreatePage";
import { e2eChangeRequestSubject } from "../../utils/selectors";

withRole(test, "approver");

test.describe("change request creation — page structure", () => {
  test("offers exactly Normal, Standard and Emergency, with none pre-selected", async ({ page }) => {
    const cr = new ChangeRequestCreatePage(page);
    await cr.goto();

    const radios = cr.typeGroup().getByRole("radio");
    await expect(radios).toHaveCount(3);
    await expect(cr.typeRadio("Normal")).not.toBeChecked();
    await expect(cr.typeRadio("Standard")).not.toBeChecked();
    await expect(cr.typeRadio("Emergency")).not.toBeChecked();
    // Order mirrors ServiceNow's "What type of change is required?" screen.
    await expect(radios.nth(0)).toHaveAttribute("value", "normal");
    await expect(radios.nth(1)).toHaveAttribute("value", "standard");
    await expect(radios.nth(2)).toHaveAttribute("value", "emergency");
  });

  test("requires a change type and a subject before Create change request is enabled", async ({ page }) => {
    const cr = new ChangeRequestCreatePage(page);
    await cr.goto();

    await expect(cr.createButton()).toBeDisabled();
    // A subject alone is not enough -- the type is required.
    await cr.subjectField().fill(e2eChangeRequestSubject("validation check"));
    await expect(cr.createButton()).toBeDisabled();
    await cr.selectType("Emergency");
    await expect(cr.createButton()).toBeEnabled();
  });

  test("sends the chosen type in the POST /change-requests payload", async ({ page }) => {
    const cr = new ChangeRequestCreatePage(page);
    await cr.goto();
    await cr.selectType("Standard");
    await cr.subjectField().fill(e2eChangeRequestSubject("payload check"));

    // Intercept (and abort) the create so this check never leaves a
    // permanent staging record behind -- it only inspects the request body.
    let sentType: unknown;
    await page.route(
      (url) => url.pathname.endsWith("/change-requests"),
      async (route) => {
        if (route.request().method() !== "POST") return route.fallback();
        sentType = (route.request().postDataJSON() as { type?: string }).type;
        await route.abort();
      },
    );
    await cr.createButton().click();
    await expect.poll(() => sentType).toBe("standard");
  });
});

test.describe("change request creation — Customer Approval / Customer Review checkboxes", () => {
  test("offers both as real checkboxes, unchecked by default, with their helper lines", async ({ page }) => {
    const cr = new ChangeRequestCreatePage(page);
    await cr.goto();

    await expect(cr.customerApprovalCheckbox()).not.toBeChecked();
    await expect(cr.customerReviewCheckbox()).not.toBeChecked();
    await expect(
      page.getByText("Adds a customer approval step after internal approval, before scheduling."),
    ).toBeVisible();
    await expect(page.getByText("Adds a customer review step after Review, before closing.")).toBeVisible();
    await expect(page.getByRole("switch", { name: /customer (approval|review)/i })).toHaveCount(0);
  });

  for (const [approval, review] of [
    [false, false],
    [true, false],
    [false, true],
    [true, true],
  ] as const) {
    test(`always sends both flags in the POST payload (approval ${approval}, review ${review})`, async ({ page }) => {
      const cr = new ChangeRequestCreatePage(page);
      await cr.goto();
      await cr.selectType("Normal");
      await cr.subjectField().fill(e2eChangeRequestSubject("customer flags payload check"));
      if (approval) await cr.customerApprovalCheckbox().check();
      if (review) await cr.customerReviewCheckbox().check();

      // Intercept (and abort) the create so this check never leaves a
      // permanent staging record behind -- it only inspects the request body.
      let sent: Record<string, unknown> | undefined;
      await page.route(
        (url) => url.pathname.endsWith("/change-requests"),
        async (route) => {
          if (route.request().method() !== "POST") return route.fallback();
          sent = route.request().postDataJSON() as Record<string, unknown>;
          await route.abort();
        },
      );
      await cr.createButton().click();
      await expect.poll(() => sent).toBeDefined();
      expect(sent).toMatchObject({ customerApprovalRequired: approval, customerReviewRequired: review });
    });
  }
});

test.describe("change request creation — happy path", () => {
  test("creates a real change request and lands on its detail page", async ({ page }) => {
    // Real network round trip to create, then a navigation and a second
    // fetch to load the detail page — comfortably exceeds the 30s default.
    test.setTimeout(60_000);

    const cr = new ChangeRequestCreatePage(page);
    await cr.goto();

    const subject = e2eChangeRequestSubject("e2e change request creation");
    await cr.fillSubjectAndSubmit(subject);

    // CsmChangeRequestDetailPage titles itself with the CR's own subject
    // once loaded, which is the strongest available confirmation that the
    // record we just created (not some other one) is what's showing.
    await expect(
      page.getByRole("heading", { level: 5, name: subject }),
    ).toBeVisible({ timeout: 15_000 });
  });
});
