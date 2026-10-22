-- Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
--
-- WSO2 LLC. licenses this file to you under the Apache License,
-- Version 2.0 (the "License"); you may not use this file except
-- in compliance with the License.
-- You may obtain a copy of the License at
--
-- http://www.apache.org/licenses/LICENSE-2.0
--
-- Unless required by applicable law or agreed to in writing,
-- software distributed under the License is distributed on an
-- "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
-- KIND, either express or implied.  See the License for the
-- specific language governing permissions and limitations
-- under the License.

-- Case Alerts: a group route names the ServiceNow assignment group it is for.
--
-- paging_group_route (0233) matched an incident's assignment group by its name
-- alone, typed on the Alert Settings tab: a rename in ServiceNow or a typo
-- broke the route without a word. group_id points at the "group" row the
-- route was saved for. group_name stays: it is the key the alert service
-- matches on, and the runtime settings now give the group's current name
-- through group_id, so a renamed group keeps its route.
--
-- Additive only: one nullable column, two indexes and a backfill. A route
-- with no group_id (a name no "group" row has, or one shared by two rows)
-- works exactly as before.
--
-- Re-runnable: IF NOT EXISTS throughout, and the backfill only fills an
-- empty group_id.

ALTER TABLE paging_group_route
    ADD COLUMN IF NOT EXISTS group_id UUID NULL REFERENCES "group"(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_paging_group_route_group_id ON paging_group_route (group_id);

-- One route per group per ladder, as there is one per group name.
CREATE UNIQUE INDEX IF NOT EXISTS paging_group_route_group_id_uq
    ON paging_group_route (ladder, group_id) WHERE group_id IS NOT NULL;

-- Existing routes: the group whose name they carry, when exactly one active
-- or inactive "group" row has it (names are not unique in "group").
UPDATE paging_group_route r
   SET group_id = g.id
  FROM (SELECT lower(btrim(name)) AS name, (array_agg(id))[1] AS id
          FROM "group"
         WHERE btrim(COALESCE(name, '')) <> ''
         GROUP BY lower(btrim(name))
        HAVING count(*) = 1) g
 WHERE r.group_id IS NULL
   AND r.group_name = g.name;
