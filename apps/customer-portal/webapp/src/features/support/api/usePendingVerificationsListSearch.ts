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

import { useQuery, type UseQueryResult } from "@tanstack/react-query";
import { useAsgardeo } from "@asgardeo/react";
import { useAuthApiClient } from "@/hooks/useAuthApiClient";
import { ApiQueryKeys } from "@constants/apiConstants";
import type {
  PendingVerificationRecordType,
  PendingVerificationSearchRequest,
  PendingVerificationSearchResponse,
} from "@features/support/types/pendingVerification";

export interface PendingVerificationsListFilters {
  recordTypes?: PendingVerificationRecordType[];
  searchQuery?: string;
  includeVerified?: boolean;
}

export interface PendingVerificationsListPagination {
  limit: number;
  offset: number;
}

/**
 * Project-scoped, paginated search over pending-verification records for the
 * sidebar list page. Kept separate from usePendingVerificationsSearch (the
 * per-case lookup used by Case Detail's badge/tab) — different shape, and a
 * distinct query key so neither can invalidate the other's cache.
 *
 * @param {string} projectId - Project to search within.
 * @param {PendingVerificationsListFilters} filters - Record-type/search/verified filters.
 * @param {PendingVerificationsListPagination} pagination - limit/offset.
 * @param {boolean} [enabled] - Whether the query should run.
 * @returns Query result with pendingVerifications, counts, and pagination.
 */
export function usePendingVerificationsListSearch(
  projectId: string,
  filters: PendingVerificationsListFilters,
  pagination: PendingVerificationsListPagination,
  enabled = true,
): UseQueryResult<PendingVerificationSearchResponse, Error> {
  const { isSignedIn, isLoading: isAuthLoading } = useAsgardeo();
  const authFetch = useAuthApiClient();

  return useQuery<PendingVerificationSearchResponse, Error>({
    queryKey: [
      ApiQueryKeys.PENDING_VERIFICATIONS_SEARCH,
      projectId,
      "list",
      filters,
      pagination,
    ],
    queryFn: async (): Promise<PendingVerificationSearchResponse> => {
      const baseUrl = window.config?.CUSTOMER_PORTAL_BACKEND_BASE_URL;
      if (!baseUrl) {
        throw new Error("CUSTOMER_PORTAL_BACKEND_BASE_URL is not configured");
      }

      const body: PendingVerificationSearchRequest = {
        projectId,
        pagination,
        filters: {
          recordTypes: filters.recordTypes,
          searchQuery: filters.searchQuery,
          includeVerified: filters.includeVerified ?? false,
        },
      };

      const response = await authFetch(`${baseUrl}/pending-verifications/search`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });

      if (!response.ok) {
        throw new Error(
          `Failed to fetch pending verifications: ${response.statusText}`,
        );
      }

      return response.json() as Promise<PendingVerificationSearchResponse>;
    },
    enabled: !!projectId && isSignedIn && !isAuthLoading && enabled,
    staleTime: 0,
  });
}

export default usePendingVerificationsListSearch;
