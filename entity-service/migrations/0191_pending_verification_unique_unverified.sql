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

-- CreatePendingVerification had no deduplication at all: calling it twice for
-- the same work item (the auto-closure caller retrying on an ambiguous
-- timeout/5xx, or simply being invoked twice for one closure event) created
-- two separate, both-unverified rows for the same case. There is exactly one
-- meaningful "is this case currently pending verification" state per work
-- item -- verified_on IS NULL -- so a partial unique index on that state is
-- the correct constraint, same reasoning as alert_incident_mapping's own
-- plain UNIQUE on alert_number (migration 0103): enforce it once, at the
-- table, so it protects every future call site too, not just today's one
-- (the same lesson migration 0190's own RLS policies on this table already
-- apply).
--
-- Partial, not a plain UNIQUE on work_item_id: a case is allowed a second
-- round of pending verification after an earlier one was verified (re-added
-- after being closed again) -- see pending_verification_repo.go's Search,
-- whose own DISTINCT ON dedup logic already treats "latest cycle" as the
-- unit of a case's pending-verification history, not "ever had an entry".
CREATE UNIQUE INDEX IF NOT EXISTS pending_verification_one_unverified_per_work_item
  ON pending_verification (work_item_id)
  WHERE verified_on IS NULL;
