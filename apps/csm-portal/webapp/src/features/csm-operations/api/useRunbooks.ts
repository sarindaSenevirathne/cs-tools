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
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from "@tanstack/react-query";
import { useBackendApi } from "@api/backend/client";
import type { Runbook, RunbookInput } from "@features/csm-operations/utils/runbooks";

const RUNBOOKS_KEY = ["operations", "runbooks"] as const;
const BASE = "/runbooks";

interface RunbookList {
  runbooks: Runbook[];
}

export interface ImportRunbooksResult {
  created: number;
  updated: number;
  /** Runbooks left without alerts because the file moved all of them elsewhere, with the link each had. */
  removed: { number: string; name: string; runbookUrl: string | null }[];
}

/** The whole runbook catalog; it is small enough to filter and page on the client. */
export function useRunbooks(): UseQueryResult<Runbook[], Error> {
  const api = useBackendApi();
  return useQuery<Runbook[], Error>({
    queryKey: RUNBOOKS_KEY,
    queryFn: async () => (await api.get<RunbookList>(BASE))?.runbooks ?? [],
    staleTime: 60 * 1000,
  });
}

/** Every change reads the catalog again, so the list matches the server. */
function useRunbookMutation<TVars, TResult>(
  fn: (vars: TVars) => Promise<TResult>,
): UseMutationResult<TResult, Error, TVars> {
  const qc = useQueryClient();
  return useMutation<TResult, Error, TVars>({
    mutationFn: fn,
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: RUNBOOKS_KEY });
    },
  });
}

export function useCreateRunbook(): UseMutationResult<Runbook, Error, RunbookInput> {
  const api = useBackendApi();
  return useRunbookMutation((input: RunbookInput) => api.post<RunbookInput, Runbook>(BASE, input));
}

export function useUpdateRunbook(): UseMutationResult<Runbook, Error, { id: string; input: RunbookInput }> {
  const api = useBackendApi();
  return useRunbookMutation(({ id, input }: { id: string; input: RunbookInput }) =>
    api.put<RunbookInput, Runbook>(`${BASE}/${encodeURIComponent(id)}`, input),
  );
}

export function useDeleteRunbook(): UseMutationResult<null, Error, string> {
  const api = useBackendApi();
  return useRunbookMutation((id: string) => api.del<null>(`${BASE}/${encodeURIComponent(id)}`));
}

export function useImportRunbooks(): UseMutationResult<ImportRunbooksResult, Error, RunbookInput[]> {
  const api = useBackendApi();
  return useRunbookMutation((runbooks: RunbookInput[]) =>
    api.post<{ runbooks: RunbookInput[] }, ImportRunbooksResult>(`${BASE}/import`, { runbooks }),
  );
}
