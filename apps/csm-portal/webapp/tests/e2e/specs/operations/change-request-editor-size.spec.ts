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
// The rich-text panels of the change request forms can be made bigger: a drag
// handle under each (also keyboard-operable) and Expand, which opens the same
// field full screen. Run against the real editor (Lexical) in a real browser.
// The create page is never submitted; the edit form runs on the fake change
// request API, so nothing is written anywhere.
//

import type { Locator, Page } from "@playwright/test";
import { test, expect, withRole } from "../../fixtures/test";
import { ChangeRequestDetailPage } from "../../pages/ChangeRequestDetailPage";
import { FAKE_CR_ID, FAKE_CREATOR, installFakeChangeRequestApi } from "../../utils/fakeChangeRequestApi";

withRole(test, "crApprover");

/** The editing area of the panel labelled `label`. */
function panelInput(page: Page, label: string): Locator {
  return page.getByRole("group", { name: label, exact: true }).locator(".editor-input");
}

async function heightOf(locator: Locator): Promise<number> {
  const box = await locator.boundingBox();
  if (!box) throw new Error("no box");
  return Math.round(box.height);
}

test.describe("change request create form — panel size", () => {
  test("a panel is made taller with the arrow keys and by dragging, and Home restores it", async ({ page }) => {
    await page.goto("/operations/change-requests/new");
    const input = panelInput(page, "Backout plan");
    await expect(input).toBeVisible();
    const before = await heightOf(input);

    const handle = page.getByRole("separator", { name: "Resize Backout plan" });
    await handle.focus();
    await page.keyboard.press("ArrowDown");
    await page.keyboard.press("ArrowDown");
    await expect.poll(() => heightOf(input)).toBeGreaterThan(before + 60);
    const afterKeys = await heightOf(input);

    const box = (await handle.boundingBox())!;
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
    await page.mouse.down();
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2 + 200, { steps: 8 });
    await page.mouse.up();
    await expect.poll(() => heightOf(input)).toBeGreaterThan(afterKeys + 150);

    await handle.focus();
    await page.keyboard.press("Home");
    await expect.poll(() => heightOf(input)).toBe(before);
  });

  test("Expand opens the field full screen; what is typed there is in the form after Done", async ({ page }) => {
    await page.goto("/operations/change-requests/new");
    await expect(panelInput(page, "Backout plan")).toBeVisible();

    await page.getByRole("button", { name: "Expand Backout plan to full screen" }).click();
    const dialog = page.getByRole("dialog", { name: "Backout plan" });
    await expect(dialog).toBeVisible();
    const big = dialog.locator(".editor-input");
    // Full screen: far taller than the panel's 300px cap.
    await expect.poll(() => heightOf(big)).toBeGreaterThan(400);
    await big.click();
    await page.keyboard.type("Restore the previous image tag, then re-run the smoke tests.");
    await dialog.getByRole("button", { name: "Done" }).click();
    await expect(dialog).toBeHidden();

    await expect(panelInput(page, "Backout plan")).toContainText("Restore the previous image tag, then re-run the smoke tests.");
    // Focus is back on Expand, so a keyboard user carries on where they were.
    await expect(page.getByRole("button", { name: "Expand Backout plan to full screen" })).toBeFocused();
  });
});

test.describe("change request edit form — panel size", () => {
  test("the rollback plan typed full screen is what Save sends", async ({ page }) => {
    const api = await installFakeChangeRequestApi(page, "normal", FAKE_CREATOR);
    const detail = new ChangeRequestDetailPage(page);
    await detail.goto(FAKE_CR_ID);
    await detail.openEditDialog();

    await page.getByRole("button", { name: "Expand Rollback plan to full screen" }).click();
    const dialog = page.getByRole("dialog", { name: "Rollback plan" });
    const big = dialog.locator(".editor-input");
    await big.click();
    await page.keyboard.type("Switch traffic back to the blue deployment.");
    await page.keyboard.press("Escape");
    await expect(dialog).toBeHidden();
    await expect(panelInput(page, "Rollback plan")).toContainText("Switch traffic back to the blue deployment.");

    await detail.saveEdit();
    await expect.poll(() => api.requestBodies().filter((b) => b.request.startsWith("PATCH")).length).toBeGreaterThan(0);
    const patch = api.requestBodies().filter((b) => b.request.startsWith("PATCH")).at(-1)!.body!;
    expect(String(patch.rollbackPlan)).toContain("Switch traffic back to the blue deployment.");
  });

  test("an approved change request's plan opens full screen read-only", async ({ page }) => {
    const api = await installFakeChangeRequestApi(page, "normal", FAKE_CREATOR);
    api.setState("scheduled");
    const detail = new ChangeRequestDetailPage(page);
    await detail.goto(FAKE_CR_ID);
    await detail.openEditDialog();

    await page.getByRole("button", { name: "Expand Rollback plan to full screen" }).click();
    const dialog = page.getByRole("dialog", { name: "Rollback plan (read-only)" });
    await expect(dialog).toBeVisible();
    await expect(dialog.locator(".editor-input")).toHaveAttribute("contenteditable", "false");
    await dialog.getByRole("button", { name: "Done" }).click();
    await expect(dialog).toBeHidden();
  });
});
