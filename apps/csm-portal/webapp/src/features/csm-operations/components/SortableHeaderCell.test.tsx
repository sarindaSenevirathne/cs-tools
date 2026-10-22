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

import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import "@testing-library/jest-dom/vitest";
import { Table, TableHead, TableRow } from "@wso2/oxygen-ui";
import SortableHeaderCell from "@features/csm-operations/components/SortableHeaderCell";
import { nextSort } from "@features/csm-operations/utils/listSort";

describe("nextSort", () => {
  it("flips the active column and starts another newest first", () => {
    expect(nextSort({ field: "updatedOn", order: "desc" }, "updatedOn")).toEqual({ field: "updatedOn", order: "asc" });
    expect(nextSort({ field: "updatedOn", order: "asc" }, "updatedOn")).toEqual({ field: "updatedOn", order: "desc" });
    expect(nextSort({ field: "updatedOn", order: "asc" }, "createdOn")).toEqual({ field: "createdOn", order: "desc" });
  });
});

describe("SortableHeaderCell", () => {
  it("marks the active column and sorts by its field when clicked", () => {
    const onSort = vi.fn();
    render(
      <Table>
        <TableHead>
          <TableRow>
            <SortableHeaderCell label="Updated" field="updatedOn" sort={{ field: "createdOn", order: "desc" }} onSort={onSort} />
            <SortableHeaderCell label="Created" field="createdOn" sort={{ field: "createdOn", order: "desc" }} onSort={onSort} />
          </TableRow>
        </TableHead>
      </Table>,
    );
    expect(screen.getByRole("columnheader", { name: /created/i })).toHaveAttribute("aria-sort", "descending");
    expect(screen.getByRole("columnheader", { name: /updated/i })).not.toHaveAttribute("aria-sort");
    fireEvent.click(screen.getByText("Updated"));
    expect(onSort).toHaveBeenCalledWith({ field: "updatedOn", order: "desc" });
  });
});
