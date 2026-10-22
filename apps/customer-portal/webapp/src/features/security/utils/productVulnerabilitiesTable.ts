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

import {
  PRODUCT_VULNERABILITIES_CLEAR_FILTERS_LABEL,
  PRODUCT_VULNERABILITIES_MAX_PAGE_SIZE,
} from "@features/security/constants/securityConstants";
import type {
  ProductVulnerabilitiesSearchFilters,
  ProductVulnerabilitiesSearchRequest,
} from "@features/security/types/security";

/**
 * Converts the severity select value to the numeric id the search API expects.
 *
 * @param value - Select value (a string id from the options, or empty).
 * @returns {number | undefined} The id, or undefined when empty or not numeric.
 */
export function toSeverityFilterId(
  value: string | number | undefined,
): number | undefined {
  if (value === undefined || value === "") return undefined;
  const id = Number(value);
  return Number.isFinite(id) ? id : undefined;
}

/**
 * Builds the server-side search request for one page of the table. Empty
 * filters are omitted and the page size never exceeds the endpoint maximum.
 *
 * @param searchText - Debounced search text.
 * @param filters - Raw filter state (severityId, productName, productVersion).
 * @param page - Zero-based page index.
 * @param rowsPerPage - Requested page size.
 * @returns {ProductVulnerabilitiesSearchRequest} The request body.
 */
export function buildProductVulnerabilitiesSearchRequest(
  searchText: string,
  filters: Record<string, string | number>,
  page: number,
  rowsPerPage: number,
): ProductVulnerabilitiesSearchRequest {
  const limit = Math.min(
    Math.max(rowsPerPage, 1),
    PRODUCT_VULNERABILITIES_MAX_PAGE_SIZE,
  );
  const searchQuery = searchText.trim();
  const severityId = toSeverityFilterId(filters.severityId);
  const productName = filters.productName ? String(filters.productName) : "";
  const productVersion = filters.productVersion
    ? String(filters.productVersion)
    : "";

  const requestFilters: ProductVulnerabilitiesSearchFilters = {
    ...(searchQuery ? { searchQuery } : {}),
    ...(severityId !== undefined ? { severityId } : {}),
    ...(productName ? { productName } : {}),
    ...(productVersion ? { productVersion } : {}),
  };

  return {
    filters: requestFilters,
    pagination: { offset: page * limit, limit },
  };
}

/**
 * Counts search, severity, product name, and product version filters for the product vulnerabilities table header.
 *
 * @param searchInput - Current search text.
 * @param filters - Raw filter state.
 * @returns {number} Number of active filters (including non-empty search).
 */
export function countProductVulnerabilityTableActiveFilters(
  searchInput: string,
  filters: Record<string, string | number>,
): number {
  const searchCount = searchInput.trim() ? 1 : 0;
  const severityCount = filters.severityId ? 1 : 0;
  const productCount = filters.productName ? 1 : 0;
  const productVersionCount = filters.productVersion ? 1 : 0;
  return searchCount + severityCount + productCount + productVersionCount;
}

/**
 * @param count - Number of active filters (for the clear button label).
 * @returns {string} Localized clear-filters control label.
 */
export function formatProductVulnerabilitiesClearFiltersLabel(
  count: number,
): string {
  return `${PRODUCT_VULNERABILITIES_CLEAR_FILTERS_LABEL} (${count})`;
}
