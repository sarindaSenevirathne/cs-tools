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

import { beforeEach, describe, expect, it, vi } from "vitest";

// The table is jspdf-autotable's business; what is checked is what this module
// hands it and the page furniture it draws, through recorders.
const { calls, autoTable } = vi.hoisted(() => {
  const calls: Array<{ op: string; args: unknown[] }> = [];
  return { calls, autoTable: vi.fn() };
});
vi.mock("jspdf", () => ({
  jsPDF: class {
    internal = {
      pageSize: { getWidth: () => 297, getHeight: () => 210 },
      getNumberOfPages: () => 3,
    };
    constructor(...args: unknown[]) {
      calls.push({ op: "new", args });
    }
    setFillColor(): void {}
    rect(): void {}
    setTextColor(): void {}
    setFont(): void {}
    setFontSize(): void {}
    setPage(...args: unknown[]): void {
      calls.push({ op: "setPage", args });
    }
    text(...args: unknown[]): void {
      calls.push({ op: "text", args });
    }
    output(): Blob {
      return new Blob(["%PDF"], { type: "application/pdf" });
    }
  },
}));
vi.mock("jspdf-autotable", () => ({ default: autoTable }));

import { buildTablePdf } from "./tableExportPdf";

beforeEach(() => {
  calls.length = 0;
  autoTable.mockReset();
});

describe("buildTablePdf", () => {
  it("draws a landscape table with the title, the subtitle and a page count on every page", () => {
    const blob = buildTablePdf({
      title: "Change requests",
      subtitle: "2 change requests · exported now",
      header: ["Number", "Subject"],
      rows: [["CHG1", "Rotate certificates"], ["CHG2"]],
    });
    expect(blob.type).toBe("application/pdf");
    expect(calls[0]).toEqual({ op: "new", args: [{ orientation: "landscape", unit: "mm", format: "a4" }] });
    const texts = calls.filter((c) => c.op === "text").map((c) => c.args[0]);
    expect(texts).toContain("Change requests");
    expect(texts).toContain("2 change requests · exported now");
    expect(texts.filter((t) => typeof t === "string" && t.startsWith("Page "))).toEqual([
      "Page 1 of 3",
      "Page 2 of 3",
      "Page 3 of 3",
    ]);

    const [, options] = autoTable.mock.calls[0] as [unknown, { head: string[][]; body: string[][] }];
    expect(options.head).toEqual([["Number", "Subject"]]);
    // A short row is filled out to the header's width.
    expect(options.body).toEqual([["CHG1", "Rotate certificates"], ["CHG2", ""]]);
  });

  it("keeps characters the PDF font cannot draw out of the table", () => {
    buildTablePdf({ title: "T", header: ["A"], rows: [["Done ✅ — “quoted”"]] });
    const [, options] = autoTable.mock.calls[0] as [unknown, { body: string[][] }];
    expect(options.body[0][0]).toBe(`Done  - "quoted"`);
  });
});
