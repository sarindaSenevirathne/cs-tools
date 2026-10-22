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

import { describe, expect, it, vi } from "vitest";
import { buildListExportFile, LIST_EXPORT_FORMATS, listExportExtension, rowsToJsonText } from "./listExport";
import { XLSX_MIME_TYPE } from "./xlsxWriter";

const buildTablePdf = vi.fn(() => new Blob(["%PDF"], { type: "application/pdf" }));
vi.mock("@utils/tableExportPdf", () => ({ buildTablePdf: (...args: unknown[]) => buildTablePdf(...(args as [])) }));

/** jsdom's Blob has no `text()` / `arrayBuffer()`: read it the way a browser page would. */
function readBlob(blob: Blob): Promise<Uint8Array> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(new Uint8Array(reader.result as ArrayBuffer));
    reader.onerror = () => reject(reader.error);
    reader.readAsArrayBuffer(blob);
  });
}

async function readText(blob: Blob): Promise<string> {
  return new TextDecoder().decode(await readBlob(blob));
}

const TABLE = {
  title: "Incidents",
  subtitle: "2 incidents",
  header: ["Number", "Subject"],
  rows: [
    ["INC1", "=cmd"],
    ["INC2"],
  ],
};

describe("listExport", () => {
  it("offers CSV, Excel, JSON and PDF, in that order", () => {
    expect(LIST_EXPORT_FORMATS.map((f) => f.label)).toEqual(["CSV", "Excel (.xlsx)", "JSON", "PDF"]);
    expect(LIST_EXPORT_FORMATS.map((f) => listExportExtension(f.format))).toEqual(["csv", "xlsx", "json", "pdf"]);
  });

  it("writes JSON as one object per row, keyed by the column header", () => {
    expect(JSON.parse(rowsToJsonText(TABLE.header, TABLE.rows))).toEqual([
      { Number: "INC1", Subject: "=cmd" },
      { Number: "INC2", Subject: "" },
    ]);
  });

  it("builds each format with its own media type", async () => {
    const csv = await buildListExportFile("csv", TABLE);
    expect(csv.type).toBe("text/csv;charset=utf-8;");
    // CSV keeps its formula guard.
    expect(await readText(csv)).toBe(`"Number","Subject"\r\n"INC1","'=cmd"\r\n"INC2"`);

    const json = await buildListExportFile("json", TABLE);
    expect(json.type).toBe("application/json;charset=utf-8");
    expect(JSON.parse(await readText(json))).toHaveLength(2);

    const xlsx = await buildListExportFile("xlsx", TABLE);
    expect(xlsx.type).toBe(XLSX_MIME_TYPE);
    expect((await readBlob(xlsx)).subarray(0, 2)).toEqual(new Uint8Array([0x50, 0x4b])); // "PK"

    const pdf = await buildListExportFile("pdf", TABLE);
    expect(pdf.type).toBe("application/pdf");
    expect(buildTablePdf).toHaveBeenCalledWith(TABLE);
  });
});
