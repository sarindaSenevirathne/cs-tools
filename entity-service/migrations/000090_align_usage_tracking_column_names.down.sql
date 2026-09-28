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

ALTER TABLE daily_usage_summary RENAME COLUMN deployment_number TO deployment_ref;

ALTER TABLE deployment_information RENAME COLUMN core_count TO number_of_cores;
ALTER TABLE deployment_information ALTER COLUMN number_of_cores TYPE VARCHAR(64) USING number_of_cores::VARCHAR;

ALTER TABLE deployment_information RENAME COLUMN payload_updated_on TO reported_updated_on;
ALTER TABLE deployment_information RENAME COLUMN payload_created_on TO reported_created_on;

ALTER TABLE deployment_node RENAME COLUMN deployment_number TO deployment_ref;

ALTER INDEX IF EXISTS idx_deployment_node_project_key RENAME TO idx_deployment_node_subscription_key;
ALTER TABLE deployment_node RENAME COLUMN project_key TO subscription_key;
