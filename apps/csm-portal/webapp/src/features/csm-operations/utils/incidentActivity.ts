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

import type { CaseAuditEntry, CaseAuditFieldChange } from "@features/csm-cases/types/csmCases";

/** Fields priority is computed from: shown under the priority change they moved. */
const PRIORITY_DRIVERS = new Set(["impact", "urgency"]);

/** The order a save's changes are listed in -- the entity service's own (incident_activity.go). */
const FIELD_ORDER = [
  "state",
  "priority",
  "impact",
  "urgency",
  "category",
  "subcategory",
  "contact_type",
  "resolution_code",
  "subject",
  "environment",
  "assigned_to_id",
  "assignment_group_id",
  "service_id",
  "service_offering_id",
  "configuration_item_id",
  "parent",
  "parent_incident_id",
  "change_request_id",
  "problem_id",
  "caused_by_id",
];

function fieldRank(field: string): number {
  const i = FIELD_ORDER.indexOf(field);
  return i === -1 ? FIELD_ORDER.length : i;
}

/** One save writes all its rows in one statement, so they share a timestamp and an actor. */
function saveKey(entry: CaseAuditEntry): string {
  return `${entry.createdAt}\u0000${entry.actorUser?.email ?? entry.actor}`;
}

/** Lists a save's changes in field order, with impact/urgency folded under a priority change. */
function foldPriorityDrivers(entry: CaseAuditEntry): CaseAuditEntry {
  const changes = [...(entry.changes ?? [])].sort((a, b) => fieldRank(a.field) - fieldRank(b.field));
  const priority = changes.find((c) => c.field === "priority");
  const drivers = changes.filter((c) => PRIORITY_DRIVERS.has(c.field));
  if (!priority || drivers.length === 0) return { ...entry, changes };
  return {
    ...entry,
    changes: changes
      .filter((c) => !PRIORITY_DRIVERS.has(c.field))
      .map((c): CaseAuditFieldChange => (c === priority ? { ...c, details: drivers } : c)),
  };
}

/**
 * Groups an incident's field-change entries into one entry per save. The entity service writes one
 * row per field, so an edit of three fields would otherwise read as three separate feed items.
 * Within a save, an impact/urgency change that moved priority is shown under that priority change
 * (`details`); one that left priority where it was stays its own line. Entries keep the order of
 * each save's first row.
 */
export function groupIncidentActivity(entries: CaseAuditEntry[]): CaseAuditEntry[] {
  const groups = new Map<string, CaseAuditEntry>();
  const out: CaseAuditEntry[] = [];
  for (const entry of entries) {
    if (!entry.changes || entry.changes.length === 0) {
      out.push(entry);
      continue;
    }
    const key = saveKey(entry);
    const group = groups.get(key);
    if (group) {
      group.changes = [...(group.changes ?? []), ...entry.changes];
      continue;
    }
    const copy = { ...entry, changes: [...entry.changes] };
    groups.set(key, copy);
    out.push(copy);
  }
  return out.map((e) => (e.changes && e.changes.length > 0 ? foldPriorityDrivers(e) : e));
}
