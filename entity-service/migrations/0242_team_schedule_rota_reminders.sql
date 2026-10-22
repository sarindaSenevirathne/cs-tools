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

-- The CRE rota reminders' three tables, one migration for the feature:
--
--   team_schedule_reminder_sent     what the weekend reminder last sent
--                                   (so it sends once, and Friday can update
--                                   or cancel calendar invitations)
--   team_schedule_reminder_setting  the Rota Access card's saved settings
--   team_schedule_reminder_request  "Send now" / "Send a test to me" presses
--                                   queued for csm-scheduled-tasks
--
-- Additive and safe to re-run: each table, index and trigger is created only
-- if missing; nothing existing is changed.

-- Team Schedule reminders: what csm-scheduled-tasks last sent for one rota
-- period, so a run can tell "already sent" from "not yet" and cancel a
-- calendar invite for somebody taken off the rota since.
--
-- One row per (family, kind, period_start). kind is the reminder, e.g.
-- 'weekend_notice'; period_start the first rota day it covers (a weekend's
-- Saturday). invites is the last set of calendar events sent:
-- [{"uid": "...", "sequence": 3, "attendees": ["a@example.com", ...]}].
--
-- Additive and safe to re-run: nothing existing is touched.
CREATE TABLE IF NOT EXISTS team_schedule_reminder_sent (
    family        VARCHAR(16)  NOT NULL,
    kind          VARCHAR(32)  NOT NULL,
    period_start  DATE         NOT NULL,
    first_sent_on TIMESTAMPTZ  NOT NULL DEFAULT now(),
    last_sent_on  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    send_count    INTEGER      NOT NULL DEFAULT 1,
    invites       JSONB        NOT NULL DEFAULT '[]'::jsonb,
    sent_by       VARCHAR(255) NOT NULL,
    PRIMARY KEY (family, kind, period_start),
    CONSTRAINT team_schedule_reminder_sent_family_check CHECK (family IN ('CRE', 'SRE', 'SME')),
    CONSTRAINT team_schedule_reminder_sent_kind_check CHECK (kind ~ '^[a-z_]{1,32}$')
);

-- Rota reminder settings, one row per rota family, saved from the Team
-- Schedule's Rota Access tab by a CSM admin or the family's rota admin and
-- read by csm-scheduled-tasks at the start of every reminder run. A family
-- with no row keeps the task's own configuration.
--
--   weekday_enabled / weekend_enabled  whether those reminders are sent
--   invites_enabled                    whether they carry calendar invitations
--   weekend_required                   {"CRE_WEEKEND": 3, ...} people each
--                                      weekend shift needs on each day
--   windows                            {"CRE_WEEKEND_NIGHT": {"start": "08:00",
--                                      "end": "18:00", "timeZone":
--                                      "America/Los_Angeles"}}: a rotation's
--                                      hours on its own clock
--   copy_to                            addresses copied on every reminder
--
-- id is there for the Team Schedule audit trigger (team_schedule_audit_row),
-- which keys its rows by it, so every change is in team_schedule_audit.
--
-- Additive and safe to re-run: nothing existing is touched.
CREATE TABLE IF NOT EXISTS team_schedule_reminder_setting (
    id               UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    family           VARCHAR(16)  NOT NULL,
    weekday_enabled  BOOLEAN      NOT NULL DEFAULT TRUE,
    weekend_enabled  BOOLEAN      NOT NULL DEFAULT TRUE,
    invites_enabled  BOOLEAN      NOT NULL DEFAULT TRUE,
    weekend_required JSONB        NOT NULL DEFAULT '{}'::jsonb,
    windows          JSONB        NOT NULL DEFAULT '{}'::jsonb,
    copy_to          TEXT[]       NOT NULL DEFAULT '{}',
    created_on       TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_on       TIMESTAMPTZ  NOT NULL DEFAULT now(),
    created_by       VARCHAR(255) NOT NULL,
    updated_by       VARCHAR(255) NOT NULL,
    CONSTRAINT team_schedule_reminder_setting_family_uq UNIQUE (family),
    CONSTRAINT team_schedule_reminder_setting_family_check CHECK (family IN ('CRE', 'SRE', 'SME')),
    CONSTRAINT team_schedule_reminder_setting_required_check CHECK (jsonb_typeof(weekend_required) = 'object'),
    CONSTRAINT team_schedule_reminder_setting_windows_check CHECK (jsonb_typeof(windows) = 'object')
);

COMMENT ON TABLE team_schedule_reminder_setting IS
    'Rota reminder settings per family, set on the Team Schedule Rota Access tab; read by csm-scheduled-tasks.';

DROP TRIGGER IF EXISTS team_schedule_reminder_setting_audit ON team_schedule_reminder_setting;
CREATE TRIGGER team_schedule_reminder_setting_audit
    AFTER INSERT OR UPDATE OR DELETE ON team_schedule_reminder_setting
    FOR EACH ROW EXECUTE FUNCTION team_schedule_audit_row();

-- Rota reminders asked for by hand ("Send now" on the Team Schedule's Rota
-- Access tab), queued for csm-scheduled-tasks, which has no address of its
-- own to call: its rota_manual_reminder job claims the oldest queued request
-- at each run, sends it with the same code as the automatic reminders and
-- reports how it went.
--
--   kind    'weekday' (the next working day's Morning and Evening) or
--           'weekend' (the coming weekend, sent even if not fully marked)
--   status  QUEUED -> SENDING (claimed) -> SENT or FAILED. A SENDING request
--           not reported within 2 hours is claimed again.
--   test    "Send a test to me": sent only to requested_by, marked as a
--           test copy, with test calendar event ids; not recorded, so the
--           real reminders are not affected.
--
-- id keys the Team Schedule audit trigger (team_schedule_audit_row), so who
-- asked and what happened is in team_schedule_audit too.
--
-- Additive and safe to re-run: nothing existing is touched.
CREATE TABLE IF NOT EXISTS team_schedule_reminder_request (
    id           UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    family       VARCHAR(16)  NOT NULL,
    kind         VARCHAR(16)  NOT NULL,
    status       VARCHAR(16)  NOT NULL DEFAULT 'QUEUED',
    test         BOOLEAN      NOT NULL DEFAULT FALSE,
    requested_by VARCHAR(255) NOT NULL,
    requested_on TIMESTAMPTZ  NOT NULL DEFAULT now(),
    claimed_on   TIMESTAMPTZ,
    handled_on   TIMESTAMPTZ,
    result       TEXT,
    created_on   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_on   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    created_by   VARCHAR(255) NOT NULL,
    updated_by   VARCHAR(255) NOT NULL,
    CONSTRAINT team_schedule_reminder_request_family_check CHECK (family IN ('CRE', 'SRE', 'SME')),
    CONSTRAINT team_schedule_reminder_request_kind_check CHECK (kind IN ('weekday', 'weekend')),
    CONSTRAINT team_schedule_reminder_request_status_check CHECK (status IN ('QUEUED', 'SENDING', 'SENT', 'FAILED'))
);

CREATE INDEX IF NOT EXISTS team_schedule_reminder_request_family_on_idx
    ON team_schedule_reminder_request (family, requested_on DESC);

COMMENT ON TABLE team_schedule_reminder_request IS
    'Rota reminders asked for by hand on the Team Schedule Rota Access tab; sent by csm-scheduled-tasks.';

DROP TRIGGER IF EXISTS team_schedule_reminder_request_audit ON team_schedule_reminder_request;
CREATE TRIGGER team_schedule_reminder_request_audit
    AFTER INSERT OR UPDATE OR DELETE ON team_schedule_reminder_request
    FOR EACH ROW EXECUTE FUNCTION team_schedule_audit_row();
