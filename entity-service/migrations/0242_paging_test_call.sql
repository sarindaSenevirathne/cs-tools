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

-- Case Alerts: the last "Test call" of a person's profile number.
--
-- A test call rings the number Case Alerts would dial: the one on the
-- person's own profile ("user".phone) when it can be called, else their
-- paging-only number. paging_contact.last_test_* (migration 0216) records the
-- tests of a paging-only number; this table records the tests of a profile
-- number, which has no paging_contact row to hold them. phone is the number
-- that was rung, so a result stops counting once the profile number changes.
--
-- Status only: no audit trigger, as for paging_contact.last_test_*.
-- Re-runnable: the table and its constraint are created only if missing.

CREATE TABLE IF NOT EXISTS paging_test_call (
    user_id          UUID PRIMARY KEY REFERENCES "user"(id) ON DELETE CASCADE,
    phone            VARCHAR(16) NOT NULL,
    requested_by     VARCHAR(255) NOT NULL,
    last_test_on     TIMESTAMPTZ NOT NULL,
    last_test_status VARCHAR(20) NOT NULL,
    CONSTRAINT paging_test_call_last_test_status_check
        CHECK (last_test_status IN ('pending', 'completed', 'no-answer', 'busy', 'failed'))
);
