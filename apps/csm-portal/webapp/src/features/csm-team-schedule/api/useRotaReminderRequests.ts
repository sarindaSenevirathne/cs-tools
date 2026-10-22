/**
 * Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from "@tanstack/react-query";
import { useBackendApi } from "@api/backend/client";
import type {
  RotaFamily,
  RotaReminderRequest,
  ScheduleAssignmentsResponse,
  SearchScheduleAssignmentsPayload,
  SendRotaReminder,
} from "../types";

const requestsKey = (family: RotaFamily) => ["team-schedule", "reminder-requests", family] as const;
const requestsPath = (family: RotaFamily) => `/team-schedule/reminder-requests/${encodeURIComponent(family)}`;

/** How often the list reads again while a request is still waiting or being sent. */
const WHILE_WAITING_MS = 60_000;

/**
 * A group's latest asks to send the rota reminders now, newest first (the
 * server keeps the last five). While one is QUEUED or SENDING the list reads
 * again every minute, to show it was sent; once none is, it stops.
 */
export function useRotaReminderRequests(family: RotaFamily): UseQueryResult<RotaReminderRequest[], Error> {
  const api = useBackendApi();
  return useQuery<RotaReminderRequest[], Error>({
    queryKey: requestsKey(family),
    queryFn: async () =>
      (await api.get<{ requests: RotaReminderRequest[] }>(requestsPath(family)))?.requests ?? [],
    refetchInterval: (query) =>
      query.state.data?.some((r) => r.status === "QUEUED" || r.status === "SENDING") ? WHILE_WAITING_MS : false,
  });
}

/**
 * Asks for a group's weekday or weekend reminders to go out now. With
 * `test: true` the reminder goes only to the person asking, never to the rota.
 * The server refuses (400, 403, 409 one is already waiting, 429 asked too
 * recently) in words, which surface as the error's message. Afterwards the list
 * is read again, whether the ask worked or not.
 */
export function useSendRotaReminder(
  family: RotaFamily,
): UseMutationResult<RotaReminderRequest, Error, SendRotaReminder> {
  const api = useBackendApi();
  const qc = useQueryClient();
  return useMutation<RotaReminderRequest, Error, SendRotaReminder>({
    mutationFn: (body) => api.post<SendRotaReminder, RotaReminderRequest>(requestsPath(family), body),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: requestsKey(family) });
    },
  });
}

/**
 * Who is on the CRE rota between two days (YYYY-MM-DD), leaving out people on
 * leave: the people a reminder for those days would reach. A query of its own,
 * so the rota page's queries are not touched by it.
 */
export function useReminderPreview(
  from: string,
  to: string,
  enabled: boolean,
): UseQueryResult<ScheduleAssignmentsResponse, Error> {
  const api = useBackendApi();
  return useQuery<ScheduleAssignmentsResponse, Error>({
    queryKey: ["team-schedule", "reminder-preview", from, to] as const,
    queryFn: () =>
      api.post<SearchScheduleAssignmentsPayload, ScheduleAssignmentsResponse>("/team-schedule/assignments/search", {
        from,
        to,
        family: "CRE",
        excludeAbsent: true,
      }),
    enabled,
  });
}
