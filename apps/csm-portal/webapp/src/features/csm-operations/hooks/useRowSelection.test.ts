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

import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { useRowSelection } from "@features/csm-operations/hooks/useRowSelection";

interface Row {
  id?: string | null;
  state: string;
}
const getId = (r: Row): string | null | undefined => r.id;
const canSelect = (r: Row): boolean => r.state !== "closed";

const a: Row = { id: "a", state: "open" };
const b: Row = { id: "b", state: "open" };
const closed: Row = { id: "c", state: "closed" };
const noId: Row = { id: null, state: "open" };

describe("useRowSelection", () => {
  it("toggles a row, never one it may not select or one with no id", () => {
    const { result } = renderHook(() => useRowSelection(getId, canSelect));
    act(() => result.current.toggle(a));
    act(() => result.current.toggle(closed));
    act(() => result.current.toggle(noId));
    expect(result.current.count).toBe(1);
    expect(result.current.isSelected(a)).toBe(true);
    expect(result.current.isSelected(closed)).toBe(false);
    act(() => result.current.toggle(a));
    expect(result.current.count).toBe(0);
  });

  it("selects a page's selectable rows, then clears them on a second toggle", () => {
    const { result } = renderHook(() => useRowSelection(getId, canSelect));
    const page = [a, b, closed];
    expect(result.current.pageState(page)).toEqual({ all: false, some: false });
    act(() => result.current.toggle(a));
    expect(result.current.pageState(page)).toEqual({ all: false, some: true });
    act(() => result.current.togglePage(page));
    expect(result.current.pageState(page)).toEqual({ all: true, some: false });
    expect(result.current.selected.map((r) => r.id)).toEqual(["a", "b"]);
    act(() => result.current.togglePage(page));
    expect(result.current.count).toBe(0);
  });

  it("keeps a selection across pages, and clears it on request", () => {
    const { result } = renderHook(() => useRowSelection(getId, canSelect));
    act(() => result.current.togglePage([a]));
    act(() => result.current.togglePage([b]));
    expect(result.current.count).toBe(2);
    // The first page is still all selected when the user comes back to it.
    expect(result.current.pageState([a]).all).toBe(true);
    act(() => result.current.clear());
    expect(result.current.count).toBe(0);
  });

  it("does not report a page with nothing selectable as all selected", () => {
    const { result } = renderHook(() => useRowSelection(getId, canSelect));
    expect(result.current.pageState([closed]).all).toBe(false);
    act(() => result.current.togglePage([closed]));
    expect(result.current.count).toBe(0);
  });
});

describe("useRowSelection — rows that change after they were picked", () => {
  it("reports a picked row as the current page shows it", () => {
    let page: Row[] = [a, b];
    const { result, rerender } = renderHook(() => useRowSelection(getId, canSelect, page));
    act(() => result.current.togglePage(page));
    page = [{ id: "a", state: "pending" }, b];
    rerender();
    expect(result.current.selected).toEqual([{ id: "a", state: "pending" }, b]);
    // A row not on the current page keeps the version it was picked with.
    page = [];
    rerender();
    expect(result.current.selected).toEqual([a, b]);
  });

  it("lets a row that has stopped being selectable be taken out, by itself or with its page", () => {
    let page: Row[] = [a, b];
    const { result, rerender } = renderHook(() => useRowSelection(getId, canSelect, page));
    act(() => result.current.togglePage(page));
    const aNowClosed = { id: "a", state: "closed" };
    page = [aNowClosed, b];
    rerender();
    // It can't be added again, but it can be removed.
    act(() => result.current.toggle(aNowClosed));
    expect(result.current.isSelected(aNowClosed)).toBe(false);
    act(() => result.current.toggle(aNowClosed));
    expect(result.current.isSelected(aNowClosed)).toBe(false);

    act(() => result.current.toggle(a));
    expect(result.current.count).toBe(2);
    page = [aNowClosed, b];
    rerender();
    // The page's only selectable row is selected, so the page box clears the page, the closed one included.
    expect(result.current.pageState(page).all).toBe(true);
    act(() => result.current.togglePage(page));
    expect(result.current.count).toBe(0);
  });

  it("removes only the ids it is given", () => {
    const { result } = renderHook(() => useRowSelection(getId, canSelect));
    act(() => result.current.togglePage([a, b]));
    act(() => result.current.remove(["a", "missing"]));
    expect(result.current.selected).toEqual([b]);
    const before = result.current.selected;
    act(() => result.current.remove(["missing"]));
    expect(result.current.selected).toBe(before);
  });
});
