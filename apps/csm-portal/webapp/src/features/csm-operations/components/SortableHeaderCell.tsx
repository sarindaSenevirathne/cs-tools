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

import { TableCell, TableSortLabel } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import { nextSort, type ListSort } from "@features/csm-operations/utils/listSort";

/** A table header cell that sorts the list by `field` when clicked. */
export default function SortableHeaderCell<F extends string>({
  label,
  field,
  sort,
  onSort,
}: {
  label: string;
  field: F;
  sort: ListSort<F>;
  onSort: (next: ListSort<F>) => void;
}): JSX.Element {
  const active = sort.field === field;
  return (
    <TableCell sortDirection={active ? sort.order : false}>
      <TableSortLabel
        active={active}
        direction={active ? sort.order : "desc"}
        onClick={() => onSort(nextSort(sort, field))}
      >
        {label}
      </TableSortLabel>
    </TableCell>
  );
}
