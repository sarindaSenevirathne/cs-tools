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

-- pending_verification has no project_id of its own -- it reaches one only
-- via work_item_id, exactly the same shape as case_attachment/work_item_tag/
-- work_item_watcher/work_item_activity (migration 0147_case_adjacent_rls.sql),
-- all of which already got this treatment. pending_verification was the one
-- table of this exact shape that never did -- a gap this migration closes,
-- not a new design.
--
-- Before this, none of POST /pending-verifications, POST /pending-verifications/
-- search, or PATCH /pending-verifications/{id} had any project-membership
-- enforcement of their own: Search was only ever incidentally safe because
-- its queries happen to join work_item (itself RLS-protected since 0147);
-- Verify's bare `UPDATE pending_verification ... WHERE id = $1` never joined
-- work_item at all, so RLS never entered into it; Create's only check was the
-- work_item_id foreign key, and Postgres foreign-key enforcement always
-- bypasses RLS by design (it would otherwise leak the existence of rows the
-- caller can't see). Confirmed live: a bare UPDATE against this table with no
-- caller identity set returned every row regardless of project.
--
-- No Go changes are needed for Search/Verify: pending_verification_repo.go
-- already runs on *Scoped (converted when this lint test was first added --
-- see TestRLSBypassLint_NoRawPoolAgainstAProtectedTable), which stamps
-- app.is_internal/app.viewer_email/app.viewer_project_ids on every query it
-- makes, regardless of which table. These policies start applying the moment
-- this migration runs, with zero code deployment required alongside it.
-- Create's own error handling does need one addition -- see the matching
-- change in pending_verification_repo.go's Create for why (a WITH CHECK
-- violation is a distinct error shape from the existing foreign-key check).
ALTER TABLE pending_verification ENABLE ROW LEVEL SECURITY;
ALTER TABLE pending_verification FORCE ROW LEVEL SECURITY;

CREATE POLICY pending_verification_visibility ON pending_verification
  FOR SELECT
  USING (
    current_setting('app.is_internal', true) = 'true'
    OR is_project_member((SELECT wi.project_id FROM work_item wi WHERE wi.id = pending_verification.work_item_id))
  );

-- CreatePendingVerification is genuinely customer-facing (manual add), same
-- reasoning as work_item_write/case_write in 0147 -- not internal-only.
CREATE POLICY pending_verification_write ON pending_verification
  FOR INSERT WITH CHECK (
    current_setting('app.is_internal', true) = 'true'
    OR is_project_member((SELECT wi.project_id FROM work_item wi WHERE wi.id = work_item_id))
  );

-- VerifyPendingVerification ("Mark Verified") is also customer-facing --
-- USING gates which existing row the caller may target, WITH CHECK gates the
-- row's own work_item_id (unchanged by this UPDATE, but repeated per
-- work_item_update's own precedent in 0147: USING's unqualified work_item_id
-- would otherwise resolve to the OLD row and WITH CHECK's to the NEW one).
CREATE POLICY pending_verification_update ON pending_verification
  FOR UPDATE USING (
    current_setting('app.is_internal', true) = 'true'
    OR is_project_member((SELECT wi.project_id FROM work_item wi WHERE wi.id = pending_verification.work_item_id))
  )
  WITH CHECK (
    current_setting('app.is_internal', true) = 'true'
    OR is_project_member((SELECT wi.project_id FROM work_item wi WHERE wi.id = work_item_id))
  );

-- Internal-only, not omitted entirely: nothing in this codebase deletes a
-- pending_verification row today, but FORCE plus zero DELETE policy would
-- also block legitimate internal/admin/test cleanup -- same sla_delete-style
-- reasoning used throughout this migration series (e.g. 0147's own
-- work_item_delete_internal_only).
CREATE POLICY pending_verification_delete_internal_only ON pending_verification
  FOR DELETE USING (current_setting('app.is_internal', true) = 'true');
