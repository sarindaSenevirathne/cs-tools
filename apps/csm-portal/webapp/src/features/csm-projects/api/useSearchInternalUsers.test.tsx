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

import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi, beforeEach } from "vitest";

const useSearchUsersMock = vi.fn();
vi.mock("@features/csm-users/api/useSearchUsers", () => ({
  useSearchUsers: (...args: unknown[]) => useSearchUsersMock(...args),
}));

import { useSearchInternalUsers } from "@features/csm-projects/api/useSearchInternalUsers";

describe("useSearchInternalUsers", () => {
  beforeEach(() => useSearchUsersMock.mockReset());

  it("asks the backend for internal, active users only, and forwards the term and enabled", () => {
    useSearchUsersMock.mockReturnValue({ data: undefined, isFetching: false, isError: false });
    renderHook(() => useSearchInternalUsers("jane", true));

    const [request, enabled] = useSearchUsersMock.mock.calls[0];
    expect(request.filters).toEqual({
      searchQuery: "jane",
      roleIds: ["internal", "agent", "admin"],
      active: true,
    });
    expect(enabled).toBe(true);
  });

  it("drops external, inactive and id-less rows client-side as a defensive guard", () => {
    useSearchUsersMock.mockReturnValue({
      isFetching: false,
      isError: false,
      data: {
        users: [
          { id: "u-1", name: "Jane Doe", userName: "jane", email: "jane.doe@example.com", userType: "internal" },
          { id: "u-2", name: "Customer Contact", userName: "cc", email: "cc@example.com", userType: "external" },
          { id: "u-3", name: "Inactive Person", userName: "ip", email: "ip@example.com", active: false },
          { id: "", name: "No Id", userName: "ni", email: "ni@example.com" },
        ],
      },
    });
    const { result } = renderHook(() => useSearchInternalUsers("", true));
    expect(result.current.data).toEqual([
      { id: "u-1", name: "Jane Doe", email: "jane.doe@example.com" },
    ]);
  });
});
