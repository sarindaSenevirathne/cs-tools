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

import { useCallback, useMemo, useState } from "react";

export interface RowSelection<T> {
  /**
   * The selected rows, in the order they were picked. A row on the page in
   * `currentRows` is its current version there; a row on another page is as it
   * was when picked.
   */
  selected: T[];
  count: number;
  isSelected: (row: T) => boolean;
  /** Add the row, or remove it when it is selected (always possible, even once it is no longer selectable). */
  toggle: (row: T) => void;
  /**
   * Select every selectable row of `rows` (a page), or, when every one is
   * already selected, remove the page's rows from the selection (a row that
   * has stopped being selectable since it was picked included).
   */
  togglePage: (rows: T[]) => void;
  /** Whether every selectable row of `rows` is selected / some are. */
  pageState: (rows: T[]) => { all: boolean; some: boolean };
  /** Remove these ids (e.g. the rows a bulk run has handled), keeping the rest. */
  remove: (ids: string[]) => void;
  clear: () => void;
}

/**
 * Row selection for a paged list: kept by id across pages, with rows
 * `canSelect` refuses never added (e.g. a closed incident). A picked row is
 * remembered as it was, and replaced by its current version whenever it is on
 * the page passed as `currentRows`, so a bulk action plans from what the list
 * shows now. The same model as the cases list's selection.
 */
export function useRowSelection<T>(
  getId: (row: T) => string | null | undefined,
  canSelect: (row: T) => boolean,
  currentRows: T[] = [],
): RowSelection<T> {
  const [byId, setById] = useState<Map<string, T>>(() => new Map());

  const selectable = useCallback((rows: T[]) => rows.filter((r) => !!getId(r) && canSelect(r)), [getId, canSelect]);

  const toggle = useCallback(
    (row: T) => {
      const id = getId(row);
      if (!id) return;
      setById((prev) => {
        if (!prev.has(id) && !canSelect(row)) return prev;
        const next = new Map(prev);
        if (next.has(id)) next.delete(id);
        else next.set(id, row);
        return next;
      });
    },
    [getId, canSelect],
  );

  const pageState = useCallback(
    (rows: T[]) => {
      const candidates = selectable(rows);
      const picked = candidates.filter((r) => byId.has(getId(r)!)).length;
      return { all: candidates.length > 0 && picked === candidates.length, some: picked > 0 && picked < candidates.length };
    },
    [byId, getId, selectable],
  );

  const togglePage = useCallback(
    (rows: T[]) => {
      const candidates = selectable(rows);
      setById((prev) => {
        const next = new Map(prev);
        const all = candidates.length > 0 && candidates.every((r) => next.has(getId(r)!));
        if (all) {
          for (const r of rows) {
            const id = getId(r);
            if (id) next.delete(id);
          }
        } else {
          for (const r of candidates) next.set(getId(r)!, r);
        }
        return next;
      });
    },
    [getId, selectable],
  );

  const remove = useCallback((ids: string[]) => {
    setById((prev) => {
      if (!ids.some((id) => prev.has(id))) return prev;
      const next = new Map(prev);
      for (const id of ids) next.delete(id);
      return next;
    });
  }, []);

  const clear = useCallback(() => setById(new Map()), []);
  const selected = useMemo(() => {
    const current = new Map<string, T>();
    for (const r of currentRows) {
      const id = getId(r);
      if (id) current.set(id, r);
    }
    return Array.from(byId, ([id, row]) => current.get(id) ?? row);
  }, [byId, currentRows, getId]);

  return {
    selected,
    count: byId.size,
    isSelected: (row: T) => {
      const id = getId(row);
      return !!id && byId.has(id);
    },
    toggle,
    togglePage,
    pageState,
    remove,
    clear,
  };
}
