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
import {
  activeRunbookFilterCount,
  cleanAlertNames,
  DEFAULT_RUNBOOK_FILTERS,
  filterRunbooks,
  findAlertsContaining,
  findRunbookForIncident,
  isValidRunbookUrl,
  parseRunbookImport,
  type Runbook,
  runbooksToCsvRows,
  runbooksToImportJson,
} from "./runbooks";

describe("parseRunbookImport", () => {
  it("reads an object whose keys are alert names, a comma included, as one alert each", () => {
    expect(
      parseRunbookImport('{"CPU > 90%, prod": "https://docs.example.com/a", " Alert B ": ""}'),
    ).toEqual([
      { name: "", runbookUrl: "https://docs.example.com/a", alerts: ["CPU > 90%, prod"] },
      { name: "", runbookUrl: null, alerts: ["Alert B"] },
    ]);
  });

  it("reads a list of runbooks with their names, a list of alerts, and a text alerts value as one alert", () => {
    expect(
      parseRunbookImport('[{"name": "Gateway", "runbookUrl": null, "alerts": ["a", "b"]}, {"alerts": "c, d"}]'),
    ).toEqual([
      { name: "Gateway", runbookUrl: null, alerts: ["a", "b"] },
      { name: "", runbookUrl: null, alerts: ["c, d"] },
    ]);
  });

  it("still reads an older list entry with one alertName", () => {
    expect(parseRunbookImport('[{"alertName": "A", "runbookUrl": null}]')).toEqual([
      { name: "", runbookUrl: null, alerts: ["A"] },
    ]);
  });

  it("reads an older alertName with a comma as one alert, never splitting it", () => {
    expect(parseRunbookImport('[{"alertName": "CPU > 90%, prod", "runbookUrl": null}]')).toEqual([
      { name: "", runbookUrl: null, alerts: ["CPU > 90%, prod"] },
    ]);
  });

  it("refuses bad JSON, entries with no alert and links that are not http(s)", () => {
    expect(() => parseRunbookImport("{")).toThrow("not valid JSON");
    expect(() => parseRunbookImport('[{"alerts": [" "]}]')).toThrow("Entry 1 needs at least one alert name");
    expect(() => parseRunbookImport('{" ": "https://x.example"}')).toThrow("at least one alert name");
    expect(() => parseRunbookImport('{"A": "javascript:alert(1)"}')).toThrow("http(s)");
  });
});

describe("isValidRunbookUrl", () => {
  it("takes a full http(s) link, or nothing", () => {
    expect(isValidRunbookUrl("https://docs.example.com/a")).toBe(true);
    expect(isValidRunbookUrl("HTTP://docs.example.com")).toBe(true);
    expect(isValidRunbookUrl("  ")).toBe(true);
  });

  it("refuses a link without a written-out host, which the server would refuse", () => {
    for (const link of ["http:foo", "https:docs.example.com", "http:///foo", "http://", "docs.example.com", "ftp://x.example"]) {
      expect(isValidRunbookUrl(link), link).toBe(false);
    }
  });
});

describe("cleanAlertNames", () => {
  it("trims and drops blanks and repeats, ignoring case", () => {
    expect(cleanAlertNames([" a ", "", "A", "b"])).toEqual(["a", "b"]);
  });
});

describe("findRunbookForIncident", () => {
  const catalog = [
    { number: "RB0000001", runbookUrl: "https://docs.example.com/gw", alerts: ["gw-5xx", "gw-latency"] },
    { number: "RB0000002", runbookUrl: "https://docs.example.com/long", alerts: ["gw-5xx-prod-001"] },
    { number: "RB0000003", runbookUrl: null, alerts: ["JVM Heap"] },
  ];

  it("picks the runbook holding the longest alert name the subject contains, ignoring case", () => {
    const match = findRunbookForIncident(catalog, "Fired GW-5XX-PROD-001 on log");
    expect(match?.runbook.number).toBe("RB0000002");
    expect(match?.alertName).toBe("gw-5xx-prod-001");
    expect(findRunbookForIncident(catalog, "gw-latency spike")?.runbook.number).toBe("RB0000001");
  });

  it("finds a runbook without a link, and nothing for an unknown subject", () => {
    expect(findRunbookForIncident(catalog, "node / jvm heap")?.runbook.runbookUrl).toBeNull();
    expect(findRunbookForIncident(catalog, "unrelated")).toBeUndefined();
  });
});

describe("findAlertsContaining", () => {
  const catalog = [
    { number: "RB0000001", runbookUrl: "https://docs.example.com/gw", alerts: ["Gateway Latency High", "gw-5xx"] },
    { number: "RB0000002", runbookUrl: null, alerts: ["Gateway Latency"] },
  ];

  it("lists the alerts whose name contains a partial name, shortest first", () => {
    expect(findAlertsContaining(catalog, "gateway LATENCY").map((m) => [m.runbook.number, m.alertName])).toEqual([
      ["RB0000002", "Gateway Latency"],
      ["RB0000001", "Gateway Latency High"],
    ]);
    expect(findAlertsContaining(catalog, "nothing")).toEqual([]);
    expect(findAlertsContaining(catalog, "  ")).toEqual([]);
  });
});

describe("filterRunbooks", () => {
  const now = Date.parse("2026-10-20T00:00:00Z");
  const rb = (id: string, over: Partial<Runbook>): Runbook => ({
    id,
    number: `RB-${id}`,
    name: `Name ${id}`,
    runbookUrl: null,
    alerts: ["a"],
    createdOn: "2026-01-01T00:00:00Z",
    createdBy: "x@wso2.com",
    updatedOn: "2026-10-19T00:00:00Z",
    updatedBy: "x@wso2.com",
    ...over,
  });
  const catalog = [
    rb("gw", { runbookUrl: "https://d/gw", alerts: ["gw-5xx", "gw-latency"] }),
    rb("jvm", { alerts: ["JVM Heap"], updatedOn: "2026-09-01T00:00:00Z" }),
    rb("disk", { runbookUrl: "https://d/disk", alerts: ["disk"], updatedOn: "2026-05-01T00:00:00Z" }),
  ];
  const ids = (filters: Partial<typeof DEFAULT_RUNBOOK_FILTERS>, search = "") =>
    filterRunbooks(catalog, search, { ...DEFAULT_RUNBOOK_FILTERS, ...filters }, now).map((r) => r.id);

  it("filters by link, alert count and age", () => {
    expect(ids({})).toEqual(["gw", "jvm", "disk"]);
    expect(ids({ coverage: "linked" })).toEqual(["gw", "disk"]);
    expect(ids({ coverage: "missing" })).toEqual(["jvm"]);
    expect(ids({ alertCount: "single" })).toEqual(["jvm", "disk"]);
    expect(ids({ alertCount: "multiple" })).toEqual(["gw"]);
    expect(ids({ updatedWithin: "7" })).toEqual(["gw"]);
    expect(ids({ updatedWithin: "90" })).toEqual(["gw", "jvm"]);
    expect(ids({ coverage: "linked", alertCount: "single" })).toEqual(["disk"]);
  });

  it("searches number, alert names and link together with the filters", () => {
    expect(ids({}, "rb-jvm")).toEqual(["jvm"]);
    expect(ids({}, "NAME DISK")).toEqual(["disk"]);
    expect(ids({}, "LATENCY")).toEqual(["gw"]);
    expect(ids({}, "d/disk")).toEqual(["disk"]);
    expect(ids({ coverage: "missing" }, "gw")).toEqual([]);
  });

  it("counts the filters that differ from the default", () => {
    expect(activeRunbookFilterCount(DEFAULT_RUNBOOK_FILTERS)).toBe(0);
    expect(activeRunbookFilterCount({ ...DEFAULT_RUNBOOK_FILTERS, alertCount: "multiple", updatedWithin: "7" })).toBe(2);
  });
});

describe("export", () => {
  const catalog: Runbook[] = [
    {
      id: "1",
      number: "RB0000002",
      name: "Gateway",
      alerts: ["gw-5xx", "CPU > 90%, prod"],
      runbookUrl: "https://docs.example.com/gw",
      createdOn: "",
      createdBy: "",
      updatedOn: "2026-10-01T00:00:00Z",
      updatedBy: "a@x",
    },
    { id: "2", number: "RB0000001", name: "Proto", alerts: ["__proto__"], runbookUrl: null, createdOn: "", createdBy: "", updatedOn: "2026-10-02T00:00:00Z", updatedBy: "b@x" },
  ];

  it("writes JSON the importer reads back, alert names with commas and __proto__ included", () => {
    expect(parseRunbookImport(runbooksToImportJson(catalog))).toEqual([
      { name: "Gateway", runbookUrl: "https://docs.example.com/gw", alerts: ["gw-5xx", "CPU > 90%, prod"] },
      { name: "Proto", runbookUrl: null, alerts: ["__proto__"] },
    ]);
    expect(JSON.parse(runbooksToImportJson(catalog))[0].number).toBe("RB0000002");
  });

  it("writes one CSV row per runbook, its alerts one per line in one cell", () => {
    expect(runbooksToCsvRows(catalog)).toEqual([
      ["RB0000002", "Gateway", "gw-5xx\nCPU > 90%, prod", "https://docs.example.com/gw", "2026-10-01T00:00:00Z"],
      ["RB0000001", "Proto", "__proto__", "", "2026-10-02T00:00:00Z"],
    ]);
  });
});
