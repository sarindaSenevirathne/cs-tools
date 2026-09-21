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

CREATE TYPE pending_verification_added_reason_enum AS ENUM ('AUTO_CLOSED', 'MANUAL');
CREATE TYPE pending_verification_previous_status_enum AS ENUM ('SOLUTION_PROPOSED', 'AWAITING_INFO');

CREATE TABLE IF NOT EXISTS pending_verification (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    work_item_id UUID NOT NULL REFERENCES work_item(id),
    added_reason pending_verification_added_reason_enum NOT NULL,
    previous_status pending_verification_previous_status_enum,
    added_on TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    added_by VARCHAR(255) NOT NULL,
    verified_on TIMESTAMPTZ,
    verified_by VARCHAR(255),
    note TEXT,

    -- previous_status is set iff the entry is auto-closed
    CONSTRAINT chk_previous_status_matches_reason
        CHECK ((added_reason = 'AUTO_CLOSED') = (previous_status IS NOT NULL)),

    -- verified_by is set iff verified_on is set
    CONSTRAINT chk_verified_by_matches_verified_on
        CHECK ((verified_on IS NULL) = (verified_by IS NULL))
);

CREATE INDEX IF NOT EXISTS idx_pending_verification_work_item_id ON pending_verification (work_item_id);
CREATE INDEX IF NOT EXISTS idx_pending_verification_verified_on ON pending_verification (verified_on);
