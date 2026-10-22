// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

import {
  Alert,
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControl,
  InputLabel,
  MenuItem,
  Select,
  Typography,
} from "@wso2/oxygen-ui";
import { useMemo, useState, type JSX } from "react";
import AsyncEntitySelect from "@components/AsyncEntitySelect";
import {
  PROJECT_ONBOARDING_STATUSES,
  type ProjectOnboardingPatch,
  type ProjectOnboardingStatusValue,
} from "@features/csm-projects/api/usePatchProjectOnboarding";
import {
  useSearchInternalUsers,
  type InternalUserOption,
} from "@features/csm-projects/api/useSearchInternalUsers";

const ONBOARDING_STATUS_LABELS: Record<ProjectOnboardingStatusValue, string> = {
  "Not-Started": "Not Started",
  "In-Progress": "In Progress",
  "On-Hold": "On Hold",
  Completed: "Completed",
  Cancelled: "Cancelled",
  Expired: "Expired",
  "Not-Applicable": "Not Applicable",
};

/** The backend matches ignoring case, hyphens, underscores and spaces, and an
 *  older source spells On Hold `OnHold`; normalize the same way so the picker
 *  opens on the project's real current value. `""` when absent or unknown. */
function matchCurrentStatus(current?: string | null): ProjectOnboardingStatusValue | "" {
  const norm = (v: string): string => v.toLowerCase().replace(/[-_\s]/g, "");
  const target = current ? norm(current) : "";
  return PROJECT_ONBOARDING_STATUSES.find((s) => norm(s) === target) ?? "";
}

interface EditProjectOnboardingDialogProps {
  /** The project's current `onboardingStatus`, if any. */
  currentStatus?: string | null;
  /** The project's current onboarding owner, if any. */
  currentOwner?: { id: string; name: string } | null;
  /** True while the PATCH is in flight; disables the pickers/actions. */
  isSaving: boolean;
  /** User-facing message for the most recent failed save, if any. */
  saveError?: string | null;
  onClose: () => void;
  /** Submit only the changed field(s) (`PATCH /projects/{id}`). */
  onSave: (patch: ProjectOnboardingPatch) => void;
}

/**
 * Edit a project's onboarding status and onboarding owner independently --
 * either may be changed on its own. The owner picker lists internal, active
 * users only (see `useSearchInternalUsers`). There is no way to clear the
 * owner back to "none": the backend does not support it, so an emptied owner
 * is never submitted. Mirrors `EditProjectClosureStateDialog`/
 * `EditAccountTeamsDialog`: a plain controlled dialog whose caller owns the
 * mutation and passes `isSaving`/`saveError` down.
 */
export default function EditProjectOnboardingDialog({
  currentStatus,
  currentOwner,
  isSaving,
  saveError,
  onClose,
  onSave,
}: EditProjectOnboardingDialogProps): JSX.Element {
  const initialStatus = matchCurrentStatus(currentStatus);
  const initialOwnerId = currentOwner?.id ?? "";
  const [status, setStatus] = useState<ProjectOnboardingStatusValue | "">(initialStatus);
  const [ownerId, setOwnerId] = useState(initialOwnerId);

  const patch = useMemo<ProjectOnboardingPatch>(() => {
    const next: ProjectOnboardingPatch = {};
    if (status && status !== initialStatus) next.onboardingStatus = status;
    if (ownerId && ownerId !== initialOwnerId) next.onboardingOwner = ownerId;
    return next;
  }, [status, initialStatus, ownerId, initialOwnerId]);

  const hasChanges = Object.keys(patch).length > 0;

  return (
    <Dialog open onClose={onClose} maxWidth="xs" fullWidth>
      <DialogTitle>Edit onboarding</DialogTitle>
      <DialogContent dividers>
        <Box sx={{ display: "flex", flexDirection: "column", gap: 2, pt: 0.5 }}>
          {saveError && <Alert severity="error">{saveError}</Alert>}

          <FormControl size="small" fullWidth>
            <InputLabel
              id="edit-project-onboarding-status-label"
              shrink={status !== ""}
              sx={{ top: "0px !important" }}
            >
              Onboarding status
            </InputLabel>
            <Select
              labelId="edit-project-onboarding-status-label"
              label="Onboarding status"
              value={status}
              onChange={(e) => setStatus(e.target.value as ProjectOnboardingStatusValue)}
              disabled={isSaving}
              notched={status !== ""}
            >
              {PROJECT_ONBOARDING_STATUSES.map((s) => (
                <MenuItem key={s} value={s}>
                  {ONBOARDING_STATUS_LABELS[s]}
                </MenuItem>
              ))}
            </Select>
          </FormControl>

          <AsyncEntitySelect<InternalUserOption>
            id="edit-project-onboarding-owner"
            label="Onboarding owner"
            placeholder="Search internal users…"
            value={ownerId}
            onChange={setOwnerId}
            disabled={isSaving}
            useSearch={useSearchInternalUsers}
            getId={(u) => u.id}
            getLabel={(u) => u.name}
            knownLabel={currentOwner?.name}
            hideClearButton
          />

          <Typography variant="caption" color="text.secondary">
            Only internal users can be the owner. Clearing the owner back to
            "no owner" isn't supported, so an emptied field won't be saved.
          </Typography>
        </Box>
      </DialogContent>
      <DialogActions>
        <Button color="inherit" onClick={onClose} disabled={isSaving}>
          Cancel
        </Button>
        <Button
          variant="contained"
          onClick={() => onSave(patch)}
          disabled={isSaving || !hasChanges}
        >
          {isSaving ? "Saving…" : "Save"}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
