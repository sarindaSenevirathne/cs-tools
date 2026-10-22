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
  anyOfPayload,
  completeAnyOfGroups,
  parseAnyOfParam,
  writeAnyOfParam,
} from "@features/csm-operations/utils/operationsAnyOf";

describe("operationsAnyOf", () => {
  const groups = [
    { conditions: [{ field: "state", values: ["NEW"] }, { field: "priority", values: [] }] },
    { conditions: [{ field: "category", values: [] }] },
    { conditions: [{ field: "assignmentGroupId", values: ["g-1", "g-2"] }] },
  ];

  it("drops conditions with no values and groups left empty", () => {
    expect(completeAnyOfGroups(groups)).toEqual([
      { conditions: [{ field: "state", values: ["NEW"] }] },
      { conditions: [{ field: "assignmentGroupId", values: ["g-1", "g-2"] }] },
    ]);
  });

  it("builds filters.anyOf with op in, or nothing when no group is complete", () => {
    expect(anyOfPayload(groups)).toEqual([
      { filters: [{ field: "state", op: "in", values: ["NEW"] }] },
      { filters: [{ field: "assignmentGroupId", op: "in", values: ["g-1", "g-2"] }] },
    ]);
    expect(anyOfPayload([{ conditions: [{ field: "state", values: [] }] }])).toBeUndefined();
  });

  it("round-trips through the URL and ignores fields that are not allowed", () => {
    const raw = writeAnyOfParam(groups);
    expect(parseAnyOfParam(raw, ["state", "assignmentGroupId"])).toEqual(completeAnyOfGroups(groups));
    expect(parseAnyOfParam(raw, ["state"])).toEqual([{ conditions: [{ field: "state", values: ["NEW"] }] }]);
    expect(parseAnyOfParam("not json", ["state"])).toEqual([]);
    expect(writeAnyOfParam([])).toBeNull();
  });
});
