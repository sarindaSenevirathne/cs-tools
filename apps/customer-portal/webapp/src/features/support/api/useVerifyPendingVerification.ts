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

import {
  useMutation,
  useQueryClient,
  type UseMutationResult,
} from "@tanstack/react-query";
import { useAsgardeo } from "@asgardeo/react";
import { useAuthApiClient } from "@/hooks/useAuthApiClient";
import { useLogger } from "@hooks/useLogger";
import { ApiQueryKeys } from "@constants/apiConstants";
import { parseApiResponseMessage } from "@utils/ApiError";
import type { VerifyPendingVerificationResponse } from "@features/support/types/pendingVerification";

/**
 * Hook to mark a pending-verification record as verified
 * (PATCH /pending-verifications/:id). Bodyless — verifiedBy is resolved
 * server-side from the caller's token.
 *
 * @param {string} projectId - Project the case belongs to, for cache invalidation.
 * @param {string} workItemId - The case (work item) id, for cache invalidation.
 * @returns Mutation result; call with the pending-verification record's own id.
 */
export function useVerifyPendingVerification(
  projectId: string,
  workItemId: string,
): UseMutationResult<VerifyPendingVerificationResponse, Error, string> {
  // Kept in the signature (matching useCreatePendingVerification's shape) for
  // call-site scope clarity, even though only projectId is needed below —
  // invalidation is now a project-wide prefix match, see onSuccess.
  void workItemId;
  const logger = useLogger();
  const queryClient = useQueryClient();
  const { isSignedIn, isLoading: isAuthLoading } = useAsgardeo();
  const authFetch = useAuthApiClient();

  return useMutation<VerifyPendingVerificationResponse, Error, string>({
    mutationFn: async (id: string): Promise<VerifyPendingVerificationResponse> => {
      logger.debug("[useVerifyPendingVerification] Request:", { id });

      if (!id) {
        throw new Error("Pending verification id is required");
      }
      if (!isSignedIn || isAuthLoading) {
        throw new Error("User must be signed in to mark a case as verified");
      }

      const baseUrl = window.config?.CUSTOMER_PORTAL_BACKEND_BASE_URL;
      if (!baseUrl) {
        throw new Error("CUSTOMER_PORTAL_BACKEND_BASE_URL is not configured");
      }

      const response = await authFetch(`${baseUrl}/pending-verifications/${id}`, {
        method: "PATCH",
      });

      if (!response.ok) {
        const text = await response.text();
        throw new Error(parseApiResponseMessage(text, response.status, response.statusText));
      }

      const data: VerifyPendingVerificationResponse = await response.json();
      logger.debug("[useVerifyPendingVerification] Verified:", data);
      return data;
    },
    onSuccess: () => {
      // Prefix match (not the exact [..., workItemId] key) so this also
      // refreshes the sidebar list page's own broader query for this
      // project, not just this one case's Case Detail badge/tab.
      void queryClient.refetchQueries({
        queryKey: [ApiQueryKeys.PENDING_VERIFICATIONS_SEARCH, projectId],
        type: "active",
      });
      queryClient.invalidateQueries({
        queryKey: [ApiQueryKeys.PENDING_VERIFICATIONS_SEARCH, projectId],
      });
    },
  });
}

export default useVerifyPendingVerification;
