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
import type { CaseAuditEntry } from "@features/csm-cases/types/csmCases";
import { groupIncidentActivity } from "@features/csm-operations/utils/incidentActivity";

const T1 = "2026-10-21T08:00:00Z";
const T2 = "2026-10-21T09:00:00Z";

function row(
  id: string,
  createdAt: string,
  email: string,
  field: string,
  previousValue: string,
  newValue: string,
): CaseAuditEntry {
  return {
    id,
    kind: "field_change",
    actor: email,
    actorUser: { id: null, email, name: email },
    createdAt,
    changes: [{ field, fieldLabel: field, previousValue, newValue }],
  };
}

describe("groupIncidentActivity", () => {
  it("makes one entry per save and folds impact/urgency under the priority they moved", () => {
    const got = groupIncidentActivity([
      row("1", T1, "jane@example.com", "urgency", "Low", "Medium"),
      row("2", T1, "jane@example.com", "priority", "Planning", "High"),
      row("3", T1, "jane@example.com", "state", "New", "In Progress"),
      row("4", T1, "jane@example.com", "impact", "Low", "High"),
    ]);
    expect(got).toHaveLength(1);
    expect(got[0].id).toBe("1");
    expect(got[0].changes?.map((c) => c.field)).toEqual(["state", "priority"]);
    expect(got[0].changes?.[1].details?.map((d) => [d.field, d.previousValue, d.newValue])).toEqual([
      ["impact", "Low", "High"],
      ["urgency", "Low", "Medium"],
    ]);
  });

  it("keeps an impact/urgency change its own line when priority did not move", () => {
    const got = groupIncidentActivity([row("1", T1, "jane@example.com", "urgency", "Low", "Medium")]);
    expect(got[0].changes).toEqual([
      { field: "urgency", fieldLabel: "urgency", previousValue: "Low", newValue: "Medium" },
    ]);
  });

  it("does not merge different saves or different people", () => {
    const got = groupIncidentActivity([
      row("1", T1, "jane@example.com", "state", "New", "In Progress"),
      row("2", T1, "john@example.com", "impact", "Low", "High"),
      row("3", T2, "jane@example.com", "priority", "Low", "Moderate"),
    ]);
    expect(got.map((e) => e.id)).toEqual(["1", "2", "3"]);
    expect(got[2].changes?.[0].details).toBeUndefined();
  });

  it("passes an entry without structured changes through untouched", () => {
    const legacy: CaseAuditEntry = { id: "x", kind: "created", actor: "System", createdAt: T1, description: "Created" };
    expect(groupIncidentActivity([legacy])).toEqual([legacy]);
  });
});
