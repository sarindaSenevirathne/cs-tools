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

-- Ports the tables that exist in digiops-cs/operations/csm-sync-service's own
-- migrations but not here ("Bucket B" of the entity-service <-> csm-sync-service
-- schema diff). csm-sync-service is the schema of record for these tables
-- (same convention 000090 already applied for the usage-tracking rename) --
-- every CREATE TABLE below matches its sync-service source migration exactly:
-- same table/column names, types, nullability, defaults, FKs and indexes, even
-- where that departs from this repo's own naming habits. The point is
-- convergence, not re-litigating naming.
--
-- Excluded deliberately: csm_migration_checkpoint/csm_migration_job/
-- csm_migration_row_error/csm_migration_run/csm_migration_schema_version --
-- sync-service's own internal job-runner bookkeeping, not business schema.
--
-- Source migrations (digiops-cs/operations/csm-sync-service/migrations/):
-- 0044_kb_tables.sql, 0075_add_group_references.sql (knowledge_article's
-- ownership_group_id), 0050_schedule_tables.sql, 0075_add_group_references.sql
-- (schedule_span's group_id), 0079_cloud_monitor_table.sql,
-- 0079_customer_engagement_tables.sql, 0090_subject_columns_widen.sql
-- (customer_engagement_status_update.subject widened to VARCHAR(512)),
-- 0083_outage_table.sql, 0081_service_portfolio_table.sql,
-- 0082_service_commitment_table.sql, 0084_service_availability_table.sql,
-- 0081_sf_invoice_and_account_relationship.sql, 0080_sf_opportunity_tables.sql,
-- 0079_skill_tables.sql.

-- ============================================================================
-- Skills (ServiceNow Skills Management plugin mirror)
-- ============================================================================

CREATE TABLE IF NOT EXISTS skill_level_type (
    id UUID PRIMARY KEY,
    created_on TIMESTAMPTZ NOT NULL,
    updated_on TIMESTAMPTZ NOT NULL,
    created_by VARCHAR(255),
    updated_by VARCHAR(255),
    name VARCHAR(255) NOT NULL,
    description TEXT
);

CREATE TABLE IF NOT EXISTS skill_level (
    id UUID PRIMARY KEY,
    created_on TIMESTAMPTZ NOT NULL,
    updated_on TIMESTAMPTZ NOT NULL,
    created_by VARCHAR(255),
    updated_by VARCHAR(255),
    name VARCHAR(255) NOT NULL,
    value INTEGER,
    description TEXT,
    color VARCHAR(50),
    level_type_id UUID NOT NULL REFERENCES skill_level_type(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_skill_level_level_type_id ON skill_level (level_type_id);

-- parent is nullable: root categories have no parent.
CREATE TABLE IF NOT EXISTS skill_category (
    id UUID PRIMARY KEY,
    created_on TIMESTAMPTZ NOT NULL,
    updated_on TIMESTAMPTZ NOT NULL,
    created_by VARCHAR(255),
    updated_by VARCHAR(255),
    name VARCHAR(255) NOT NULL,
    code VARCHAR(100),
    category_path VARCHAR(500),
    is_lowest_category BOOLEAN NOT NULL DEFAULT false,
    parent_id UUID REFERENCES skill_category(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_skill_category_parent_id ON skill_category (parent_id);

CREATE TABLE IF NOT EXISTS skill (
    id UUID PRIMARY KEY,
    created_on TIMESTAMPTZ NOT NULL,
    updated_on TIMESTAMPTZ NOT NULL,
    created_by VARCHAR(255),
    updated_by VARCHAR(255),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    active BOOLEAN NOT NULL DEFAULT true,
    keywords VARCHAR(500),
    level_type_id UUID REFERENCES skill_level_type(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_skill_level_type_id ON skill (level_type_id);

CREATE TABLE IF NOT EXISTS skill_category_link (
    id UUID PRIMARY KEY,
    created_on TIMESTAMPTZ NOT NULL,
    updated_on TIMESTAMPTZ NOT NULL,
    created_by VARCHAR(255),
    updated_by VARCHAR(255),
    skill_id UUID NOT NULL REFERENCES skill(id) ON DELETE CASCADE,
    category_id UUID NOT NULL REFERENCES skill_category(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_skill_category_link_skill_id ON skill_category_link (skill_id);
CREATE INDEX IF NOT EXISTS idx_skill_category_link_category_id ON skill_category_link (category_id);

-- inherited_from_group_id deliberately has no FK: this table is migrated
-- unscoped (every WSO2 user), so it can point at any SN group, most of
-- which are never migrated into team - see sys_user_has_skill.yaml.
CREATE TABLE IF NOT EXISTS user_skill (
    id UUID PRIMARY KEY,
    created_on TIMESTAMPTZ NOT NULL,
    updated_on TIMESTAMPTZ NOT NULL,
    created_by VARCHAR(255),
    updated_by VARCHAR(255),
    user_id UUID NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    skill_id UUID NOT NULL REFERENCES skill(id) ON DELETE CASCADE,
    skill_level_id UUID REFERENCES skill_level(id) ON DELETE CASCADE,
    active BOOLEAN NOT NULL DEFAULT true,
    inherited BOOLEAN NOT NULL DEFAULT false,
    inherited_from_group_id UUID
);

CREATE INDEX IF NOT EXISTS idx_user_skill_user_id ON user_skill (user_id);
CREATE INDEX IF NOT EXISTS idx_user_skill_skill_id ON user_skill (skill_id);

CREATE TABLE IF NOT EXISTS user_skill_history (
    id UUID PRIMARY KEY,
    created_on TIMESTAMPTZ NOT NULL,
    updated_on TIMESTAMPTZ NOT NULL,
    created_by VARCHAR(255),
    updated_by VARCHAR(255),
    user_id UUID NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    skill_id UUID NOT NULL REFERENCES skill(id) ON DELETE CASCADE,
    level_from_id UUID REFERENCES skill_level(id) ON DELETE CASCADE,
    level_to_id UUID REFERENCES skill_level(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_user_skill_history_user_id ON user_skill_history (user_id);
CREATE INDEX IF NOT EXISTS idx_user_skill_history_skill_id ON user_skill_history (skill_id);

-- ============================================================================
-- Schedules (ServiceNow support-rotation roster mirror: cmn_schedule ->
-- schedule, agent_events -> user_schedule, cmn_schedule_span -> schedule_span)
-- ============================================================================

CREATE TABLE IF NOT EXISTS schedule (
    id UUID PRIMARY KEY,
    created_on TIMESTAMPTZ NOT NULL,
    updated_on TIMESTAMPTZ NOT NULL,
    created_by VARCHAR(255),
    updated_by VARCHAR(255),
    name VARCHAR(255),
    type VARCHAR(100),
    time_zone VARCHAR(64),
    read_only BOOLEAN
);

-- agent_events: binds a user to a schedule. No UNIQUE on schedule_id,
-- deliberately -- one user can hold several schedules.
CREATE TABLE IF NOT EXISTS user_schedule (
    id UUID PRIMARY KEY,
    created_on TIMESTAMPTZ NOT NULL,
    updated_on TIMESTAMPTZ NOT NULL,
    created_by VARCHAR(255),
    updated_by VARCHAR(255),
    user_id UUID NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    schedule_id UUID NOT NULL REFERENCES schedule(id) ON DELETE CASCADE,
    active BOOLEAN
);

CREATE INDEX IF NOT EXISTS idx_user_schedule_user_id ON user_schedule (user_id);
CREATE INDEX IF NOT EXISTS idx_user_schedule_schedule_id ON user_schedule (schedule_id);

-- cmn_schedule_span: the roster rows themselves, and the leave rows.
-- start_on/end_on are TIMESTAMP WITHOUT TIME ZONE deliberately -- they are
-- wall-clock times in the owning schedule's own time_zone, not instants.
-- group_id (from csm-sync-service's own 0075_add_group_references.sql) is
-- included directly here rather than as a separate ALTER, since this table
-- has no prior migration in this repo to alter.
CREATE TABLE IF NOT EXISTS schedule_span (
    id UUID PRIMARY KEY,
    created_on TIMESTAMPTZ NOT NULL,
    updated_on TIMESTAMPTZ NOT NULL,
    created_by VARCHAR(255),
    updated_by VARCHAR(255),
    schedule_id UUID NOT NULL REFERENCES schedule(id) ON DELETE CASCADE,
    span_type VARCHAR(50),
    start_on TIMESTAMP,
    end_on TIMESTAMP,
    all_day BOOLEAN,
    name VARCHAR(255),
    notes TEXT,
    user_id UUID REFERENCES "user"(id) ON DELETE SET NULL,
    team_id UUID REFERENCES team(id) ON DELETE SET NULL,
    group_id UUID REFERENCES "group"(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_schedule_span_type_window ON schedule_span (span_type, start_on, end_on);
CREATE INDEX IF NOT EXISTS idx_schedule_span_schedule_id ON schedule_span (schedule_id);
CREATE INDEX IF NOT EXISTS idx_schedule_span_user_id ON schedule_span (user_id);
CREATE INDEX IF NOT EXISTS idx_schedule_span_group_id ON schedule_span (group_id);

-- ============================================================================
-- Knowledge base (ServiceNow kb_knowledge_base/kb_knowledge mirror)
-- ============================================================================

-- knowledge_base: the article container. kb_managers and product association
-- are deliberately not mapped - no local groups/teams concept to resolve
-- kb_managers against yet, and no reliable KB-to-product derivation exists.
CREATE TABLE IF NOT EXISTS knowledge_base (
    id UUID PRIMARY KEY,
    created_on TIMESTAMPTZ NOT NULL,
    updated_on TIMESTAMPTZ NOT NULL,
    created_by VARCHAR(255) NOT NULL,
    updated_by VARCHAR(255) NOT NULL,
    title VARCHAR(255) NOT NULL,
    active BOOLEAN
);

-- knowledge_article: one row per ServiceNow kb_knowledge row, including
-- historical versions - there is no separate history table. "History" is
-- just other rows sharing a base_version_id lineage; latest=true marks the
-- current one. ownership_group_id is from csm-sync-service's own
-- 0075_add_group_references.sql, included directly here.
CREATE TABLE IF NOT EXISTS knowledge_article (
    id UUID PRIMARY KEY,
    created_on TIMESTAMPTZ NOT NULL,
    updated_on TIMESTAMPTZ NOT NULL,
    created_by VARCHAR(255) NOT NULL,
    updated_by VARCHAR(255) NOT NULL,
    number VARCHAR(100),
    title VARCHAR(255) NOT NULL,
    body TEXT,
    state VARCHAR(40),
    knowledge_base_id UUID REFERENCES knowledge_base(id) ON DELETE SET NULL,
    author_id UUID REFERENCES "user"(id) ON DELETE SET NULL,
    revised_by_id UUID REFERENCES "user"(id) ON DELETE SET NULL,
    source_case_id UUID REFERENCES work_item(id) ON DELETE SET NULL,
    base_version_id UUID REFERENCES knowledge_article(id) ON DELETE SET NULL,
    latest BOOLEAN,
    rejection_comment TEXT,
    generated_with_ai BOOLEAN,
    ai_generated_by VARCHAR(100),
    helpful_count INTEGER,
    rating NUMERIC,
    use_count INTEGER,
    view_count INTEGER,
    published_on TIMESTAMPTZ,
    retired_on TIMESTAMPTZ,
    scheduled_publish_on TIMESTAMPTZ,
    ownership_group_id UUID REFERENCES "group"(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_knowledge_article_knowledge_base_id ON knowledge_article (knowledge_base_id);
CREATE INDEX IF NOT EXISTS idx_knowledge_article_author_id ON knowledge_article (author_id);
CREATE INDEX IF NOT EXISTS idx_knowledge_article_revised_by_id ON knowledge_article (revised_by_id);
CREATE INDEX IF NOT EXISTS idx_knowledge_article_source_case_id ON knowledge_article (source_case_id);
CREATE INDEX IF NOT EXISTS idx_knowledge_article_base_version_id ON knowledge_article (base_version_id);
CREATE INDEX IF NOT EXISTS idx_knowledge_article_ownership_group_id ON knowledge_article (ownership_group_id);

-- ============================================================================
-- Cloud monitor (SaaS status-page mirror)
-- ============================================================================

DO $$ BEGIN
    CREATE TYPE cloud_monitor_cloud_offering_enum AS ENUM (
        'ASGARDEO', 'BIJIRA', 'AGENT_MANAGER', 'CHOREO_EU', 'MOESIF', 'DEVANT', 'CHOREO'
    );
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    CREATE TYPE cloud_monitor_status_enum AS ENUM (
        'PARTIAL_OUTAGE', 'MAINTENANCE', 'DEGRADED', 'OPERATIONAL', 'MAJOR_OUTAGE'
    );
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

CREATE TABLE IF NOT EXISTS cloud_monitor (
    id UUID PRIMARY KEY,
    created_on TIMESTAMPTZ NOT NULL,
    updated_on TIMESTAMPTZ NOT NULL,
    created_by VARCHAR(255) NOT NULL,
    updated_by VARCHAR(255) NOT NULL,
    name VARCHAR(255),
    "group" VARCHAR(255),
    group_priority INTEGER,
    description TEXT,
    service_offering_id UUID REFERENCES service_offering(id) ON DELETE SET NULL,
    service_id UUID REFERENCES service(id) ON DELETE SET NULL,
    cloud_offering cloud_monitor_cloud_offering_enum,
    region VARCHAR(100),
    status cloud_monitor_status_enum,
    is_active BOOLEAN
);

-- ============================================================================
-- Customer engagements, resource allocations and weekly status updates
-- ============================================================================

DO $$ BEGIN
    CREATE TYPE customer_engagement_state_enum AS ENUM
        ('NEW', 'IN_PROGRESS', 'COMPLETED', 'CANCELLED', 'ON_HOLD', 'REQUESTED');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    CREATE TYPE customer_engagement_business_unit_enum AS ENUM
        ('API_AND_INTEGRATION_SOFTWARE', 'CHOREO', 'IAM');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    CREATE TYPE customer_engagement_consumption_mode_enum AS ENUM
        ('ONE_TIME_FIXED_START', 'ONE_TIME_FLEXIBLE_START', 'MULTIPLE_SPANS');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    CREATE TYPE customer_engagement_delivery_mode_enum AS ENUM ('OFFSITE', 'ONSITE');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- Nine values, and the three rejection/cancellation ones are why this column
-- is worth mapping at all.
DO $$ BEGIN
    CREATE TYPE engagement_allocation_state_enum AS ENUM (
        'TENTATIVE',
        'READY_FOR_CLEARANCE',
        'CONFIRMED',
        'CLEARANCE_IN_PROGRESS',
        'ALLOCATION_CANCELLED',
        'ACCEPTED_BY_CONSULTANT',
        'REJECTED_OTHER',
        'REJECTED_BY_CONSULTANT',
        'CONFIRMED_VISA_PENDING'
    );
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    CREATE TYPE customer_engagement_status_update_state_enum AS ENUM ('DRAFT', 'PUBLISHED', 'CANCELLED');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    CREATE TYPE customer_engagement_status_update_frequency_enum AS ENUM
        ('DAILY', 'WEEKLY', 'BIWEEKLY', 'MONTHLY', 'ON_DEMAND');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

DO $$ BEGIN
    CREATE TYPE customer_engagement_status_update_scope_enum AS ENUM ('INDIVIDUAL', 'PROJECT');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- Raw values are "0"/"1"; the labels are Internal/Customer.
DO $$ BEGIN
    CREATE TYPE customer_engagement_status_update_visibility_enum AS ENUM ('INTERNAL', 'CUSTOMER');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- ---------- the engagement ----------
CREATE TABLE IF NOT EXISTS customer_engagement (
    id                  UUID PRIMARY KEY,
    created_on          TIMESTAMPTZ NOT NULL,
    updated_on          TIMESTAMPTZ NOT NULL,
    created_by          VARCHAR(255),
    updated_by          VARCHAR(255),
    name                VARCHAR(200),
    engagement_id       VARCHAR(20),
    engagement_code     VARCHAR(20),
    state               customer_engagement_state_enum,
    business_unit       customer_engagement_business_unit_enum,
    consumption_mode    customer_engagement_consumption_mode_enum,
    delivery_mode       customer_engagement_delivery_mode_enum,
    is_paid             BOOLEAN,
    account_id          UUID REFERENCES account(id) ON DELETE SET NULL,
    project_id          UUID REFERENCES project(id) ON DELETE SET NULL,
    lead_id             UUID REFERENCES "user"(id) ON DELETE SET NULL,
    owner_id            UUID REFERENCES "user"(id) ON DELETE SET NULL,
    -- Unsynced reference targets -- raw sys_ids, not FKs.
    engagement_type_id  VARCHAR(32),
    opportunity_id      VARCHAR(32),
    line_item_id_ref    VARCHAR(32),
    -- u_line_item_id is a separate string(40) field from the u_line_item
    -- reference above, despite the near-identical name.
    line_item_id        VARCHAR(40),
    planned_start_date  DATE,
    planned_end_date    DATE,
    actual_start_date   DATE,
    actual_end_date     DATE
);

CREATE INDEX IF NOT EXISTS idx_customer_engagement_state   ON customer_engagement (state);
CREATE INDEX IF NOT EXISTS idx_customer_engagement_account ON customer_engagement (account_id);
CREATE INDEX IF NOT EXISTS idx_customer_engagement_project ON customer_engagement (project_id);

-- ---------- who is allocated ----------
CREATE TABLE IF NOT EXISTS customer_engagement_allocation_resource (
    id            UUID PRIMARY KEY,
    created_on    TIMESTAMPTZ NOT NULL,
    updated_on    TIMESTAMPTZ NOT NULL,
    created_by    VARCHAR(255),
    updated_by    VARCHAR(255),
    -- u_allocation_id: a human-readable reference, not a sys_id.
    allocation_id VARCHAR(20),
    engagement_id UUID REFERENCES customer_engagement(id) ON DELETE CASCADE,
    resource_id   UUID REFERENCES "user"(id) ON DELETE SET NULL,
    state         engagement_allocation_state_enum,
    -- DATE, not TIMESTAMPTZ: the source fields are glide_date (no time part).
    start_date    DATE,
    end_date      DATE,
    -- The time-of-day halves are string(8) upstream ("09:00:00"), kept as
    -- text rather than TIME: they are wall-clock in timezone below.
    start_time    VARCHAR(8),
    end_time      VARCHAR(8),
    timezone      VARCHAR(20),
    -- percent_complete upstream; a percentage, so NUMERIC not INTEGER.
    utilization   NUMERIC(6,2)
);

CREATE INDEX IF NOT EXISTS idx_cea_resource_engagement ON customer_engagement_allocation_resource (engagement_id);
CREATE INDEX IF NOT EXISTS idx_cea_resource_resource   ON customer_engagement_allocation_resource (resource_id);
CREATE INDEX IF NOT EXISTS idx_cea_resource_dates      ON customer_engagement_allocation_resource (start_date, end_date);
CREATE INDEX IF NOT EXISTS idx_cea_resource_state      ON customer_engagement_allocation_resource (state);

-- ---------- the weekly update ----------
-- subject is VARCHAR(512), already widened here to match sync-service's own
-- 0090_subject_columns_widen.sql, rather than creating at 160 and altering.
CREATE TABLE IF NOT EXISTS customer_engagement_status_update (
    id               UUID PRIMARY KEY,
    created_on       TIMESTAMPTZ NOT NULL,
    updated_on       TIMESTAMPTZ NOT NULL,
    created_by       VARCHAR(255),
    updated_by       VARCHAR(255),
    engagement_id    UUID REFERENCES customer_engagement(id) ON DELETE CASCADE,
    allocation_id    UUID REFERENCES customer_engagement_allocation_resource(id) ON DELETE SET NULL,
    author_id        UUID REFERENCES "user"(id) ON DELETE SET NULL,
    subject          VARCHAR(512),
    -- html(8000) upstream. TEXT rather than VARCHAR(8000): Postgres stores
    -- both the same way, and a length cap here would reject an update the
    -- source accepted.
    content          TEXT,
    state            customer_engagement_status_update_state_enum,
    frequency        customer_engagement_status_update_frequency_enum,
    scope            customer_engagement_status_update_scope_enum,
    visibility       customer_engagement_status_update_visibility_enum,
    cycle_start_date DATE,
    published_date   TIMESTAMPTZ,
    -- string(2500) upstream: a free-text recipient list, not a reference.
    mailing_list     VARCHAR(2500)
);

CREATE INDEX IF NOT EXISTS idx_ce_status_update_lookup ON customer_engagement_status_update (engagement_id, author_id, cycle_start_date);
CREATE INDEX IF NOT EXISTS idx_ce_status_update_cycle  ON customer_engagement_status_update (cycle_start_date);
CREATE INDEX IF NOT EXISTS idx_ce_status_update_alloc  ON customer_engagement_status_update (allocation_id);

-- ============================================================================
-- Outages
-- ============================================================================

DO $$ BEGIN
    CREATE TYPE outage_type_enum AS ENUM ('DEGRADATION', 'OUTAGE', 'PLANNED');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

-- cmdb_ci is polymorphic on the ServiceNow side: it can point at either a
-- service or a service_offering row. Both FKs are nullable and exactly one
-- is populated per row.
CREATE TABLE IF NOT EXISTS outage (
    id UUID PRIMARY KEY,
    created_on TIMESTAMPTZ NOT NULL,
    updated_on TIMESTAMPTZ NOT NULL,
    created_by VARCHAR(255) NOT NULL,
    updated_by VARCHAR(255) NOT NULL,
    number VARCHAR(255) UNIQUE,
    service_id UUID REFERENCES service(id) ON DELETE SET NULL,
    service_offering_id UUID REFERENCES service_offering(id) ON DELETE SET NULL,
    work_item_id UUID REFERENCES work_item(id) ON DELETE SET NULL,
    name VARCHAR(255),
    message VARCHAR(255),
    type outage_type_enum,
    start_on TIMESTAMPTZ,
    end_on TIMESTAMPTZ,
    duration INTERVAL,
    external_outage_communications TEXT,
    additional_outage_comments TEXT,
    internal_outage_communications TEXT
);

-- ============================================================================
-- Service portfolio, service commitments and availability
-- ============================================================================

CREATE TABLE IF NOT EXISTS service_portfolio (
    id UUID PRIMARY KEY,
    created_on TIMESTAMPTZ NOT NULL,
    updated_on TIMESTAMPTZ NOT NULL,
    created_by VARCHAR(255) NOT NULL,
    updated_by VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    auto_create_service_offerings_enabled BOOLEAN,
    offerings_per_service INTEGER CONSTRAINT chk_service_portfolio_offerings_per_service CHECK (offerings_per_service BETWEEN 1 AND 20),
    is_active BOOLEAN
);

DO $$ BEGIN
    CREATE TYPE service_commitment_type_enum AS ENUM (
        'AVAILABILITY', 'DELIVERY', 'MAINTENANCE_WINDOW', 'OTHER',
        'RECOVERY_POINT_OBJECTIVE', 'RECOVERY_TIME_OBJECTIVE', 'RESPONSE_TIME'
    );
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

CREATE TABLE IF NOT EXISTS service_commitment (
    id UUID PRIMARY KEY,
    created_on TIMESTAMPTZ NOT NULL,
    updated_on TIMESTAMPTZ NOT NULL,
    created_by VARCHAR(255) NOT NULL,
    updated_by VARCHAR(255) NOT NULL,
    name VARCHAR(255),
    description TEXT,
    type service_commitment_type_enum,
    precision INTEGER,
    percentage_avail NUMERIC,
    per VARCHAR(50),
    -- schedule is dot-walked to schedule.name at extraction time and stored
    -- as a plain display string, not an FK to the schedule table above.
    schedule VARCHAR(255),
    timezone VARCHAR(100),
    breach_penalty_amount NUMERIC(20,4)
);

DO $$ BEGIN
    CREATE TYPE service_availability_type_enum AS ENUM (
        'ANNUALLY', 'DAILY', 'LAST_12_MONTHS', 'LAST_1_DAYS', 'LAST_30_DAYS',
        'LAST_7_DAYS', 'LAST_90_DAYS', 'MONTHLY', 'WEEKLY'
    );
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

CREATE TABLE IF NOT EXISTS service_availability (
    id UUID PRIMARY KEY,
    created_on TIMESTAMPTZ NOT NULL,
    updated_on TIMESTAMPTZ NOT NULL,
    created_by VARCHAR(255) NOT NULL,
    updated_by VARCHAR(255) NOT NULL,
    name VARCHAR(255),
    type service_availability_type_enum,
    service_offering_id UUID REFERENCES service_offering(id) ON DELETE SET NULL,
    service_commitment_id UUID REFERENCES service_commitment(id) ON DELETE SET NULL,
    start_on TIMESTAMPTZ,
    end_on TIMESTAMPTZ,
    time_zone VARCHAR(255),
    absolute_downtime_duration INTERVAL,
    scheduled_downtime_duration INTERVAL,
    absolute_count INTEGER,
    scheduled_count INTEGER,
    absolute_availability NUMERIC,
    scheduled_availability NUMERIC,
    committed_uptime_duration INTERVAL,
    mtbf_duration INTERVAL,
    mtrs_duration INTERVAL,
    is_commitment_met BOOLEAN,
    allowed_downtime_duration INTERVAL
);

-- ============================================================================
-- Account relationships and Salesforce opportunity/invoice mirror
-- ============================================================================

-- account_relationship: a ServiceNow BASELINE CSM table (0 custom columns).
-- from_company/to_company are renamed to from_account_id/to_account_id, since
-- "company" is ServiceNow's legacy word for what this repo calls an account.
CREATE TABLE IF NOT EXISTS account_relationship (
    id UUID PRIMARY KEY,
    created_on TIMESTAMPTZ NOT NULL,
    updated_on TIMESTAMPTZ NOT NULL,
    created_by VARCHAR(255) NOT NULL,
    updated_by VARCHAR(255) NOT NULL,
    from_account_id UUID REFERENCES account(id) ON DELETE CASCADE,
    to_account_id UUID REFERENCES account(id) ON DELETE CASCADE,
    -- relationship_type references sn_customerservice_account_relationship_type,
    -- which is not synced, so it is carried as a raw sys_id string.
    relationship_type_id VARCHAR(32),
    relationship_label VARCHAR(40),
    reverse_relationship_label VARCHAR(40),
    -- A row is stored once and read from either end; this flags which
    -- direction the labels are written for.
    is_reverse_relationship BOOLEAN,
    -- "order" is a reserved word; renamed rather than quoted, since nothing
    -- builds this table's SQL dynamically.
    display_order INTEGER
);

CREATE INDEX IF NOT EXISTS idx_account_relationship_from ON account_relationship (from_account_id);
CREATE INDEX IF NOT EXISTS idx_account_relationship_to ON account_relationship (to_account_id);

-- The Salesforce opportunity tables, mirrored so query-hour entitlement can be
-- computed in Postgres. Names drop the u_/u_sf_ shape of the source per
-- sync-service's own de-awkwarding convention (u_sf_link_opportunity ->
-- sf_opportunity_link).

-- Choice values confirmed on u_sf_opportunity.u_query_hour_state: 1 = Notified
-- (75%), 2 = Notified (90%), 3 = Closure Notice (100%/Exceed). There is no
-- explicit "under 75%" choice, so the enum has no NORMAL member and the
-- column stays nullable.
DO $$ BEGIN
    CREATE TYPE query_hour_state_enum AS ENUM (
        'NOTIFIED_75', 'NOTIFIED_90', 'CLOSURE_NOTICE_100'
    );
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

CREATE TABLE IF NOT EXISTS sf_opportunity (
    id UUID PRIMARY KEY,
    created_on TIMESTAMPTZ NOT NULL,
    updated_on TIMESTAMPTZ NOT NULL,
    created_by VARCHAR(255) NOT NULL,
    updated_by VARCHAR(255) NOT NULL,
    -- u_opportunity_id: the Salesforce Opportunity id. Distinct from `id`,
    -- which is the ServiceNow sys_id rewritten as a UUID.
    sf_id VARCHAR(40),
    name VARCHAR(200),
    account_id UUID REFERENCES account(id) ON DELETE SET NULL,
    stage VARCHAR(40),
    type VARCHAR(40),
    is_won BOOLEAN,
    close_date DATE,
    -- u_owner is a plain string on the source, not a reference to sys_user.
    owner VARCHAR(40),
    engagement_code VARCHAR(40),
    eula_version VARCHAR(160),
    eula_version_decimal NUMERIC,
    query_hour_state query_hour_state_enum,
    sync_time_stamp TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_sf_opportunity_account_id ON sf_opportunity (account_id);
CREATE INDEX IF NOT EXISTS idx_sf_opportunity_sf_id ON sf_opportunity (sf_id);

CREATE TABLE IF NOT EXISTS sf_opportunity_product (
    id UUID PRIMARY KEY,
    created_on TIMESTAMPTZ NOT NULL,
    updated_on TIMESTAMPTZ NOT NULL,
    created_by VARCHAR(255) NOT NULL,
    updated_by VARCHAR(255) NOT NULL,
    -- u_id: the Salesforce OpportunityLineItem id, used in the lightning
    -- deep-links ServiceNow builds into its emails.
    line_item_sf_id VARCHAR(40),
    name VARCHAR(200),
    opportunity_id UUID REFERENCES sf_opportunity(id) ON DELETE CASCADE,
    -- The entitlement inputs. quantity x hours-per-pack is the whole
    -- calculation; ServiceNow derives hours-per-pack by string-matching
    -- product_name against "Development Support - N hours".
    product_name VARCHAR(100),
    quantity NUMERIC,
    service_start_date DATE,
    service_end_date DATE,
    -- u_development_support_hours: a DECIMAL column that already holds the
    -- number ServiceNow re-derives from the product name.
    development_support_hours NUMERIC,
    product_code VARCHAR(70),
    product_description VARCHAR(250),
    product_family VARCHAR(100),
    -- u_product_id is the Salesforce Product2 id, a string; u_product is a
    -- reference to cmdb_model, which is not mirrored, so only the Salesforce
    -- id is carried.
    product_sf_id VARCHAR(40),
    product_unit VARCHAR(40),
    classification VARCHAR(40),
    engagement_code VARCHAR(40),
    eng_product_code VARCHAR(40),
    environment VARCHAR(40),
    total_price NUMERIC,
    sync_time_stamp TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_sf_opportunity_product_opportunity_id
    ON sf_opportunity_product (opportunity_id);
-- The entitlement query filters on product_name and the service window.
CREATE INDEX IF NOT EXISTS idx_sf_opportunity_product_entitlement
    ON sf_opportunity_product (product_name, service_start_date, service_end_date);

-- The opportunity <-> project join -- the only path from a project to the
-- product lines that fund it.
CREATE TABLE IF NOT EXISTS sf_opportunity_link (
    id UUID PRIMARY KEY,
    created_on TIMESTAMPTZ NOT NULL,
    updated_on TIMESTAMPTZ NOT NULL,
    created_by VARCHAR(255) NOT NULL,
    updated_by VARCHAR(255) NOT NULL,
    -- u_id: the Salesforce record id for the link itself.
    link_sf_id VARCHAR(40),
    -- u_name is a human-readable code (LO-26-09-N-00034824), not a title.
    number VARCHAR(40),
    opportunity_id UUID REFERENCES sf_opportunity(id) ON DELETE CASCADE,
    project_id UUID REFERENCES project(id) ON DELETE CASCADE,
    sync_time_stamp TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_sf_opportunity_link_opportunity_id
    ON sf_opportunity_link (opportunity_id);
CREATE INDEX IF NOT EXISTS idx_sf_opportunity_link_project_id
    ON sf_opportunity_link (project_id);

-- u_sf_invoice -> sf_invoice. Hangs off an opportunity, so it is created
-- after sf_opportunity above. Every column here is a u_ customisation apart
-- from the sys_* six -- a WSO2 table that happens to live in the Global
-- scope, not a baseline one.
CREATE TABLE IF NOT EXISTS sf_invoice (
    id UUID PRIMARY KEY,
    created_on TIMESTAMPTZ NOT NULL,
    updated_on TIMESTAMPTZ NOT NULL,
    created_by VARCHAR(255) NOT NULL,
    updated_by VARCHAR(255) NOT NULL,
    -- u_id: the Salesforce record id (a0IE... -- an a-prefixed custom object).
    sf_id VARCHAR(40),
    -- u_name is a short code (e.g. "2508"), not a title.
    name VARCHAR(40),
    description VARCHAR(40),
    classification VARCHAR(40),
    opportunity_id UUID REFERENCES sf_opportunity(id) ON DELETE SET NULL,
    invoiced_amount NUMERIC,
    invoice_date DATE,
    -- Three separate due dates on the source, all kept: the ACP
    -- invoice-closure flows distinguish the current due date from the one
    -- originally agreed, and `paid` is what closes the loop.
    invoiced_due_date DATE,
    original_invoice_due_date DATE,
    invoiced_paid_date DATE,
    service_start_date DATE,
    service_end_date DATE,
    sync_time_stamp TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_sf_invoice_opportunity_id ON sf_invoice (opportunity_id);
CREATE INDEX IF NOT EXISTS idx_sf_invoice_sf_id ON sf_invoice (sf_id);
-- "which invoices are overdue and unpaid" is the question the closure flows
-- ask, so it gets an index rather than a scan.
CREATE INDEX IF NOT EXISTS idx_sf_invoice_due_unpaid
    ON sf_invoice (invoiced_due_date)
    WHERE invoiced_paid_date IS NULL;
