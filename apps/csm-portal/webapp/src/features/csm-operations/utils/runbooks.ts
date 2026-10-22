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

export interface Runbook {
  id: string;
  /** The runbook's own number (RB0000001), given by the backend and never changed. */
  number: string;
  /** Unique, ignoring case and surrounding spaces. */
  name: string;
  /** Null when the runbook has not been written yet. */
  runbookUrl: string | null;
  /** Alert titles; an incident whose subject contains one gets this runbook. */
  alerts: string[];
  createdOn: string;
  createdBy: string;
  updatedOn: string;
  updatedBy: string;
}

export interface RunbookInput {
  /** Required on add and edit; on import, empty keeps a matched runbook's name and names a new one after its first alert. */
  name: string;
  runbookUrl: string | null;
  alerts: string[];
}

export type RunbookCoverage = "all" | "linked" | "missing";
export type RunbookAlertCount = "all" | "single" | "multiple";
export type RunbookUpdatedWithin = "all" | "7" | "30" | "90";

export interface RunbookFilters {
  coverage: RunbookCoverage;
  alertCount: RunbookAlertCount;
  updatedWithin: RunbookUpdatedWithin;
}

export const DEFAULT_RUNBOOK_FILTERS: RunbookFilters = { coverage: "all", alertCount: "all", updatedWithin: "all" };

/** How many filters differ from their default. */
export function activeRunbookFilterCount(filters: RunbookFilters): number {
  return (Object.keys(DEFAULT_RUNBOOK_FILTERS) as (keyof RunbookFilters)[]).filter(
    (k) => filters[k] !== DEFAULT_RUNBOOK_FILTERS[k],
  ).length;
}

const DAY_MS = 24 * 60 * 60 * 1000;

/** The runbooks matching the search text (number, alert name or link) and every filter. */
export function filterRunbooks(runbooks: Runbook[], search: string, filters: RunbookFilters, now = Date.now()): Runbook[] {
  const term = search.trim().toLowerCase();
  const since = filters.updatedWithin === "all" ? undefined : now - Number(filters.updatedWithin) * DAY_MS;
  return runbooks.filter((r) => {
    if (filters.coverage === "linked" && !r.runbookUrl) return false;
    if (filters.coverage === "missing" && r.runbookUrl) return false;
    if (filters.alertCount === "single" && r.alerts.length !== 1) return false;
    if (filters.alertCount === "multiple" && r.alerts.length < 2) return false;
    if (since !== undefined) {
      const updated = Date.parse(r.updatedOn);
      if (Number.isNaN(updated) || updated < since) return false;
    }
    return (
      !term ||
      r.number.toLowerCase().includes(term) ||
      r.name.toLowerCase().includes(term) ||
      r.alerts.some((a) => a.toLowerCase().includes(term)) ||
      (r.runbookUrl ?? "").toLowerCase().includes(term)
    );
  });
}

export function isValidRunbookUrl(value: string): boolean {
  const link = value.trim();
  if (!link) return true;
  // The browser parser turns "http:foo" or "http:///foo" into a host; entity-service's parser does not, so the authority must be written out.
  if (!/^https?:\/\/[^/?#\s]/i.test(link)) return false;
  try {
    return new URL(link).host !== "";
  } catch {
    return false;
  }
}

/** Trimmed alert names with blanks and repeats (ignoring case) dropped, in their first order. */
export function cleanAlertNames(names: string[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const raw of names) {
    const name = raw.trim();
    const key = name.toLowerCase();
    if (!name || seen.has(key)) continue;
    seen.add(key);
    out.push(name);
  }
  return out;
}

export interface RunbookMatch<T extends Pick<Runbook, "runbookUrl" | "alerts">> {
  runbook: T;
  alertName: string;
}

/** The runbook for an incident: the one holding the longest alert name its subject contains, case-insensitively. Mirrors entity-service. */
export function findRunbookForIncident<T extends Pick<Runbook, "runbookUrl" | "alerts">>(
  runbooks: T[],
  subject: string,
): RunbookMatch<T> | undefined {
  const haystack = subject.toLowerCase();
  let best: RunbookMatch<T> | undefined;
  let bestLen = 0;
  for (const runbook of runbooks) {
    for (const alertName of runbook.alerts) {
      const needle = alertName.trim().toLowerCase();
      if (needle && needle.length > bestLen && haystack.includes(needle)) {
        best = { runbook, alertName };
        bestLen = needle.length;
      }
    }
  }
  return best;
}

/** Alerts whose name contains the query, ignoring case, shortest first: for a partial alert name that no alert is contained in. */
export function findAlertsContaining<T extends Pick<Runbook, "runbookUrl" | "alerts">>(
  runbooks: T[],
  query: string,
  limit = 5,
): RunbookMatch<T>[] {
  const q = query.trim().toLowerCase();
  if (!q) return [];
  return runbooks
    .flatMap((runbook) => runbook.alerts.filter((a) => a.toLowerCase().includes(q)).map((alertName) => ({ runbook, alertName })))
    .sort((a, b) => a.alertName.length - b.alertName.length)
    .slice(0, limit);
}

/**
 * Reads an import file: a JSON object whose keys are alert names and whose values
 * are links ("" for none); or an array of {name?, runbookUrl, alerts}, where alerts
 * is a list (an older {alertName, runbookUrl} entry is read as one alert). A key or
 * a text alerts value is one alert name, commas included, so several alerts need
 * the list form. An entry with no name keeps a matched runbook's name and names a
 * new one after its first alert. Throws with a readable message otherwise.
 */
export function parseRunbookImport(text: string): RunbookInput[] {
  let data: unknown;
  try {
    data = JSON.parse(text);
  } catch {
    throw new Error("The file is not valid JSON.");
  }
  const toInput = (name: unknown, alerts: unknown, url: unknown, label: string): RunbookInput => {
    const alertList = cleanAlertNames(
      typeof alerts === "string" ? [alerts] : Array.isArray(alerts) ? alerts.map((a) => (typeof a === "string" ? a : "")) : [],
    );
    if (alertList.length === 0) {
      throw new Error(`${label} needs at least one alert name.`);
    }
    if (url !== null && url !== undefined && typeof url !== "string") {
      throw new Error(`The link for "${alertList[0]}" is not text.`);
    }
    const link = typeof url === "string" ? url.trim() : "";
    if (!isValidRunbookUrl(link)) {
      throw new Error(`The link for "${alertList[0]}" is not a full http(s) link.`);
    }
    return { name: typeof name === "string" ? name.trim() : "", runbookUrl: link || null, alerts: alertList };
  };
  if (Array.isArray(data)) {
    return data.map((row, i) => {
      const r = (row ?? {}) as Record<string, unknown>;
      return toInput(r.name, r.alerts ?? (typeof r.alertName === "string" ? [r.alertName] : r.alertName), r.runbookUrl, `Entry ${i + 1}`);
    });
  }
  if (data && typeof data === "object") {
    return Object.entries(data as Record<string, unknown>).map(([key, url]) => toInput(undefined, key, url, `"${key}"`));
  }
  throw new Error("Expected an object of alert names to link, or a list of runbooks.");
}

/** Runbooks as an import file: a list of {number, name, runbookUrl, alerts}; the import ignores the number and matches by alert. */
export function runbooksToImportJson(runbooks: Pick<Runbook, "number" | "name" | "runbookUrl" | "alerts">[]): string {
  const out = runbooks.map((rb) => ({ number: rb.number, name: rb.name, runbookUrl: rb.runbookUrl, alerts: rb.alerts }));
  return `${JSON.stringify(out, null, 2)}\n`;
}

export const RUNBOOK_CSV_HEADER = ["Number", "Name", "Alerts", "Runbook link", "Last updated"];

/** One CSV row per runbook, in the order of RUNBOOK_CSV_HEADER; the alerts share a cell, one per line. */
export function runbooksToCsvRows(runbooks: Runbook[]): string[][] {
  return runbooks.map((rb) => [rb.number, rb.name, rb.alerts.join("\n"), rb.runbookUrl ?? "", rb.updatedOn]);
}
