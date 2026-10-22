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

function parseDate(value?: string | null): Date | null {
  if (!value) return null;
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? null : d;
}

// `createdOn` is a full timestamp (e.g. `10:23:00Z`) while `startDate` is
// effectively a date (`00:00:00Z`). Comparing the two instants directly would
// make almost every never-renewed project look renewed, since any non-zero
// time of day on `createdOn` sorts after midnight. Reduce both to a local
// calendar-day key first, so the comparison matches what the viewer actually
// sees on screen (the pages render both dates in the local time zone).
function localDayKey(d: Date): number {
  return d.getFullYear() * 10000 + (d.getMonth() + 1) * 100 + d.getDate();
}

/**
 * Label for the `startDate` cell: a project has been renewed only if
 * `startDate` falls on a later calendar day than `createdOn`. Same-day, a
 * reversed (earlier `startDate`) pair, or missing/unparseable input all fall
 * back to "Started on" — never claim a renewal the dates can't substantiate.
 */
export function startDateLabel(
  createdOn?: string | null,
  startDate?: string | null,
): "Started on" | "Renewed on" {
  const created = parseDate(createdOn);
  const start = parseDate(startDate);
  if (!created || !start) return "Started on";
  return localDayKey(start) > localDayKey(created) ? "Renewed on" : "Started on";
}

/**
 * Label for the `endDate` cell: tense depends on whether the date has passed.
 * Missing/unparseable input defaults to "Ends on" (the value itself renders
 * as an em dash). `now` is injectable so callers can test deterministically.
 */
export function endDateLabel(
  endDate?: string | null,
  now: Date = new Date(),
): "Ends on" | "Ended on" {
  const end = parseDate(endDate);
  if (!end) return "Ends on";
  return end.getTime() < now.getTime() ? "Ended on" : "Ends on";
}

export type ClosureStateSeverity = "success" | "warning" | "error" | "default";

export interface ClosureStatePresentation {
  label: string;
  severity: ClosureStateSeverity;
}

const CLOSURE_STATE_SEVERITY: Record<string, ClosureStateSeverity> = {
  open: "success",
  notify: "warning",
  read_only: "warning",
  restricted: "warning",
  suspended: "error",
  closed: "error",
};

// Humanises a snake_case value into sentence case, e.g. `read_only` ->
// "Read only". Used for both known and unknown closure-state values.
function humanize(value: string): string {
  const spaced = value.replace(/_/g, " ").toLowerCase();
  return spaced.charAt(0).toUpperCase() + spaced.slice(1);
}

/**
 * Chip presentation for a project's `closureState`. Returns `null` for a
 * missing/empty state (the caller renders an em dash). The field is
 * free-form, so any value not in the known set still renders — as a neutral
 * chip with the raw value humanised — rather than crashing or rendering
 * blank.
 */
export function closureStatePresentation(
  closureState?: string | null,
): ClosureStatePresentation | null {
  const trimmed = closureState?.trim();
  if (!trimmed) return null;
  // Match the severity case-insensitively: the field is free-form, so a value
  // that differs only in casing (`Open`, `READ_ONLY`) must still colour the
  // chip rather than silently degrading to neutral.
  return {
    label: humanize(trimmed),
    severity: CLOSURE_STATE_SEVERITY[trimmed.toLowerCase()] ?? "default",
  };
}

export type ProjectOnboardingStatusSeverity = "success" | "info" | "warning" | "error" | "default";

export interface ProjectOnboardingStatusPresentation {
  label: string;
  severity: ProjectOnboardingStatusSeverity;
}

const PROJECT_ONBOARDING_STATUS_PRESENTATION: Record<string, ProjectOnboardingStatusPresentation> = {
  "In-Progress": { label: "In Progress", severity: "info" },
  Completed: { label: "Completed", severity: "success" },
  "Not-Started": { label: "Not Started", severity: "default" },
  Expired: { label: "Expired", severity: "warning" },
  Cancelled: { label: "Cancelled", severity: "default" },
  OnHold: { label: "On Hold", severity: "warning" },
  "On-Hold": { label: "On Hold", severity: "warning" },
};

/**
 * Chip presentation for a project's `onboardingStatus`. Returns `null` for a
 * missing status or `"Not-Applicable"` (the project has no onboarding
 * engagement at all, so there is nothing to show — same gate
 * `CsmProjectDetailPage.tsx`'s own onboarding owner cell already uses). An
 * upstream value this build does not know still renders — as a neutral chip
 * with the raw value humanised — rather than crashing or rendering blank.
 */
export function projectOnboardingStatusPresentation(
  onboardingStatus?: string | null,
): ProjectOnboardingStatusPresentation | null {
  if (!onboardingStatus || onboardingStatus === "Not-Applicable") return null;
  return PROJECT_ONBOARDING_STATUS_PRESENTATION[onboardingStatus] ?? {
    label: humanize(onboardingStatus),
    severity: "default",
  };
}
