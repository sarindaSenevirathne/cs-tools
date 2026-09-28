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

-- Backs the Verified tab's "one row per work_item_id, its latest cycle"
-- DISTINCT ON (pv.work_item_id) ... ORDER BY pv.work_item_id, pv.added_on DESC
-- pattern (pending_verification_repo.go's Search, VerifiedOnly branch). The
-- pre-existing single-column idx_pending_verification_work_item_id index
-- doesn't carry added_on's order, so this query has to sort the matching
-- rows by (work_item_id, added_on DESC) itself before DISTINCT ON can pick a
-- winner per group. This composite index carries that exact order already.
CREATE INDEX IF NOT EXISTS idx_pending_verification_work_item_id_added_on
    ON pending_verification (work_item_id, added_on DESC);
