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

-- A runbook gets a required name, unique ignoring case and surrounding spaces; an existing runbook is named after its first alert. Safe to run again.
ALTER TABLE runbook ADD COLUMN IF NOT EXISTS name TEXT;

UPDATE runbook r
SET name = (SELECT a.alert_name FROM runbook_alert a WHERE a.runbook_id = r.id ORDER BY lower(a.alert_name), a.id LIMIT 1)
WHERE r.name IS NULL OR btrim(r.name) = '';

UPDATE runbook SET name = number WHERE name IS NULL;

ALTER TABLE runbook ALTER COLUMN name SET NOT NULL;
ALTER TABLE runbook DROP CONSTRAINT IF EXISTS runbook_name_check;
ALTER TABLE runbook ADD CONSTRAINT runbook_name_check CHECK (btrim(name) <> '' AND char_length(name) <= 1000);
CREATE UNIQUE INDEX IF NOT EXISTS runbook_name_key ON runbook (lower(btrim(name)));

COMMENT ON TABLE runbook IS
    'Runbook catalog: a numbered, named runbook link (NULL when not written yet) for the alert names in runbook_alert.';
