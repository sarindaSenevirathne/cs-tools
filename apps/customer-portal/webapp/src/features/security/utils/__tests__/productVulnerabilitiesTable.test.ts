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
  buildProductVulnerabilitiesSearchRequest,
  countProductVulnerabilityTableActiveFilters,
  formatProductVulnerabilitiesClearFiltersLabel,
  toSeverityFilterId,
} from "@features/security/utils/productVulnerabilitiesTable";

describe("productVulnerabilitiesTable utils", () => {
  it("counts active filters and trimmed search input", () => {
    expect(
      countProductVulnerabilityTableActiveFilters("  CVE  ", {
        severityId: 1,
        productName: "APIM",
        productVersion: "4.2.0",
      }),
    ).toBe(4);
    expect(countProductVulnerabilityTableActiveFilters(" ", {})).toBe(0);
  });

  it("formats clear-filter label with count", () => {
    expect(formatProductVulnerabilitiesClearFiltersLabel(3)).toBe(
      "Clear Filters (3)",
    );
  });

  it("converts the severity select value to a number and guards NaN", () => {
    expect(toSeverityFilterId("4")).toBe(4);
    expect(toSeverityFilterId(3)).toBe(3);
    expect(toSeverityFilterId("")).toBeUndefined();
    expect(toSeverityFilterId(undefined)).toBeUndefined();
    expect(toSeverityFilterId("critical")).toBeUndefined();
  });

  it("builds a paged request that omits empty filters", () => {
    expect(buildProductVulnerabilitiesSearchRequest("", {}, 0, 10)).toEqual({
      filters: {},
      pagination: { offset: 0, limit: 10 },
    });
    expect(
      buildProductVulnerabilitiesSearchRequest(
        "  log4j ",
        { severityId: "4", productName: "product-a", productVersion: "" },
        3,
        25,
      ),
    ).toEqual({
      filters: { searchQuery: "log4j", severityId: 4, productName: "product-a" },
      pagination: { offset: 75, limit: 25 },
    });
  });

  it("never requests more than 50 rows and drops a non-numeric severity", () => {
    expect(
      buildProductVulnerabilitiesSearchRequest("", { severityId: "x" }, 2, 500),
    ).toEqual({ filters: {}, pagination: { offset: 100, limit: 50 } });
  });
});
