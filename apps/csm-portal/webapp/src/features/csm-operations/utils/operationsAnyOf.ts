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

/** One condition of an OR group: a field and the values it may hold ("is one of"). */
export interface AnyOfCondition {
  field: string;
  values: string[];
}

/** An OR group: its conditions must all match; any one group may match. */
export interface AnyOfGroup {
  conditions: AnyOfCondition[];
}

// Bounds kept in step with the entity service's (10 branches of 10 filters).
const MAX_GROUPS = 10;
const MAX_CONDITIONS = 10;
const MAX_VALUES = 100;
const MAX_VALUE_LEN = 200;

/** The groups that would be sent: conditions without values dropped, and
 * groups left with none dropped (an empty group would match everything). */
export function completeAnyOfGroups(groups: AnyOfGroup[]): AnyOfGroup[] {
  return groups
    .map((g) => ({ conditions: g.conditions.filter((c) => c.field && c.values.length > 0) }))
    .filter((g) => g.conditions.length > 0);
}

/** `filters.anyOf` for the search request, or undefined when there is none. */
export function anyOfPayload(groups: AnyOfGroup[]): Array<{ filters: Array<{ field: string; op: "in"; values: string[] }> }> | undefined {
  const complete = completeAnyOfGroups(groups);
  if (complete.length === 0) return undefined;
  return complete.map((g) => ({
    filters: g.conditions.map((c) => ({ field: c.field, op: "in" as const, values: c.values })),
  }));
}

/** URL form: `[[[field, [values]], ...], ...]` as JSON, complete groups only. */
export function writeAnyOfParam(groups: AnyOfGroup[]): string | null {
  const complete = completeAnyOfGroups(groups);
  if (complete.length === 0) return null;
  return JSON.stringify(complete.map((g) => g.conditions.map((c) => [c.field, c.values])));
}

/** Reads the URL form back, keeping only the listed fields and bounded sizes;
 * anything malformed is dropped rather than failing the page. */
export function parseAnyOfParam(raw: string | null, allowedFields: readonly string[]): AnyOfGroup[] {
  if (!raw) return [];
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return [];
  }
  if (!Array.isArray(parsed)) return [];
  const groups: AnyOfGroup[] = [];
  for (const g of parsed.slice(0, MAX_GROUPS)) {
    if (!Array.isArray(g)) continue;
    const conditions: AnyOfCondition[] = [];
    for (const c of g.slice(0, MAX_CONDITIONS)) {
      if (!Array.isArray(c) || c.length !== 2) continue;
      const [field, values] = c as [unknown, unknown];
      if (typeof field !== "string" || !allowedFields.includes(field) || !Array.isArray(values)) continue;
      const vs = values
        .filter((v): v is string => typeof v === "string" && v.length > 0)
        .map((v) => v.slice(0, MAX_VALUE_LEN))
        .slice(0, MAX_VALUES);
      if (vs.length > 0) conditions.push({ field, values: vs });
    }
    if (conditions.length > 0) groups.push({ conditions });
  }
  return groups;
}
