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

import { describe, expect, it, vi } from "vitest";

vi.mock("@api/backend/client", () => ({
  BackendApiError: class BackendApiError extends Error {
    status: number;
    constructor(status: number, message: string) {
      super(message);
      this.status = status;
    }
  },
}));

import { BackendApiError } from "@api/backend/client";
import { bulkFailureMessage, bulkResultSummary, mapWithConcurrency, runBulkUpdate } from "@features/csm-operations/utils/bulkUpdate";

describe("bulkFailureMessage", () => {
  it("gives a refusal in the backend's own words, and anything else a generic line", () => {
    expect(bulkFailureMessage(new BackendApiError(400, "state cannot be set"))).toBe("state cannot be set");
    expect(bulkFailureMessage(new BackendApiError(409, "the change request moved on"))).toBe("the change request moved on");
    expect(bulkFailureMessage(new BackendApiError(500, "pq: connection reset"))).toBe("Could not update this record. Try again.");
    expect(bulkFailureMessage(new BackendApiError(400, "  "))).toBe("Could not update this record. Try again.");
    expect(bulkFailureMessage(new Error("network down"))).toBe("Could not update this record. Try again.");
  });
});

describe("runBulkUpdate", () => {
  it("updates every target, reports each refusal, and keeps the targets' order", async () => {
    const update = vi.fn((t: { id: string }) =>
      t.id === "b" ? Promise.reject(new BackendApiError(400, "on hold")) : Promise.resolve(),
    );
    const result = await runBulkUpdate(
      [{ id: "a", number: "N-1" }, { id: "b", number: "N-2" }, { id: "c" }],
      update,
    );
    expect(update).toHaveBeenCalledTimes(3);
    expect(result.succeeded).toEqual(["a", "c"]);
    expect(result.failed).toEqual([{ id: "b", number: "N-2", message: "on hold" }]);
  });

  it("never has more than `concurrency` requests in flight", async () => {
    let inFlight = 0;
    let peak = 0;
    const update = async (): Promise<void> => {
      inFlight++;
      peak = Math.max(peak, inFlight);
      await new Promise((r) => setTimeout(r, 1));
      inFlight--;
    };
    const targets = Array.from({ length: 10 }, (_, i) => ({ id: String(i) }));
    const result = await runBulkUpdate(targets, update, 3);
    expect(peak).toBe(3);
    expect(result.succeeded).toHaveLength(10);
  });

  it("does nothing for no targets", async () => {
    const update = vi.fn();
    expect(await runBulkUpdate([], update)).toEqual({ succeeded: [], failed: [] });
    expect(update).not.toHaveBeenCalled();
  });

  it("names a record with no number by its id", async () => {
    const result = await runBulkUpdate([{ id: "x-1", number: null }], () => Promise.reject(new Error("boom")));
    expect(result.failed[0].number).toBe("x-1");
  });
});

describe("bulkResultSummary", () => {
  it("says how many were updated, refused and skipped", () => {
    expect(bulkResultSummary({ succeeded: ["a", "b"], failed: [], skipped: [] }, "incidents")).toBe(
      "Updated 2 of 2 incidents.",
    );
    expect(
      bulkResultSummary(
        { succeeded: ["a"], failed: [{ id: "b", number: "B", message: "no" }], skipped: [{ id: "c", number: "C", message: "x" }] },
        "change requests",
        "Assigned to you",
      ),
    ).toBe("Assigned to you 1 of 3 change requests. 1 could not be updated. 1 skipped.");
  });
});

describe("mapWithConcurrency", () => {
  it("keeps the items' order, leaves a rejection as undefined, and bounds the requests in flight", async () => {
    let inFlight = 0;
    let peak = 0;
    const results = await mapWithConcurrency(
      [1, 2, 3, 4, 5, 6],
      async (n) => {
        inFlight++;
        peak = Math.max(peak, inFlight);
        await new Promise((r) => setTimeout(r, 6 - n));
        inFlight--;
        if (n === 3) throw new Error("no");
        return n * 10;
      },
      2,
    );
    expect(results).toEqual([10, 20, undefined, 40, 50, 60]);
    expect(peak).toBe(2);
  });
});
