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
import { getSeverityDisplay } from "@features/support/utils/pendingVerification";

// entity-service sends the lowercase domain severity word (matching
// Case.severity/CaseView.severity everywhere else in the API), and this
// util must format it to match the case list's own severity chip exactly
// (mapSeverityToDisplay in support.ts) -- short code only, no spelled-out
// word, "S4(Query)" for Low.
describe("getSeverityDisplay", () => {
  it("returns null when severity is absent", () => {
    expect(getSeverityDisplay(undefined)).toBeNull();
  });

  it("returns null for an unrecognized value", () => {
    expect(getSeverityDisplay("S1")).toBeNull();
    expect(getSeverityDisplay("not-a-severity")).toBeNull();
  });

  it.each([
    ["catastrophic", "S0"],
    ["critical", "S1"],
    ["high", "S2"],
    ["medium", "S3"],
    ["low", "S4(Query)"],
  ])("formats domain severity %s as %s, matching the case list", (severity, expectedLabel) => {
    expect(getSeverityDisplay(severity)?.label).toBe(expectedLabel);
  });

  it("is case-insensitive", () => {
    expect(getSeverityDisplay("CRITICAL")?.label).toBe("S1");
  });
});
