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

import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { describe, expect, it, vi, beforeEach } from "vitest";
import type { ReactNode } from "react";

// useGetAccount.ts reads window.config at module load (via @config/apiConfig)
// and calls the real Asgardeo-backed useAuthApiClient; neither is present
// under vitest, so mock both. authFetchMock stands in for the fetch wrapper
// and is what the hook actually calls, so the response body below goes through
// the real hook code, unlike the page tests, which mock the hook itself.
const authFetchMock = vi.fn();
vi.mock("@config/apiConfig", () => ({
  apiConfig: { backendUrl: "https://example.test" },
}));
vi.mock("@hooks/useAuthApiClient", () => ({
  useAuthApiClient: () => authFetchMock,
}));

import { useGetAccount } from "@features/csm-accounts/api/useGetAccount";

function jsonResponse(body: unknown): Response {
  return {
    ok: true,
    status: 200,
    statusText: "OK",
    json: async () => body,
    text: async () => JSON.stringify(body),
  } as unknown as Response;
}

function wrapper({ children }: { children: ReactNode }) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
}

describe("useGetAccount", () => {
  beforeEach(() => {
    authFetchMock.mockReset();
  });

  it("passes the account's country through from the response body", async () => {
    authFetchMock.mockResolvedValueOnce(
      jsonResponse({ id: "acct-1", name: "Acme Corp", region: "APAC", country: "Sri Lanka" }),
    );

    const { result } = renderHook(() => useGetAccount("acct-1"), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(authFetchMock).toHaveBeenCalledWith("https://example.test/accounts/acct-1");
    expect(result.current.data?.country).toBe("Sri Lanka");
  });

  it("keeps a null country as null rather than dropping or defaulting it", async () => {
    authFetchMock.mockResolvedValueOnce(
      jsonResponse({ id: "acct-1", name: "Acme Corp", country: null }),
    );

    const { result } = renderHook(() => useGetAccount("acct-1"), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.country).toBeNull();
  });
});
