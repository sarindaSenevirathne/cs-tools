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

import { fireEvent, render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import ProductVulnerabilitiesTable from "@features/security/components/ProductVulnerabilitiesTable";
import { PRODUCT_VULNERABILITIES_SEARCH_PLACEHOLDER } from "@features/security/constants/securityConstants";

const mockSearchHook = vi.fn();
const mockMetaHook = vi.fn();

vi.mock("@features/security/api/usePostProductVulnerabilitiesSearch", () => ({
  usePostProductVulnerabilitiesSearch: (...args: unknown[]) => mockSearchHook(...args),
}));

vi.mock("@features/security/api/useGetVulnerabilitiesMetaData", () => ({
  useGetVulnerabilitiesMetaData: () => mockMetaHook(),
}));

// Debounce is covered by its own hook; here the value passes straight through.
vi.mock("@hooks/useDebouncedValue", () => ({
  useDebouncedValue: (value: unknown) => value,
}));

const META = {
  severities: [
    { id: "4", label: "Critical" },
    { id: "3", label: "High" },
  ],
  products: [
    { name: "product-a", versions: ["4.10.0", "4.7.0"] },
    { name: "product-b", versions: ["1.0.0"] },
    { name: "product-no-versions", versions: [] },
  ],
};

const row = (n: number) => ({
  id: `id-${n}`,
  cveId: `CVE-2024-${1000 + n}`,
  vulnerabilityId: `VULN-${n}`,
  severity: { id: "4", label: "Critical" },
  productName: "product-a",
  productVersion: "4.10.0",
  componentName: `component-${n}`,
  version: "1.0.0",
});

/** The request the table passed to the search hook on its latest render. */
const lastRequest = () =>
  mockSearchHook.mock.calls[mockSearchHook.mock.calls.length - 1][0];

const openFilters = () =>
  fireEvent.click(screen.getByRole("button", { name: /^filters$/i }));

const LABEL_IDS: Record<string, string> = {
  Severity: "severity-label",
  Product: "product-label",
  "Product Version": "product-version-label",
};

const select = (label: string) =>
  screen
    .getAllByRole("combobox")
    .find((el) => el.getAttribute("aria-labelledby")?.includes(LABEL_IDS[label]))!;

const choose = (label: string, option: string) => {
  fireEvent.mouseDown(select(label));
  fireEvent.click(within(screen.getByRole("listbox")).getByText(option));
};

describe("ProductVulnerabilitiesTable", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockMetaHook.mockReturnValue({ data: META });
    mockSearchHook.mockReturnValue({
      data: { productVulnerabilities: [row(1), row(2)], totalRecords: 7279 },
      isLoading: false,
      isError: false,
    });
  });

  it("requests the first page with a capped size and keeps the previous page while loading", () => {
    render(<ProductVulnerabilitiesTable />);

    expect(lastRequest()).toEqual({
      filters: {},
      pagination: { offset: 0, limit: 10 },
    });
    expect(mockSearchHook.mock.calls[0][1]).toEqual({ keepPreviousData: true });
    expect(screen.getByText("CVE-2024-1001")).toBeInTheDocument();
  });

  it("uses the server totalRecords for the pager and the total callback", () => {
    const onTotalRecordsChange = vi.fn();
    render(<ProductVulnerabilitiesTable onTotalRecordsChange={onTotalRecordsChange} />);

    // Two rows are rendered, but the pager reflects the server total.
    expect(screen.getByText(/1.10 of 7279/)).toBeInTheDocument();
    expect(onTotalRecordsChange).toHaveBeenCalledWith(7279);
  });

  it("pages on the server with offset = page * size, never above 50 rows", () => {
    render(<ProductVulnerabilitiesTable />);

    fireEvent.click(screen.getByRole("button", { name: /next page/i }));
    expect(lastRequest().pagination).toEqual({ offset: 10, limit: 10 });

    fireEvent.mouseDown(screen.getByRole("combobox", { name: /rows per page/i }));
    fireEvent.click(within(screen.getByRole("listbox")).getByText("50"));
    expect(lastRequest().pagination).toEqual({ offset: 0, limit: 50 });

    fireEvent.click(screen.getByRole("button", { name: /next page/i }));
    expect(lastRequest().pagination).toEqual({ offset: 50, limit: 50 });
  });

  it("sends the search text and resets to the first page when it changes", () => {
    render(<ProductVulnerabilitiesTable />);

    fireEvent.click(screen.getByRole("button", { name: /next page/i }));
    fireEvent.click(screen.getByRole("button", { name: /next page/i }));
    expect(lastRequest().pagination.offset).toBe(20);

    fireEvent.change(
      screen.getByPlaceholderText(PRODUCT_VULNERABILITIES_SEARCH_PLACEHOLDER),
      { target: { value: "CVE-2024-1" } },
    );
    expect(lastRequest()).toEqual({
      filters: { searchQuery: "CVE-2024-1" },
      pagination: { offset: 0, limit: 10 },
    });
  });

  it("sends severity (as a number), product and version filters and resets the page", () => {
    render(<ProductVulnerabilitiesTable />);
    fireEvent.click(screen.getByRole("button", { name: /next page/i }));
    openFilters();

    choose("Severity", "Critical");
    expect(lastRequest().filters).toEqual({ severityId: 4 });
    expect(lastRequest().pagination.offset).toBe(0);

    choose("Product", "product-a");
    choose("Product Version", "4.7.0");
    expect(lastRequest().filters).toEqual({
      severityId: 4,
      productName: "product-a",
      productVersion: "4.7.0",
    });
  });

  it("offers the products from meta and the selected product's versions, clearing the version on product change", () => {
    render(<ProductVulnerabilitiesTable />);
    openFilters();

    // Version control is disabled until a product is chosen.
    expect(select("Product Version")).toHaveAttribute("aria-disabled", "true");

    fireEvent.mouseDown(select("Product"));
    expect(
      within(screen.getByRole("listbox")).getAllByRole("option").map((o) => o.textContent),
    ).toEqual(["All Products", "product-a", "product-b", "product-no-versions"]);
    fireEvent.click(within(screen.getByRole("listbox")).getByText("product-a"));

    fireEvent.mouseDown(select("Product Version"));
    expect(
      within(screen.getByRole("listbox")).getAllByRole("option").map((o) => o.textContent),
    ).toEqual(["All Versions", "4.10.0", "4.7.0"]);
    fireEvent.click(within(screen.getByRole("listbox")).getByText("4.7.0"));
    expect(lastRequest().filters.productVersion).toBe("4.7.0");

    choose("Product", "product-b");
    expect(lastRequest().filters).toEqual({ productName: "product-b" });

    fireEvent.mouseDown(select("Product Version"));
    expect(
      within(screen.getByRole("listbox")).getAllByRole("option").map((o) => o.textContent),
    ).toEqual(["All Versions", "1.0.0"]);
  });

  it("keeps the version control disabled when the product has no versions", () => {
    render(<ProductVulnerabilitiesTable />);
    openFilters();

    choose("Product", "product-no-versions");

    expect(select("Product Version")).toHaveAttribute("aria-disabled", "true");
    expect(screen.getByText("No versions available")).toBeInTheDocument();
    expect(lastRequest().filters).toEqual({ productName: "product-no-versions" });
  });

  it("clears the filters without clearing the search text", () => {
    render(<ProductVulnerabilitiesTable />);
    const search = screen.getByPlaceholderText(PRODUCT_VULNERABILITIES_SEARCH_PLACEHOLDER);
    fireEvent.change(search, { target: { value: "log4j" } });
    openFilters();
    choose("Severity", "High");
    expect(lastRequest().filters).toEqual({ searchQuery: "log4j", severityId: 3 });

    fireEvent.click(screen.getByRole("button", { name: /clear filters \(1\)/i }));

    expect(lastRequest().filters).toEqual({ searchQuery: "log4j" });
    expect(search).toHaveValue("log4j");
  });

  it("still renders and pages rows when the meta request fails", () => {
    mockMetaHook.mockReturnValue({ data: undefined, isError: true, isLoading: false });
    render(<ProductVulnerabilitiesTable />);

    expect(screen.getByText("CVE-2024-1001")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /next page/i }));
    expect(lastRequest().pagination).toEqual({ offset: 10, limit: 10 });

    openFilters();
    expect(select("Product")).toBeInTheDocument();
    expect(select("Product Version")).toHaveAttribute("aria-disabled", "true");
  });

  it("reports a failed search through onError", () => {
    const onError = vi.fn();
    mockSearchHook.mockReturnValue({ data: undefined, isLoading: false, isError: true });
    render(<ProductVulnerabilitiesTable onError={onError} />);

    expect(onError).toHaveBeenLastCalledWith(true);
  });
});
