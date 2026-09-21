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
import type {
  CreatePendingVerificationRequest,
  CreatePendingVerificationResponse,
} from "@features/support/types/pendingVerification";

/**
 * Hook to add a work item (case) to the pending-verification list
 * (POST /pending-verifications).
 *
 * @param {string} projectId - Project the case belongs to, for cache invalidation.
 * @param {string} workItemId - The case (work item) id being added.
 * @returns Mutation result.
 */
export function useCreatePendingVerification(
  projectId: string,
  workItemId: string,
): UseMutationResult<
  CreatePendingVerificationResponse,
  Error,
  CreatePendingVerificationRequest
> {
  const logger = useLogger();
  const queryClient = useQueryClient();
  const { isSignedIn, isLoading: isAuthLoading } = useAsgardeo();
  const authFetch = useAuthApiClient();

  return useMutation<
    CreatePendingVerificationResponse,
    Error,
    CreatePendingVerificationRequest
  >({
    mutationFn: async (
      body: CreatePendingVerificationRequest,
    ): Promise<CreatePendingVerificationResponse> => {
      logger.debug("[useCreatePendingVerification] Request:", { workItemId, body });

      if (!isSignedIn || isAuthLoading) {
        throw new Error("User must be signed in to add a case to verification");
      }

      const baseUrl = window.config?.CUSTOMER_PORTAL_BACKEND_BASE_URL;
      if (!baseUrl) {
        throw new Error("CUSTOMER_PORTAL_BACKEND_BASE_URL is not configured");
      }

      const response = await authFetch(`${baseUrl}/pending-verifications`, {
        method: "POST",
        body: JSON.stringify(body),
      });

      if (!response.ok) {
        const text = await response.text();
        throw new Error(parseApiResponseMessage(text, response.status, response.statusText));
      }

      const data: CreatePendingVerificationResponse = await response.json();
      logger.debug("[useCreatePendingVerification] Created:", data);
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

export default useCreatePendingVerification;
