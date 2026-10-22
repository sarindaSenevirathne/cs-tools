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

import { describe, expect, it } from "vitest";
import { buildXlsx, crc32, escapeXlsxCharRefs, XLSX_CELL_MAX, xlsxColumnName, zipStored } from "./xlsxWriter";

const decoder = new TextDecoder();

/** Reads a zip of stored (uncompressed) entries back through its central directory, checking each CRC. */
function readZip(bytes: Uint8Array): Map<string, string> {
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  const eocd = bytes.length - 22;
  expect(view.getUint32(eocd, true)).toBe(0x06054b50);
  const count = view.getUint16(eocd + 10, true);
  let at = view.getUint32(eocd + 16, true);
  const files = new Map<string, string>();
  for (let i = 0; i < count; i++) {
    expect(view.getUint32(at, true)).toBe(0x02014b50);
    const crc = view.getUint32(at + 16, true);
    const size = view.getUint32(at + 20, true);
    const nameLength = view.getUint16(at + 28, true);
    const localOffset = view.getUint32(at + 42, true);
    const name = decoder.decode(bytes.subarray(at + 46, at + 46 + nameLength));
    expect(view.getUint32(localOffset, true)).toBe(0x04034b50);
    expect(view.getUint16(localOffset + 8, true)).toBe(0); // stored
    const localNameLength = view.getUint16(localOffset + 26, true);
    const dataStart = localOffset + 30 + localNameLength;
    const data = bytes.subarray(dataStart, dataStart + size);
    expect(crc32(data)).toBe(crc);
    files.set(name, decoder.decode(data));
    at += 46 + nameLength;
  }
  return files;
}

describe("crc32", () => {
  it("matches the IEEE check value", () => {
    expect(crc32(new TextEncoder().encode("123456789")).toString(16)).toBe("cbf43926");
    expect(crc32(new Uint8Array())).toBe(0);
  });
});

describe("xlsxColumnName", () => {
  it.each([
    [0, "A"],
    [25, "Z"],
    [26, "AA"],
    [27, "AB"],
    [701, "ZZ"],
    [702, "AAA"],
  ])("column %i is %s", (index, name) => {
    expect(xlsxColumnName(index)).toBe(name);
  });
});

describe("zipStored", () => {
  it("writes every file so that it reads back intact", () => {
    const enc = new TextEncoder();
    const zip = zipStored([
      { path: "a.txt", data: enc.encode("alpha") },
      { path: "dir/b.txt", data: enc.encode("béta") },
    ]);
    const files = readZip(zip);
    expect([...files.keys()]).toEqual(["a.txt", "dir/b.txt"]);
    expect(files.get("dir/b.txt")).toBe("béta");
  });
});

describe("buildXlsx", () => {
  it("is a workbook with one sheet: a bold, frozen header and one row per entry", () => {
    const files = readZip(buildXlsx("Incidents", ["Number", "Subject"], [["INC1", "Disk full"], ["INC2", ""]]));
    expect([...files.keys()].sort()).toEqual([
      "[Content_Types].xml",
      "_rels/.rels",
      "xl/_rels/workbook.xml.rels",
      "xl/styles.xml",
      "xl/workbook.xml",
      "xl/worksheets/sheet1.xml",
    ]);
    expect(files.get("xl/workbook.xml")).toContain(`<sheet name="Incidents" sheetId="1" r:id="rId1"/>`);
    const sheet = files.get("xl/worksheets/sheet1.xml")!;
    expect(sheet).toContain(`state="frozen"`);
    expect(sheet).toContain(`<c r="A1" t="inlineStr" s="1"><is><t xml:space="preserve">Number</t></is></c>`);
    expect(sheet).toContain(`<c r="B2" t="inlineStr"><is><t xml:space="preserve">Disk full</t></is></c>`);
    expect(sheet).toContain(`<c r="B3" t="inlineStr"><is><t xml:space="preserve"></t></is></c>`);
    expect(sheet.match(/<row /g)).toHaveLength(3);
    expect(files.get("xl/styles.xml")).toContain("<b/>");
  });

  it("escapes markup, drops characters XML forbids, and keeps text as text (never a formula)", () => {
    const files = readZip(buildXlsx("x", ["A"], [[`=SUM(1)<b>&"x"\u0001\u0008ok\ttab\nline`]]));
    const sheet = files.get("xl/worksheets/sheet1.xml")!;
    expect(sheet).toContain(`=SUM(1)&lt;b&gt;&amp;&quot;x&quot;ok\ttab\nline`);
    expect(sheet).not.toContain("<f>");
    expect(sheet.includes(String.fromCharCode(1)) || sheet.includes(String.fromCharCode(8))).toBe(false);
  });

  it("cuts a value to the most characters a cell holds", () => {
    const files = readZip(buildXlsx("x", ["A"], [["y".repeat(XLSX_CELL_MAX + 10)]]));
    const sheet = files.get("xl/worksheets/sheet1.xml")!;
    expect(sheet).toContain("y".repeat(XLSX_CELL_MAX) + "</t>");
    expect(sheet).not.toContain("y".repeat(XLSX_CELL_MAX + 1));
  });

  it("gives the sheet a name Excel accepts", () => {
    const longName = readZip(buildXlsx("Change requests: Q4 / [draft] export list", ["A"], []));
    expect(longName.get("xl/workbook.xml")).toContain(`<sheet name="Change requests  Q4    draft  e"`);
    const blank = readZip(buildXlsx("  ", ["A"], []));
    expect(blank.get("xl/workbook.xml")).toContain(`<sheet name="Sheet1"`);
  });

  it("fills a short row with empty cells so every row has every column", () => {
    const sheet = readZip(buildXlsx("x", ["A", "B", "C"], [["1"]])).get("xl/worksheets/sheet1.xml")!;
    expect(sheet).toContain(`<c r="C2" t="inlineStr"><is><t xml:space="preserve"></t></is></c>`);
  });
});

describe("escapeXlsxCharRefs", () => {
  it("keeps text that looks like Excel's _xHHHH_ escape as written", () => {
    expect(escapeXlsxCharRefs("Rename Account_x0041_Name")).toBe("Rename Account_x005F_x0041_Name");
    expect(escapeXlsxCharRefs("a_x000D_b_X00ff_c")).toBe("a_x005F_x000D_b_X00ff_c");
    expect(escapeXlsxCharRefs("_x00Fe_")).toBe("_x005F_x00Fe_");
    expect(escapeXlsxCharRefs("snake_case_x1 and _x12_")).toBe("snake_case_x1 and _x12_");
  });

  it("is applied to every cell of the sheet", () => {
    const sheet = readZip(buildXlsx("x", ["Subject"], [["Account_x0041_Name"]])).get("xl/worksheets/sheet1.xml")!;
    expect(sheet).toContain(">Account_x005F_x0041_Name</t>");
  });
});
