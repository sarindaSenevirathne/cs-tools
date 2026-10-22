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

import { jsPDF } from "jspdf";
import autoTable from "jspdf-autotable";
import { PDF_ACCENT_COLOR, PDF_MUTED_TEXT_COLOR, toPdfSafeText } from "@utils/pdfReportKit";

/**
 * A filtered list as a PDF table: landscape A4 (a list is wide), the report
 * kit's header band with the title and a line saying what is in it, the table
 * (its header repeated on every page) and "Page X of Y" in the footer. Every
 * string goes through `toPdfSafeText`, as everywhere else in the PDF exports.
 *
 * Loaded on demand (`import("@utils/tableExportPdf")`) so jsPDF stays out of the
 * main bundle, like the report exports.
 */
export interface TablePdfInput {
  title: string;
  /** One line under the title, e.g. "42 incidents · exported 21 Oct 2026, 10:30". */
  subtitle?: string;
  header: string[];
  rows: string[][];
}

const MARGIN = 10;

export function buildTablePdfDoc({ title, subtitle, header, rows }: TablePdfInput): jsPDF {
  const doc = new jsPDF({ orientation: "landscape", unit: "mm", format: "a4" });
  const pageWidth = doc.internal.pageSize.getWidth();
  const pageHeight = doc.internal.pageSize.getHeight();

  const band = subtitle ? 24 : 18;
  doc.setFillColor(...PDF_ACCENT_COLOR);
  doc.rect(0, 0, pageWidth, band, "F");
  doc.setTextColor(255, 255, 255);
  doc.setFont("helvetica", "bold");
  doc.setFontSize(15);
  doc.text(toPdfSafeText(title), MARGIN, 12);
  if (subtitle) {
    doc.setFont("helvetica", "normal");
    doc.setFontSize(9.5);
    doc.text(toPdfSafeText(subtitle), MARGIN, 19);
  }

  autoTable(doc, {
    startY: band + 6,
    head: [header.map(toPdfSafeText)],
    body: rows.map((row) => header.map((_, i) => toPdfSafeText(row[i] ?? ""))),
    styles: { fontSize: 8, cellPadding: 1.5, overflow: "linebreak", valign: "top" },
    headStyles: { fillColor: [240, 242, 246], textColor: [40, 40, 40], fontStyle: "bold" },
    alternateRowStyles: { fillColor: [250, 251, 253] },
    margin: { left: MARGIN, right: MARGIN, bottom: 14 },
  });

  const pages = (doc as jsPDF & { internal: { getNumberOfPages: () => number } }).internal.getNumberOfPages();
  doc.setFont("helvetica", "normal");
  doc.setFontSize(8);
  doc.setTextColor(...PDF_MUTED_TEXT_COLOR);
  for (let i = 1; i <= pages; i++) {
    doc.setPage(i);
    doc.text(`Page ${i} of ${pages}`, pageWidth - MARGIN, pageHeight - 6, { align: "right" });
  }
  return doc;
}

export function buildTablePdf(input: TablePdfInput): Blob {
  return buildTablePdfDoc(input).output("blob");
}
