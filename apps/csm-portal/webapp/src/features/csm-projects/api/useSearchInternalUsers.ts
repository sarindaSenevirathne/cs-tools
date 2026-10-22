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

import { useMemo } from "react";
import { useSearchUsers } from "@features/csm-users/api/useSearchUsers";
import { INTERNAL_USER_ROLES } from "@features/csm-users/types/csmUsers";

/** One selectable internal user. */
export interface InternalUserOption {
  id: string;
  name: string;
  email: string;
}

/**
 * Type-ahead search over INTERNAL, active users only, for pickers that must
 * never offer an external (customer) user. The role and active scoping is sent
 * to `POST /users/search` so the backend applies it; the same constraints are
 * re-checked client-side (`userType`, `active`) as a defensive guard, exactly
 * as `AssignEngineerDialog` does. Rows without an id are dropped since the
 * consumer stores the id. Shaped for `AsyncEntitySelect`'s `useSearch`.
 */
export function useSearchInternalUsers(
  query: string,
  enabled: boolean,
): { data: InternalUserOption[] | undefined; isFetching: boolean; isError: boolean } {
  const { data, isFetching, isError } = useSearchUsers(
    {
      filters: {
        ...(query.length > 0 && { searchQuery: query }),
        roleIds: INTERNAL_USER_ROLES,
        active: true,
      },
      pagination: { limit: 10, offset: 0 },
    },
    enabled,
  );

  const users = useMemo<InternalUserOption[] | undefined>(() => {
    if (!data) return undefined;
    return data.users
      .filter(
        (u) =>
          !!u.id &&
          u.active !== false &&
          (u.userType ? u.userType === "internal" : true),
      )
      .map((u) => ({
        id: u.id,
        name: u.name || u.userName || u.email,
        email: u.email,
      }));
  }, [data]);

  return { data: users, isFetching, isError };
}
