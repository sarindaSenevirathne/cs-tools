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

/**
 * A minimal Excel (.xlsx) writer for a single sheet of text cells, with no
 * dependency: an .xlsx file is a zip of a handful of XML parts, written here
 * with the zip "stored" method (no compression) and a CRC-32 per part.
 *
 * Every cell is an inline string (`t="inlineStr"`), so a value that starts with
 * `=` is text, never a formula: the export carries user-written text (subjects,
 * names) and must not be evaluated when the file is opened. The header row is
 * bold and frozen, and the columns get a width from their longest value.
 */

const encoder = new TextEncoder();

/** Whether XML 1.0 allows this code unit: tab, newline and carriage return, everything else from space up, but not U+FFFE / U+FFFF. */
function isXmlChar(code: number): boolean {
  if (code === 0x09 || code === 0x0a || code === 0x0d) return true;
  return code >= 0x20 && code !== 0xfffe && code !== 0xffff;
}

/** XML 1.0 forbids most control characters; they are dropped, and the five markup characters escaped. */
function xmlText(value: string): string {
  let kept = "";
  for (let i = 0; i < value.length; i++) {
    if (isXmlChar(value.charCodeAt(i))) kept += value[i];
  }
  return kept
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

/** The column letters of a zero-based index: 0 -> A, 25 -> Z, 26 -> AA. */
export function xlsxColumnName(index: number): string {
  let n = index + 1;
  let name = "";
  while (n > 0) {
    const rem = (n - 1) % 26;
    name = String.fromCharCode(65 + rem) + name;
    n = Math.floor((n - 1) / 26);
  }
  return name;
}

/** A sheet name Excel accepts: at most 31 characters, none of `\ / ? * [ ] :`. */
function sheetName(name: string): string {
  const cleaned = name.replace(/[\\/?*[\]:]/g, " ").trim().slice(0, 31);
  return cleaned || "Sheet1";
}

/**
 * Excel and LibreOffice read `_xHHHH_` in cell text as the character U+HHHH
 * (OOXML's ST_Xstring escape), so `Account_x0041_Name` would open as
 * `AccountAName`. Escaping the leading underscore (`_x005F_`) keeps the text as
 * written, as XlsxWriter does.
 */
export function escapeXlsxCharRefs(text: string): string {
  return text.replace(/_(x[0-9A-Fa-f]{4}_)/g, "_x005F_$1");
}

/** The most characters Excel holds in one cell. */
export const XLSX_CELL_MAX = 32767;

function sheetXml(header: string[], rows: string[][]): string {
  const all = [header, ...rows];
  const widths = header.map((_, c) =>
    Math.min(80, Math.max(8, ...all.map((r) => (r[c] ?? "").length)) + 2),
  );
  const cols = widths.map((w, c) => `<col min="${c + 1}" max="${c + 1}" width="${w}" customWidth="1"/>`).join("");
  const rowXml = all
    .map((row, r) => {
      const cells = header
        .map((_, c) => {
          const ref = `${xlsxColumnName(c)}${r + 1}`;
          const style = r === 0 ? ` s="1"` : "";
          // Excel refuses a cell longer than 32,767 characters (it reports the file as damaged).
          const text = escapeXlsxCharRefs((row[c] ?? "").slice(0, XLSX_CELL_MAX));
          return `<c r="${ref}" t="inlineStr"${style}><is><t xml:space="preserve">${xmlText(text)}</t></is></c>`;
        })
        .join("");
      return `<row r="${r + 1}">${cells}</row>`;
    })
    .join("");
  return (
    `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
    `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
    `<sheetViews><sheetView workbookViewId="0"><pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews>` +
    `<cols>${cols}</cols>` +
    `<sheetData>${rowXml}</sheetData>` +
    `</worksheet>`
  );
}

function parts(name: string, header: string[], rows: string[][]): Array<{ path: string; xml: string }> {
  return [
    {
      path: "[Content_Types].xml",
      xml:
        `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
        `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
        `<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
        `<Default Extension="xml" ContentType="application/xml"/>` +
        `<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>` +
        `<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>` +
        `<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>` +
        `</Types>`,
    },
    {
      path: "_rels/.rels",
      xml:
        `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
        `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
        `<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>` +
        `</Relationships>`,
    },
    {
      path: "xl/workbook.xml",
      xml:
        `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
        `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">` +
        `<sheets><sheet name="${xmlText(sheetName(name))}" sheetId="1" r:id="rId1"/></sheets>` +
        `</workbook>`,
    },
    {
      path: "xl/_rels/workbook.xml.rels",
      xml:
        `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
        `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
        `<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>` +
        `<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>` +
        `</Relationships>`,
    },
    {
      path: "xl/styles.xml",
      xml:
        `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
        `<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
        `<fonts count="2"><font><sz val="11"/><name val="Calibri"/></font><font><b/><sz val="11"/><name val="Calibri"/></font></fonts>` +
        `<fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill></fills>` +
        `<borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>` +
        `<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>` +
        `<cellXfs count="2"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/><xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyFont="1"/></cellXfs>` +
        `<cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles>` +
        `</styleSheet>`,
    },
    { path: "xl/worksheets/sheet1.xml", xml: sheetXml(header, rows) },
  ];
}

const CRC_TABLE = (() => {
  const table = new Uint32Array(256);
  for (let n = 0; n < 256; n++) {
    let c = n;
    for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
    table[n] = c >>> 0;
  }
  return table;
})();

/** The CRC-32 (IEEE) of `bytes`, as zip stores it. */
export function crc32(bytes: Uint8Array): number {
  let crc = 0xffffffff;
  for (let i = 0; i < bytes.length; i++) crc = CRC_TABLE[(crc ^ bytes[i]) & 0xff] ^ (crc >>> 8);
  return (crc ^ 0xffffffff) >>> 0;
}

/** A zip archive of `files`, each stored uncompressed. */
export function zipStored(files: Array<{ path: string; data: Uint8Array }>): Uint8Array<ArrayBuffer> {
  const chunks: Uint8Array[] = [];
  const central: Uint8Array[] = [];
  let offset = 0;
  for (const file of files) {
    const name = encoder.encode(file.path);
    const crc = crc32(file.data);
    const local = new DataView(new ArrayBuffer(30));
    local.setUint32(0, 0x04034b50, true); // local file header signature
    local.setUint16(4, 20, true); // version needed
    local.setUint16(6, 0x0800, true); // UTF-8 names
    local.setUint16(8, 0, true); // method: stored
    local.setUint16(10, 0, true); // time
    local.setUint16(12, 0x21, true); // date: 1980-01-01
    local.setUint32(14, crc, true);
    local.setUint32(18, file.data.length, true);
    local.setUint32(22, file.data.length, true);
    local.setUint16(26, name.length, true);
    local.setUint16(28, 0, true);
    chunks.push(new Uint8Array(local.buffer), name, file.data);

    const entry = new DataView(new ArrayBuffer(46));
    entry.setUint32(0, 0x02014b50, true); // central directory signature
    entry.setUint16(4, 20, true); // version made by
    entry.setUint16(6, 20, true); // version needed
    entry.setUint16(8, 0x0800, true);
    entry.setUint16(10, 0, true);
    entry.setUint16(12, 0, true);
    entry.setUint16(14, 0x21, true);
    entry.setUint32(16, crc, true);
    entry.setUint32(20, file.data.length, true);
    entry.setUint32(24, file.data.length, true);
    entry.setUint16(28, name.length, true);
    entry.setUint32(42, offset, true); // offset of the local header
    central.push(new Uint8Array(entry.buffer), name);

    offset += 30 + name.length + file.data.length;
  }
  const centralSize = central.reduce((n, c) => n + c.length, 0);
  const end = new DataView(new ArrayBuffer(22));
  end.setUint32(0, 0x06054b50, true); // end of central directory
  end.setUint16(8, files.length, true);
  end.setUint16(10, files.length, true);
  end.setUint32(12, centralSize, true);
  end.setUint32(16, offset, true);
  const out = new Uint8Array(offset + centralSize + 22);
  let at = 0;
  for (const c of [...chunks, ...central, new Uint8Array(end.buffer)]) {
    out.set(c, at);
    at += c.length;
  }
  return out;
}

/** The .xlsx bytes of one sheet: a bold, frozen header row and one row per entry of `rows`. */
export function buildXlsx(sheet: string, header: string[], rows: string[][]): Uint8Array<ArrayBuffer> {
  return zipStored(parts(sheet, header, rows).map(({ path, xml }) => ({ path, data: encoder.encode(xml) })));
}

export const XLSX_MIME_TYPE = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet";
