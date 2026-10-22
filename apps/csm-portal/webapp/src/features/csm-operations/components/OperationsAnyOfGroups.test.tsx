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

import { useState, type JSX } from "react";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import "@testing-library/jest-dom/vitest";
import OperationsAnyOfGroups, { type AnyOfFieldDef } from "@features/csm-operations/components/OperationsAnyOfGroups";
import { completeAnyOfGroups, type AnyOfGroup } from "@features/csm-operations/utils/operationsAnyOf";

// A value picker per field: one button per value, toggling it.
const picker =
  (values: string[]): AnyOfFieldDef["render"] =>
  ({ id, values: selected, onChange }) => (
    <div data-testid={id}>
      {values.map((v) => (
        <button
          key={v}
          type="button"
          aria-pressed={selected.includes(v)}
          onClick={() => onChange(selected.includes(v) ? selected.filter((x) => x !== v) : [...selected, v])}
        >
          {v}
        </button>
      ))}
    </div>
  );

const FIELDS: AnyOfFieldDef[] = [
  { field: "state", label: "State", render: picker(["new", "closed"]) },
  { field: "impact", label: "Impact", render: picker(["high", "low"]) },
];

const SAVED_VIEW: AnyOfGroup[] = [
  { conditions: [{ field: "impact", values: ["low"] }] },
  { conditions: [{ field: "state", values: ["new"] }] },
];

/**
 * The lists keep their filters in the URL, which holds complete conditions
 * only: what the component gets back is never what it sent while a condition
 * has no value. This harness does exactly that round trip.
 */
function ThroughTheUrl({ onSent }: { onSent: (groups: AnyOfGroup[]) => void }): JSX.Element {
  const [groups, setGroups] = useState<AnyOfGroup[]>([]);
  return (
    <>
      {/* What changes the filters from outside: Clear filters, a saved view. */}
      <button type="button" onClick={() => setGroups([])}>
        Clear filters
      </button>
      <button type="button" onClick={() => setGroups(SAVED_VIEW)}>
        Open saved view
      </button>
      <OperationsAnyOfGroups
        idPrefix="t"
        fields={FIELDS}
        groups={groups}
        onChange={(next) => {
          onSent(next);
          setGroups(completeAnyOfGroups(next));
        }}
      />
    </>
  );
}

const group = (n: number): HTMLElement => screen.getByRole("group", { name: `OR group ${n}` });

describe("OperationsAnyOfGroups — editing survives the URL round trip", () => {
  it("keeps a group just added, a condition just added and a field just changed, all with no value yet", () => {
    const onSent = vi.fn();
    render(<ThroughTheUrl onSent={onSent} />);

    fireEvent.click(screen.getByRole("button", { name: "Add group" }));
    expect(group(1)).toBeInTheDocument();

    // A field change resets the values: the condition stays.
    fireEvent.mouseDown(within(group(1)).getByRole("combobox", { name: "Field" }));
    fireEvent.click(screen.getByRole("option", { name: "Impact" }));
    expect(within(group(1)).getByRole("combobox", { name: "Field" })).toHaveTextContent("Impact");

    fireEvent.click(within(group(1)).getByRole("button", { name: "high" }));
    expect(onSent).toHaveBeenLastCalledWith([{ conditions: [{ field: "impact", values: ["high"] }] }]);

    // A condition added to a complete group stays too.
    fireEvent.click(within(group(1)).getByRole("button", { name: "Add condition" }));
    expect(within(group(1)).getAllByRole("combobox", { name: "Field" })).toHaveLength(2);

    // A second group, then OR between them.
    fireEvent.click(screen.getByRole("button", { name: "Add group" }));
    expect(group(2)).toBeInTheDocument();
    expect(screen.getByText("OR")).toBeInTheDocument();
  });

  it("keeps a condition whose last value was removed", () => {
    render(<ThroughTheUrl onSent={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: "Add group" }));
    fireEvent.click(within(group(1)).getByRole("button", { name: "new" }));
    fireEvent.click(within(group(1)).getByRole("button", { name: "new" }));
    expect(within(group(1)).getByRole("combobox", { name: "Field" })).toBeInTheDocument();
  });

  it("follows the filters when they change from outside (Clear filters, a saved view, Back)", () => {
    render(<ThroughTheUrl onSent={vi.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: "Add group" }));
    fireEvent.click(within(group(1)).getByRole("button", { name: "closed" }));
    expect(group(1)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Clear filters" }));
    expect(screen.queryByRole("group", { name: "OR group 1" })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Open saved view" }));
    expect(within(group(1)).getByRole("combobox", { name: "Field" })).toHaveTextContent("Impact");
    expect(within(group(2)).getByRole("combobox", { name: "Field" })).toHaveTextContent("State");
  });
});
