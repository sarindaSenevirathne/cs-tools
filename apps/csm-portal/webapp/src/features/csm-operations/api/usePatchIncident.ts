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
  type QueryClient,
  type UseMutationResult,
} from "@tanstack/react-query";
import { ApiQueryKeys } from "@constants/apiConstants";
import { useBackendApi } from "@api/backend/client";
import type {
  BePatchIncidentResponse,
  BeUpdateIncidentPayload,
} from "@api/backend/types";

/**
 * entity-service's incident flows (the report task on In Progress; the report,
 * alert tasks and problem on Resolved) run in a background drain every 5s,
 * after this PATCH has returned. The detail and its tasks are fetched again
 * once that has had time to happen, so the new task or problem shows on the
 * Related tab without a reload.
 */
const INCIDENT_FLOW_SETTLE_MS = 7_000;
const STATES_WITH_FLOWS = new Set(["IN_PROGRESS", "RESOLVED"]);

export interface PatchIncidentInput {
  id: string;
  patch: BeUpdateIncidentPayload;
}

/**
 * Update an incident via `PATCH /incidents/{id}` (ServiceNow data source
 * only). The BE requires at least one field in `patch`. On success the
 * detail and any cached list/search are invalidated so the new values show.
 */
export function usePatchIncident(): UseMutationResult<
  BePatchIncidentResponse,
  Error,
  PatchIncidentInput
> {
  const api = useBackendApi();
  const queryClient = useQueryClient();

  return useMutation<BePatchIncidentResponse, Error, PatchIncidentInput>({
    mutationFn: (input): Promise<BePatchIncidentResponse> =>
      api.patch<BeUpdateIncidentPayload, BePatchIncidentResponse>(
        `/incidents/${encodeURIComponent(input.id)}`,
        input.patch,
      ),
    onSuccess: (_data, variables) => {
      void queryClient.invalidateQueries({
        queryKey: [ApiQueryKeys.INCIDENTS],
      });
      refreshIncidentRecord(queryClient, variables.id, variables.patch);
    },
  });
}

/**
 * What an incident's PATCH makes stale, beside the list: its detail, its
 * tasks (a move to Closed or Cancelled closes them), its activity feed and
 * its notes (the update's own, and the flows'), and once more after the
 * background flows of In Progress / Resolved have had time to run. Shared by
 * the single-record edit and the list's bulk edit, so both refresh the same.
 */
export function refreshIncidentRecord(queryClient: QueryClient, id: string, patch: BeUpdateIncidentPayload): void {
  void queryClient.invalidateQueries({ queryKey: [ApiQueryKeys.INCIDENT_DETAILS, id] });
  void queryClient.invalidateQueries({ queryKey: [ApiQueryKeys.CSM_INCIDENT_TASKS, id] });
  // The update writes a field-change entry to the activity feed.
  void queryClient.invalidateQueries({ queryKey: [ApiQueryKeys.INCIDENT_ACTIVITIES, id] });
  // Notes sent with the update (work notes, close notes) land in the comments lane.
  void queryClient.invalidateQueries({ queryKey: [ApiQueryKeys.INCIDENT_COMMENTS, id] });
  if (patch.state && STATES_WITH_FLOWS.has(patch.state)) {
    setTimeout(() => {
      void queryClient.invalidateQueries({ queryKey: [ApiQueryKeys.INCIDENT_DETAILS, id] });
      void queryClient.invalidateQueries({ queryKey: [ApiQueryKeys.CSM_INCIDENT_TASKS, id] });
      // The flows add work notes of their own.
      void queryClient.invalidateQueries({ queryKey: [ApiQueryKeys.INCIDENT_COMMENTS, id] });
    }, INCIDENT_FLOW_SETTLE_MS);
  }
}
