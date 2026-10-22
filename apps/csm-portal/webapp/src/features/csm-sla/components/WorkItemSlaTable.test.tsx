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
import { describe, expect, it, vi, beforeEach } from "vitest";
import "@testing-library/jest-dom/vitest";
import { WorkItemSlaTable } from "@features/csm-sla/components/WorkItemSlaTable";
import type { WorkItemSla, WorkItemSlaList } from "@features/csm-sla/types/workItemSla";
import { useGetWorkItemSlas } from "@features/csm-sla/api/useGetWorkItemSlas";

vi.mock("@features/csm-sla/api/useGetWorkItemSlas", () => ({
  useGetWorkItemSlas: vi.fn(),
}));

const mockedUseGetWorkItemSlas = vi.mocked(useGetWorkItemSlas);

const SLA_ROW: WorkItemSla = {
  id: "sla-1",
  definition: "S1 - Response",
  target: "1 Business Hour",
  stage: "in_progress",
  stageLabel: "In progress",
  hasBreached: false,
  businessTimeLeftLabel: "30 minutes",
  businessElapsedLabel: "30 minutes",
  businessElapsedPercent: 50,
  startTime: "2026-07-01T10:00:00Z",
  stopTime: null,
};

function mockResult(overrides: Partial<ReturnType<typeof useGetWorkItemSlas>>): void {
  mockedUseGetWorkItemSlas.mockReturnValue({
    data: null,
    isLoading: false,
    isError: false,
    isFetching: false,
    dataUpdatedAt: 0,
    refetch: vi.fn(),
    ...overrides,
  } as unknown as ReturnType<typeof useGetWorkItemSlas>);
}

describe("WorkItemSlaTable", () => {
  beforeEach(() => {
    mockedUseGetWorkItemSlas.mockReset();
  });

  it("renders a loading skeleton while the query is in flight", () => {
    mockResult({ isLoading: true });
    render(<WorkItemSlaTable workItemId="case-1" workItemLabel="case" />);
    expect(screen.getByText("SLAs")).toBeInTheDocument();
  });

  it("renders an empty state when there are no SLAs", () => {
    const empty: WorkItemSlaList = { count: 0, slas: [] };
    mockResult({ data: empty });
    render(<WorkItemSlaTable workItemId="case-1" workItemLabel="case" />);
    expect(screen.getByText("No SLAs on this case.")).toBeInTheDocument();
  });

  it("renders SLA rows with definition, stage, and time labels", () => {
    const list: WorkItemSlaList = { count: 1, slas: [SLA_ROW] };
    mockResult({ data: list });
    render(<WorkItemSlaTable workItemId="case-1" workItemLabel="case" />);
    expect(screen.getByText("S1 - Response")).toBeInTheDocument();
    expect(screen.getByText("In progress")).toBeInTheDocument();
    expect(screen.getAllByText("30 minutes").length).toBeGreaterThan(0);
    expect(screen.getByText("50%")).toBeInTheDocument();
    // Stop time is null → rendered as an em dash.
    expect(screen.getByText("—")).toBeInTheDocument();
  });

  it("renders an error state with a retry action that refetches", () => {
    const refetch = vi.fn();
    mockResult({ isError: true, refetch });
    render(<WorkItemSlaTable workItemId="case-1" workItemLabel="case" />);
    expect(screen.getByText("Could not load SLAs for this case.")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /retry/i }));
    expect(refetch).toHaveBeenCalled();
  });

  it("calls refetch when the header refresh button is clicked", () => {
    const refetch = vi.fn();
    const list: WorkItemSlaList = { count: 1, slas: [SLA_ROW] };
    mockResult({ data: list, refetch });
    render(<WorkItemSlaTable workItemId="case-1" workItemLabel="case" />);
    fireEvent.click(screen.getByRole("button", { name: /refresh slas/i }));
    expect(refetch).toHaveBeenCalled();
  });

  it("shows the 'Last refreshed' hint only after a manual refresh click, not on initial load", () => {
    const refetch = vi.fn();
    const list: WorkItemSlaList = { count: 1, slas: [SLA_ROW] };
    mockResult({ data: list, refetch, dataUpdatedAt: Date.now() });
    render(<WorkItemSlaTable workItemId="case-1" workItemLabel="case" />);

    // Initial load already set `dataUpdatedAt`, but the hint must stay hidden.
    expect(screen.queryByText(/last refreshed/i)).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /refresh slas/i }));

    expect(refetch).toHaveBeenCalled();
    expect(screen.getByText(/last refreshed/i)).toBeInTheDocument();
  });

  it("shows a truncation notice when fewer rows are rendered than the total count", () => {
    const list: WorkItemSlaList = { count: 51, slas: [SLA_ROW] };
    mockResult({ data: list });
    render(<WorkItemSlaTable workItemId="case-1" workItemLabel="case" />);
    expect(screen.getByText("Showing first 1 of 51")).toBeInTheDocument();
  });

  it("uses the work item label in the empty and error copy, so it reads right for an incident", () => {
    mockResult({ data: { count: 0, slas: [] } });
    const { unmount } = render(
      <WorkItemSlaTable workItemId="inc-1" workItemLabel="incident" />,
    );
    expect(screen.getByText("No SLAs on this incident.")).toBeInTheDocument();
    expect(mockedUseGetWorkItemSlas).toHaveBeenCalledWith("inc-1");
    unmount();

    mockResult({ isError: true });
    render(<WorkItemSlaTable workItemId="inc-1" workItemLabel="incident" />);
    expect(screen.getByText("Could not load SLAs for this incident.")).toBeInTheDocument();
  });

  it("renders a breached row for an incident in error styling with its elapsed percent", () => {
    mockResult({
      data: {
        count: 1,
        slas: [{ ...SLA_ROW, hasBreached: true, stage: "breached", stageLabel: "Breached", businessElapsedPercent: 120 }],
      },
    });
    render(<WorkItemSlaTable workItemId="inc-1" workItemLabel="incident" />);
    expect(screen.getByText("Breached")).toBeInTheDocument();
    expect(screen.getByText("120%")).toBeInTheDocument();
    expect(
      screen.getByRole("row", { name: "S1 - Response 100% elapsed" }),
    ).toBeInTheDocument();
  });
});
