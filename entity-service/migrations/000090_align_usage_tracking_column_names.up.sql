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

-- 000054 named five columns differently from how digiops-cs/csm-sync-service's own
-- migrations actually built them in every real (stg/dev) database. Every reader of
-- these tables (internal/repository/instance_repo.go, deployed_product_repo.go,
-- project_stats_repo.go) already queries the sync-service names -- see
-- project_stats_repo.go's own comment on project_key -- so a database built fresh
-- from this repo's migrations alone (e.g. the phase-2 local stack) has never
-- actually matched this service's own code. This migration renames forward to the
-- names already treated as truth everywhere else, rather than the other way
-- around: sync-service is the schema of record for anything already live in stg.
ALTER TABLE deployment_node RENAME COLUMN subscription_key TO project_key;
ALTER INDEX IF EXISTS idx_deployment_node_subscription_key RENAME TO idx_deployment_node_project_key;

ALTER TABLE deployment_node RENAME COLUMN deployment_ref TO deployment_number;

ALTER TABLE deployment_information RENAME COLUMN reported_created_on TO payload_created_on;
ALTER TABLE deployment_information RENAME COLUMN reported_updated_on TO payload_updated_on;

-- number_of_cores was VARCHAR(64) here to tolerate a value like "8 (4 physical)"
-- (see 000054's own comment) -- but sync-service's real core_count column is
-- INTEGER, and every reader (instance_repo.go, deployed_product_repo.go) already
-- scans it as one. Extract the leading integer; anything that doesn't start with
-- a digit becomes NULL rather than failing the migration.
ALTER TABLE deployment_information
    ALTER COLUMN number_of_cores TYPE INTEGER
    USING (NULLIF(substring(number_of_cores FROM '^[0-9]+'), ''))::INTEGER;
ALTER TABLE deployment_information RENAME COLUMN number_of_cores TO core_count;

ALTER TABLE daily_usage_summary RENAME COLUMN deployment_ref TO deployment_number;
