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

import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import "@testing-library/jest-dom/vitest";
import FilteredExportMenu from "@components/FilteredExportMenu";

const { showError, showSuccess, saveBlob, buildListExportFile } = vi.hoisted(() => ({
  showError: vi.fn(),
  showSuccess: vi.fn(),
  saveBlob: vi.fn(),
  buildListExportFile: vi.fn(async () => new Blob(["x"])),
}));

vi.mock("@context/error-banner/ErrorBannerContext", () => ({
  useErrorBanner: () => ({ showError }),
}));
vi.mock("@context/success-banner/SuccessBannerContext", () => ({
  useSuccessBanner: () => ({ showSuccess }),
}));
vi.mock("@utils/saveBlob", () => ({ saveBlob }));
// The file builders have their own tests; here only what they are handed matters.
vi.mock("@utils/listExport", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@utils/listExport")>()),
  buildListExportFile,
}));

interface Row {
  n: string;
}

function renderMenu(fetchPage: (offset: number, limit: number) => Promise<{ items: Row[]; total: number }>): void {
  render(
    <FilteredExportMenu<Row>
      entityName="change-requests"
      title="Change requests"
      header={["Number"]}
      toRow={(r) => [r.n]}
      fetchPage={fetchPage}
      pageSize={2}
      maxRows={4}
    />,
  );
}

beforeEach(() => {
  showError.mockClear();
  showSuccess.mockClear();
  saveBlob.mockClear();
  buildListExportFile.mockClear();
});

describe("FilteredExportMenu", () => {
  it("offers CSV, Excel, JSON and PDF", () => {
    renderMenu(async () => ({ items: [], total: 0 }));
    fireEvent.click(screen.getByRole("button", { name: "Export" }));
    const menu = screen.getByRole("menu", { name: "Export format" });
    expect(within(menu).getAllByRole("menuitem").map((i) => i.textContent)).toEqual([
      "CSV",
      "Excel (.xlsx)",
      "JSON",
      "PDF",
    ]);
  });

  it("exports every page of the filtered list in the chosen format, and says so", async () => {
    const fetchPage = vi.fn(async (offset: number) => ({
      items: offset === 0 ? [{ n: "CHG1" }, { n: "CHG2" }] : [{ n: "CHG3" }],
      total: 3,
    }));
    renderMenu(fetchPage);
    fireEvent.click(screen.getByRole("button", { name: "Export" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Excel (.xlsx)" }));

    await waitFor(() => expect(showSuccess).toHaveBeenCalledWith("Exported 3 change requests to Excel (.xlsx)."));
    expect(fetchPage).toHaveBeenCalledTimes(2);
    expect(buildListExportFile).toHaveBeenCalledWith(
      "xlsx",
      expect.objectContaining({ title: "Change requests", header: ["Number"], rows: [["CHG1"], ["CHG2"], ["CHG3"]] }),
    );
    const [, filename] = saveBlob.mock.calls[0] as [Blob, string];
    expect(filename).toMatch(/^change-requests-export-.+\.xlsx$/);
    expect(filename).not.toContain("partial");
  });

  it("tags a capped export as partial and says how to get the rest", async () => {
    renderMenu(async (offset) => ({ items: offset === 0 ? [{ n: "A" }, { n: "B" }] : [{ n: "C" }, { n: "D" }], total: 9 }));
    fireEvent.click(screen.getByRole("button", { name: "Export" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "JSON" }));

    await waitFor(() => expect(showError).toHaveBeenCalled());
    expect(showError.mock.calls[0][0]).toMatch(/Export stopped at 4 of 9 change requests/);
    const [, filename] = saveBlob.mock.calls[0] as [Blob, string];
    expect(filename).toMatch(/-partial\.json$/);
  });

  it("downloads nothing when a page fails", async () => {
    renderMenu(async () => {
      throw new Error("boom");
    });
    fireEvent.click(screen.getByRole("button", { name: "Export" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "PDF" }));

    await waitFor(() => expect(showError).toHaveBeenCalled());
    expect(showError.mock.calls[0][0]).toMatch(/No file was downloaded/);
    expect(saveBlob).not.toHaveBeenCalled();
    expect(buildListExportFile).not.toHaveBeenCalled();
  });

  it("can be disabled", () => {
    render(
      <FilteredExportMenu<Row>
        entityName="incidents"
        title="Incidents"
        header={["Number"]}
        toRow={(r) => [r.n]}
        fetchPage={async () => ({ items: [], total: 0 })}
        disabled
      />,
    );
    expect(screen.getByRole("button", { name: "Export" })).toBeDisabled();
  });
});
