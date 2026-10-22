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

import { ListingTable, Divider, Box } from "@wso2/oxygen-ui";
import { type JSX, useState, useMemo, useEffect } from "react";
import { usePostProductVulnerabilitiesSearch } from "@features/security/api/usePostProductVulnerabilitiesSearch";
import { useGetVulnerabilitiesMetaData } from "@features/security/api/useGetVulnerabilitiesMetaData";
import { useDebouncedValue } from "@hooks/useDebouncedValue";
import ProductVulnerabilitiesTableHeader from "@features/security/components/ProductVulnerabilitiesTableHeader";
import ProductVulnerabilitiesFilters from "@features/security/components/ProductVulnerabilitiesFilters";
import ProductVulnerabilitiesList from "@features/security/components/ProductVulnerabilitiesList";
import {
  PRODUCT_VULNERABILITIES_DEFAULT_ROWS_PER_PAGE,
  PRODUCT_VULNERABILITIES_SEARCH_DEBOUNCE_MS,
} from "@features/security/constants/securityConstants";
import type { ProductVulnerabilitiesTableProps } from "@features/security/types/security";
import {
  buildProductVulnerabilitiesSearchRequest,
  countProductVulnerabilityTableActiveFilters,
} from "@features/security/utils/productVulnerabilitiesTable";

/**
 * Product Vulnerabilities table.
 *
 * Architecture note:
 * - Search, filters and pagination are all server-side: every change issues a
 *   search request for one page (at most 50 rows, the endpoint maximum).
 * - The pager and the reported total use the server's `totalRecords`.
 * - The previous page stays on screen while the next one loads.
 * - Product and version dropdown options come from the metadata request. If it
 *   fails the dropdowns are simply empty; the table itself is unaffected.
 */
const ProductVulnerabilitiesTable = ({
  onTotalRecordsChange,
  onError,
  onVulnerabilityClick,
}: ProductVulnerabilitiesTableProps): JSX.Element => {
  const [searchInput, setSearchInput] = useState("");
  const debouncedSearch = useDebouncedValue(
    searchInput,
    PRODUCT_VULNERABILITIES_SEARCH_DEBOUNCE_MS,
  );
  const [filters, setFilters] = useState<Record<string, string | number>>({});
  const [isFilterOpen, setIsFilterOpen] = useState(false);
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(
    PRODUCT_VULNERABILITIES_DEFAULT_ROWS_PER_PAGE,
  );

  // ── Dropdown metadata (severities, products and their versions) ─────────────
  const { data: metaData } = useGetVulnerabilitiesMetaData();
  const severityOptions = useMemo(
    () =>
      metaData?.severities?.map((s) => ({ value: s.id, label: s.label })) ?? [],
    [metaData?.severities],
  );
  const productOptions = useMemo(
    () =>
      metaData?.products?.map((p) => ({ value: p.name, label: p.name })) ?? [],
    [metaData?.products],
  );
  const productVersionOptions = useMemo(() => {
    if (!filters.productName) return [];
    const product = metaData?.products?.find(
      (p) => p.name === filters.productName,
    );
    return (product?.versions ?? []).map((v) => ({ value: v, label: v }));
  }, [metaData?.products, filters.productName]);

  // ── Current page, fetched from the server ───────────────────────────────────
  const request = useMemo(
    () =>
      buildProductVulnerabilitiesSearchRequest(
        debouncedSearch,
        filters,
        page,
        rowsPerPage,
      ),
    [debouncedSearch, filters, page, rowsPerPage],
  );
  const { data, isLoading, isError } = usePostProductVulnerabilitiesSearch(
    request,
    { keepPreviousData: true },
  );

  const listData = useMemo(
    () => ({
      vulnerabilities: data?.productVulnerabilities ?? [],
      totalRecords: data?.totalRecords ?? 0,
    }),
    [data],
  );

  useEffect(() => {
    onTotalRecordsChange?.(listData.totalRecords);
  }, [listData.totalRecords, onTotalRecordsChange]);

  useEffect(() => {
    onError?.(isError);
  }, [isError, onError]);

  // ── Handlers ────────────────────────────────────────────────────────────────
  const handleUpdateFilter = (field: string, value: string | number) => {
    setFilters((prev) => ({
      ...prev,
      [field]: value,
      ...(field === "productName" ? { productVersion: "" } : {}),
    }));
    setPage(0);
  };

  // Clears the filters only; the search text is kept.
  const handleClearFilters = () => {
    setFilters({});
    setPage(0);
  };

  const handleChangePage = (_event: unknown, newPage: number) => {
    setPage(newPage);
  };

  const handleChangeRowsPerPage = (
    event: React.ChangeEvent<HTMLInputElement>,
  ) => {
    setRowsPerPage(parseInt(event.target.value, 10));
    setPage(0);
  };

  const activeFilterCount = useMemo(
    () => countProductVulnerabilityTableActiveFilters("", filters),
    [filters],
  );

  // ── Render ───────────────────────────────────────────────────────────────────
  return (
    <ListingTable.Container sx={{ width: "100%", mb: 4, p: 3 }}>
      <ProductVulnerabilitiesTableHeader
        searchValue={searchInput}
        onSearchChange={(v) => {
          setSearchInput(v);
          setPage(0);
        }}
        onFilterToggle={() => {
          if (activeFilterCount > 0) {
            handleClearFilters();
          } else {
            setIsFilterOpen(!isFilterOpen);
          }
        }}
        isFiltersOpen={isFilterOpen}
        activeFiltersCount={activeFilterCount}
      />

      {!isLoading && isFilterOpen && (
        <>
          <Divider sx={{ my: 2 }} />
          <Box sx={{ mb: 3 }}>
            <ProductVulnerabilitiesFilters
              filters={filters}
              severityOptions={severityOptions}
              productOptions={productOptions}
              productVersionOptions={productVersionOptions}
              onFilterChange={handleUpdateFilter}
              onClearFilters={handleClearFilters}
            />
          </Box>
        </>
      )}

      <ProductVulnerabilitiesList
        isLoading={isLoading}
        isError={isError}
        data={listData}
        page={page}
        rowsPerPage={rowsPerPage}
        onPageChange={handleChangePage}
        onRowsPerPageChange={handleChangeRowsPerPage}
        onVulnerabilityClick={onVulnerabilityClick}
      />
    </ListingTable.Container>
  );
};

export default ProductVulnerabilitiesTable;
