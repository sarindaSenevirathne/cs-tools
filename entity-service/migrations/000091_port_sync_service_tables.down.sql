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

-- Reverses 000091_port_sync_service_tables.up.sql. Tables are dropped in
-- reverse dependency order; enum types are dropped after every table that
-- references them is gone.

DROP TABLE IF EXISTS sf_invoice;
DROP TABLE IF EXISTS sf_opportunity_link;
DROP TABLE IF EXISTS sf_opportunity_product;
DROP TABLE IF EXISTS sf_opportunity;
DROP TYPE IF EXISTS query_hour_state_enum;

DROP TABLE IF EXISTS account_relationship;

DROP TABLE IF EXISTS service_availability;
DROP TYPE IF EXISTS service_availability_type_enum;

DROP TABLE IF EXISTS service_commitment;
DROP TYPE IF EXISTS service_commitment_type_enum;

DROP TABLE IF EXISTS service_portfolio;

DROP TABLE IF EXISTS outage;
DROP TYPE IF EXISTS outage_type_enum;

DROP TABLE IF EXISTS customer_engagement_status_update;
DROP TYPE IF EXISTS customer_engagement_status_update_visibility_enum;
DROP TYPE IF EXISTS customer_engagement_status_update_scope_enum;
DROP TYPE IF EXISTS customer_engagement_status_update_frequency_enum;
DROP TYPE IF EXISTS customer_engagement_status_update_state_enum;

DROP TABLE IF EXISTS customer_engagement_allocation_resource;
DROP TYPE IF EXISTS engagement_allocation_state_enum;

DROP TABLE IF EXISTS customer_engagement;
DROP TYPE IF EXISTS customer_engagement_delivery_mode_enum;
DROP TYPE IF EXISTS customer_engagement_consumption_mode_enum;
DROP TYPE IF EXISTS customer_engagement_business_unit_enum;
DROP TYPE IF EXISTS customer_engagement_state_enum;

DROP TABLE IF EXISTS cloud_monitor;
DROP TYPE IF EXISTS cloud_monitor_status_enum;
DROP TYPE IF EXISTS cloud_monitor_cloud_offering_enum;

DROP TABLE IF EXISTS knowledge_article;
DROP TABLE IF EXISTS knowledge_base;

DROP TABLE IF EXISTS schedule_span;
DROP TABLE IF EXISTS user_schedule;
DROP TABLE IF EXISTS schedule;

DROP TABLE IF EXISTS user_skill_history;
DROP TABLE IF EXISTS user_skill;
DROP TABLE IF EXISTS skill_category_link;
DROP TABLE IF EXISTS skill;
DROP TABLE IF EXISTS skill_category;
DROP TABLE IF EXISTS skill_level;
DROP TABLE IF EXISTS skill_level_type;
