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
import type { RotaFamily, RotaReminderSettings, SaveRotaReminderSettings } from "../types";

const settingsKey = (family: RotaFamily) => ["team-schedule", "reminder-settings", family] as const;
const settingsPath = (family: RotaFamily) => `/team-schedule/reminder-settings/${encodeURIComponent(family)}`;

/**
 * A group's saved rota reminder settings. `null` (not an error) when nothing
 * was saved yet: the backend answers 404 and the client's get() resolves a 404
 * to null, so the reminders run on their built-in settings. Any other failure
 * is an error.
 */
export function useRotaReminderSettings(family: RotaFamily): UseQueryResult<RotaReminderSettings | null, Error> {
  const api = useBackendApi();
  return useQuery<RotaReminderSettings | null, Error>({
    queryKey: settingsKey(family),
    queryFn: async () => (await api.get<RotaReminderSettings>(settingsPath(family))) ?? null,
    staleTime: 60 * 1000,
  });
}

/**
 * Saves a group's rota reminder settings. The server refuses (400, 403) in
 * words, which surface as the error's message. Afterwards the settings are
 * read again, whether the save worked or not.
 */
export function useSaveRotaReminderSettings(
  family: RotaFamily,
): UseMutationResult<RotaReminderSettings, Error, SaveRotaReminderSettings> {
  const api = useBackendApi();
  const qc = useQueryClient();
  return useMutation<RotaReminderSettings, Error, SaveRotaReminderSettings>({
    mutationFn: (body) => api.put<SaveRotaReminderSettings, RotaReminderSettings>(settingsPath(family), body),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: settingsKey(family) });
    },
  });
}
