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

import { render, screen } from "@testing-library/react";
import type { JSX } from "react";
import { describe, expect, it, vi } from "vitest";
import "@testing-library/jest-dom/vitest";
import BulkSelectionBar from "@features/csm-operations/components/BulkSelectionBar";

function bar(count: number, extra: Partial<Parameters<typeof BulkSelectionBar>[0]> = {}): JSX.Element {
  return (
    <BulkSelectionBar
      count={count}
      noun="incident"
      onEdit={vi.fn()}
      onAssignToMe={vi.fn()}
      onClear={vi.fn()}
      {...extra}
    />
  );
}

describe("BulkSelectionBar", () => {
  it("keeps one live region mounted, so the first selection is announced too", () => {
    const view = render(bar(0));
    const live = screen.getByRole("status");
    expect(live).toHaveAttribute("aria-live", "polite");
    expect(live).toHaveTextContent("");
    expect(screen.queryByRole("region", { name: "Selected rows" })).not.toBeInTheDocument();

    view.rerender(bar(1));
    // The same element, now holding the count.
    expect(screen.getByRole("status")).toBe(live);
    expect(live).toHaveTextContent("1 incident selected");
    view.rerender(bar(3));
    expect(live).toHaveTextContent("3 incidents selected");
    expect(screen.getByRole("button", { name: "Edit (3)" })).toBeEnabled();
  });

  it("holds every action while a run is in flight, and says why Assign to me is unavailable", () => {
    render(bar(2, { busy: true }));
    expect(screen.getByRole("button", { name: "Edit (2)" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Assign to me (2)" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Clear selection" })).toBeDisabled();
  });

  it("disables Assign to me with its reason", () => {
    render(bar(2, { assignToMeDisabledReason: "Your user record is still loading." }));
    expect(screen.getByRole("button", { name: "Assign to me (2)" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Edit (2)" })).toBeEnabled();
  });
});
