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

import { rowsToCsvText } from "@utils/csvExport";
import { buildXlsx, XLSX_MIME_TYPE } from "@utils/xlsxWriter";

/** The formats a filtered list can be exported in. */
export type ListExportFormat = "csv" | "xlsx" | "json" | "pdf";

export const LIST_EXPORT_FORMATS: ReadonlyArray<{ format: ListExportFormat; label: string; extension: string }> = [
  { format: "csv", label: "CSV", extension: "csv" },
  { format: "xlsx", label: "Excel (.xlsx)", extension: "xlsx" },
  { format: "json", label: "JSON", extension: "json" },
  { format: "pdf", label: "PDF", extension: "pdf" },
];

export interface ListExportTable {
  /** The list's name, e.g. "Incidents": the PDF title and the Excel sheet. */
  title: string;
  /** One line describing the export (count, time), for the PDF. */
  subtitle?: string;
  header: string[];
  rows: string[][];
}

/**
 * The rows as JSON: one object per row, keyed by the column header, in column
 * order -- the same table as every other format, readable without a key.
 */
export function rowsToJsonText(header: string[], rows: string[][]): string {
  return JSON.stringify(
    rows.map((row) => Object.fromEntries(header.map((h, i) => [h, row[i] ?? ""]))),
    null,
    2,
  );
}

/** The file for `format`. The PDF writer (jsPDF) is loaded only when a PDF is asked for. */
export async function buildListExportFile(format: ListExportFormat, table: ListExportTable): Promise<Blob> {
  switch (format) {
    case "csv":
      return new Blob([rowsToCsvText(table.header, table.rows)], { type: "text/csv;charset=utf-8;" });
    case "json":
      return new Blob([rowsToJsonText(table.header, table.rows)], { type: "application/json;charset=utf-8" });
    case "xlsx":
      return new Blob([buildXlsx(table.title, table.header, table.rows)], { type: XLSX_MIME_TYPE });
    case "pdf": {
      const { buildTablePdf } = await import("@utils/tableExportPdf");
      return buildTablePdf(table);
    }
  }
}

export function listExportExtension(format: ListExportFormat): string {
  return LIST_EXPORT_FORMATS.find((f) => f.format === format)?.extension ?? format;
}
