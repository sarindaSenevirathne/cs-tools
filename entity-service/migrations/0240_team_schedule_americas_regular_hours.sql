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

-- Team Schedule: the Americas team's ordinary weekday on its own clock.
--
-- The Americas team works 08:00-18:00 PDT. Its ordinary day was Americas
-- cover (CRE_AMERICAS), 21:00-06:00 on the Colombo clock, which starts as the
-- Sri Lankan day (06:00-21:00 IST) ends, so the half hour either side where
-- both are at work could not be shown.
--
-- So a window of its own: 08:00-18:00 on a fixed UTC-7 clock (Etc/GMT+7, the
-- PDT offset, which keeps no daylight saving), i.e. 20:30-06:30 IST on every
-- day of the year, a 30-minute overlap with the Sri Lankan day at each end.
-- Fixed on purpose for now (decided 2026-10-18): authored in
-- America/Los_Angeles the window would move an hour when the US clocks go
-- back, and leave a 30-minute gap at 21:00 IST all winter. It is standing
-- hours (is_rotation FALSE), like Regular hours, and becomes the Americas
-- team's ordinary day in place of Americas cover.
--
-- Americas cover itself is left as it is. The ABT roster import files the
-- sheet's NLK cells on it, rows already stored point at it, and Case Alerts
-- takes the shift of an incident from the Colombo clock, not from this table.
-- Display only: the team's ordinary day is drawn by the webapp and never
-- stored, so Case Alerts, which reads stored rows, sees no change.
--
-- Safe to re-run: the window is added once, and the default moves only where
-- it is still Americas cover.
INSERT INTO team_schedule_shift
    (code, label, family, zone_id, tier, day_scope, start_minute, end_minute, authoring_time_zone,
     is_on_call, is_escalation, is_rotation, required_headcount,
     short_code, colour_token, sort_order, created_by, updated_by)
VALUES
    ('CRE_AMERICAS_PT', 'Americas regular hours', 'CRE', NULL, NULL, 'WEEKDAY', 480, 1080, 'Etc/GMT+7',
     FALSE, FALSE, FALSE, NULL,
     'NLK', 'NLK', 55, 'migration', 'migration')
ON CONFLICT (code) DO NOTHING;

UPDATE team_schedule_team_default_shift
   SET shift_code = 'CRE_AMERICAS_PT'
 WHERE shift_code = 'CRE_AMERICAS'
   AND EXISTS (SELECT 1 FROM team_schedule_shift s WHERE s.code = 'CRE_AMERICAS_PT');
