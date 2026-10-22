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


export interface ListSort<F extends string> {
  field: F;
  order: "asc" | "desc";
}

/** Next sort after a click on `field`'s header: the active column flips its
 * direction, another column starts newest first. */
export function nextSort<F extends string>(current: ListSort<F>, field: F): ListSort<F> {
  if (current.field === field) return { field, order: current.order === "asc" ? "desc" : "asc" };
  return { field, order: "desc" };
}
