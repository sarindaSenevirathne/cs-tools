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
  PendingVerificationSearchRequest,
  PendingVerificationSearchResponse,
} from "@features/support/types/pendingVerification";

/**
 * Searches pending-verification records for a work item (case). Used both to
 * check whether a case already has a record (badge/action-row state) and to
 * render the Verifications tab's full history — same query key, so mounting
 * both at once dedupes to a single network call.
 *
 * @param {string} projectId - Project the case belongs to.
 * @param {string} workItemId - The case (work item) id to look up.
 * @param {boolean} [enabled] - Whether the query should run.
 * @returns Query result with pendingVerifications, counts, and pagination.
 */
export function usePendingVerificationsSearch(
  projectId: string,
  workItemId: string,
  enabled = true,
): UseQueryResult<PendingVerificationSearchResponse, Error> {
  const { isSignedIn, isLoading: isAuthLoading } = useAsgardeo();
  const authFetch = useAuthApiClient();

  return useQuery<PendingVerificationSearchResponse, Error>({
    queryKey: [ApiQueryKeys.PENDING_VERIFICATIONS_SEARCH, projectId, workItemId],
    queryFn: async (): Promise<PendingVerificationSearchResponse> => {
      const baseUrl = window.config?.CUSTOMER_PORTAL_BACKEND_BASE_URL;
      if (!baseUrl) {
        throw new Error("CUSTOMER_PORTAL_BACKEND_BASE_URL is not configured");
      }

      const body: PendingVerificationSearchRequest = {
        projectId,
        pagination: { limit: 20, offset: 0 },
        filters: { workItemId, includeVerified: true },
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
    enabled: !!projectId && !!workItemId && isSignedIn && !isAuthLoading && enabled,
    staleTime: 0,
  });
}

export default usePendingVerificationsSearch;
