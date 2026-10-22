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
  FormHelperText,
  InputLabel,
  MenuItem,
  Select,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import { useQueries } from "@tanstack/react-query";
import { useMemo, useState, type JSX } from "react";
import { useBackendApi } from "@api/backend/client";
import { useSearchGroups } from "@api/useSearchGroups";
import { useSearchInternalUsersByName } from "@api/useSearchUsersByName";
import AsyncEntitySelect from "@components/AsyncEntitySelect";
import { changeRequestDetailQuery } from "@features/csm-operations/api/useGetChangeRequest";
import { userLabel } from "@features/csm-operations/utils/incidentFormOptions";
import {
  bulkChangeRequestTargets,
  CHANGE_REQUEST_CATEGORY_OPTIONS,
  CHANGE_REQUEST_IMPACT_OPTIONS,
  CHANGE_REQUEST_JOURNAL_MAX,
  CHANGE_REQUEST_PRIORITY_OPTIONS,
  changeRequestApprovedReadOnlyReason,
  changeRequestStateLabel,
  changeRequestTransitionLabel,
  changeRequestTransitionRequiresReason,
  isChangeRequestTerminal,
} from "@features/csm-operations/utils/changeRequests";
import {
  bulkChangeRequestEditProblem,
  type BulkChangeRequestEdit,
} from "@features/csm-operations/utils/bulkChangeRequestEdit";
import type {
  BeChangeRequestCategory,
  BeChangeRequestDetail,
  BeChangeRequestImpact,
  BeChangeRequestPriority,
  BeChangeRequestSearchView,
  BeGroup,
  BeUser,
} from "@api/backend/types";

interface BulkEditChangeRequestsDialogProps {
  /** The selected change requests (none closed, canceled or rolled back: the list does not let those be selected). */
  changeRequests: BeChangeRequestSearchView[];
  isApplying: boolean;
  onClose: () => void;
  /** The one change to apply to every selected change request: only the fields set here. */
  onApply: (edit: BulkChangeRequestEdit) => void;
}

const NO_CHANGE = "";

/**
 * Bulk edit for the change requests selected in the list. Every field left on
 * "No change" is not sent. The state offers only the moves every selected
 * change request can make, read from each one's own `legalNextStates` (so a
 * Close, Cancel change or Roll back is offered when it is legal for all);
 * Roll back and Cancel change need a reason, which is added to each as an
 * internal work note in the same request. Once a change request has been
 * approved its fields are read-only, so for those only the state and the work
 * note are sent (the dialog says how many that is). The backend stays the
 * authority: what it refuses for one change request is reported for that one.
 */
export default function BulkEditChangeRequestsDialog({
  changeRequests,
  isApplying,
  onClose,
  onApply,
}: BulkEditChangeRequestsDialogProps): JSX.Element {
  const api = useBackendApi();
  const [state, setState] = useState(NO_CHANGE);
  const [impact, setImpact] = useState<BeChangeRequestImpact | "">(NO_CHANGE);
  const [priority, setPriority] = useState<BeChangeRequestPriority | "">(NO_CHANGE);
  const [category, setCategory] = useState<BeChangeRequestCategory | "">(NO_CHANGE);
  const [assignedTeamId, setAssignedTeamId] = useState("");
  const [assignedEngineerId, setAssignedEngineerId] = useState("");
  const [workNote, setWorkNote] = useState("");

  // Each change request's own legal moves come with its detail, not the list row.
  const details = useQueries({
    queries: changeRequests.map((cr) => changeRequestDetailQuery(api, cr.id)),
  });
  const detailsLoading = details.some((q) => q.isPending);
  const detailsFailed = details.some((q) => q.isError);
  // Each change request as it is now: its detail once loaded, else the row as it was picked.
  const current = changeRequests.map((cr, i) => details[i]?.data ?? cr);
  // One that has finished since it was picked is skipped by the run, so it neither narrows the moves nor counts.
  const editable = current.filter((cr) => !isChangeRequestTerminal(cr.state));
  const finishedCount = current.length - editable.length;
  const targets =
    detailsLoading || detailsFailed || editable.length === 0
      ? []
      : bulkChangeRequestTargets(editable as BeChangeRequestDetail[]);
  const approvedCount = editable.filter((cr) => !!changeRequestApprovedReadOnlyReason(cr.state)).length;
  const needsReason = !!state && changeRequestTransitionRequiresReason(state);

  const edit = useMemo<BulkChangeRequestEdit>(() => {
    const next: BulkChangeRequestEdit = {};
    if (state) next.state = state;
    if (impact) next.impact = impact;
    if (priority) next.priority = priority;
    if (category) next.category = category;
    if (assignedTeamId) next.assignedTeamId = assignedTeamId;
    if (assignedEngineerId) next.assignedEngineerId = assignedEngineerId;
    if (workNote.trim()) next.workNote = workNote.trim();
    return next;
  }, [state, impact, priority, category, assignedTeamId, assignedEngineerId, workNote]);

  const problem = bulkChangeRequestEditProblem(edit);
  const canApply = Object.keys(edit).length > 0 && !problem && !isApplying;
  const count = changeRequests.length;
  const noun = `change request${count === 1 ? "" : "s"}`;

  const stateHelper = detailsLoading
    ? "Loading the moves each selected change request can make…"
    : detailsFailed
      ? "Couldn't load the selected change requests' moves, so the state can't be changed here. Try again later."
      : targets.length === 0
        ? "The selected change requests have no state they can all move to."
        : "Only the moves every selected change request can make.";

  const select = <V extends string>(
    id: string,
    label: string,
    value: V | "",
    onChange: (v: V | "") => void,
    options: ReadonlyArray<{ value: V; label: string }>,
    opts: { helper?: string; disabled?: boolean } = {},
  ): JSX.Element => (
    <FormControl fullWidth size="small" disabled={isApplying || opts.disabled}>
      <InputLabel id={`${id}-label`} shrink>
        {label}
      </InputLabel>
      <Select
        labelId={`${id}-label`}
        label={label}
        value={value}
        displayEmpty
        onChange={(e) => onChange(String(e.target.value) as V | "")}
      >
        <MenuItem value={NO_CHANGE}>
          <Typography component="span" color="text.secondary">
            No change
          </Typography>
        </MenuItem>
        {options.map((o) => (
          <MenuItem key={o.value} value={o.value}>
            {o.label}
          </MenuItem>
        ))}
      </Select>
      {opts.helper && <FormHelperText>{opts.helper}</FormHelperText>}
    </FormControl>
  );

  const fieldsDisabled = editable.length > 0 && approvedCount === editable.length;
  const fieldsHelper = fieldsDisabled
    ? "All the selected change requests have been approved, so their fields are read-only."
    : undefined;

  return (
    <Dialog open onClose={isApplying ? undefined : onClose} maxWidth="sm" fullWidth>
      <DialogTitle>
        Edit {count} {noun}
      </DialogTitle>
      <DialogContent dividers sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
        <Typography variant="body2" color="text.secondary">
          Only the fields you change are applied; the rest keep each change request's own value.
        </Typography>
        {finishedCount > 0 && (
          <Alert severity="warning">
            {finishedCount} of the selected change requests {finishedCount === 1 ? "has" : "have"} been closed,
            canceled or rolled back since {finishedCount === 1 ? "it was" : "they were"} selected and will be skipped.
          </Alert>
        )}
        {approvedCount > 0 && approvedCount < editable.length && (
          <Alert severity="info">
            {approvedCount} of the selected change requests {approvedCount === 1 ? "has" : "have"} been approved: only the
            state and the work note apply to {approvedCount === 1 ? "it" : "them"}, since an approved change request's
            fields are read-only.
          </Alert>
        )}
        {select(
          "bulk-cr-state",
          "State",
          state,
          setState,
          targets.map((t) => ({ value: t, label: `${changeRequestTransitionLabel(t)} (${changeRequestStateLabel(t)})` })),
          { helper: stateHelper, disabled: detailsLoading || detailsFailed || targets.length === 0 },
        )}
        <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr 1fr" }, gap: 2 }}>
          {select("bulk-cr-impact", "Impact", impact, setImpact, CHANGE_REQUEST_IMPACT_OPTIONS, { disabled: fieldsDisabled })}
          {select("bulk-cr-priority", "Priority", priority, setPriority, CHANGE_REQUEST_PRIORITY_OPTIONS, {
            disabled: fieldsDisabled,
          })}
          {select("bulk-cr-category", "Category", category, setCategory, CHANGE_REQUEST_CATEGORY_OPTIONS, {
            disabled: fieldsDisabled,
          })}
        </Box>
        <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" }, gap: 2 }}>
          <AsyncEntitySelect<BeGroup>
            id="bulk-cr-assigned-team"
            label="Assignment group"
            placeholder="No change"
            value={assignedTeamId}
            onChange={setAssignedTeamId}
            disabled={isApplying || fieldsDisabled}
            useSearch={useSearchGroups}
            getId={(g) => g.id}
            getLabel={(g) => g.name}
          />
          <AsyncEntitySelect<BeUser>
            id="bulk-cr-assigned-engineer"
            label="Assigned to"
            placeholder="No change"
            value={assignedEngineerId}
            onChange={setAssignedEngineerId}
            disabled={isApplying || fieldsDisabled}
            useSearch={useSearchInternalUsersByName}
            getId={(u) => u.id!}
            getLabel={userLabel}
          />
        </Box>
        {fieldsHelper && (
          <Typography variant="caption" color="text.secondary">
            {fieldsHelper}
          </Typography>
        )}
        <TextField
          id="bulk-cr-work-note"
          label={needsReason ? "Reason (internal work note)" : "Work note (internal)"}
          required={needsReason}
          multiline
          minRows={2}
          size="small"
          value={workNote}
          onChange={(e) => setWorkNote(e.target.value.slice(0, CHANGE_REQUEST_JOURNAL_MAX))}
          disabled={isApplying}
          helperText={
            needsReason
              ? `Required to ${changeRequestTransitionLabel(state).toLowerCase()}: added to every selected change request as an internal work note, never shown to the customer.`
              : "Added to every selected change request; never shown to the customer."
          }
        />
        {problem && (
          <Alert severity="info" role="status">
            {problem}
          </Alert>
        )}
      </DialogContent>
      <DialogActions>
        <Button color="inherit" onClick={onClose} disabled={isApplying}>
          Cancel
        </Button>
        <Button variant="contained" onClick={() => onApply(edit)} disabled={!canApply}>
          {isApplying ? "Applying…" : `Apply to ${count} ${noun}`}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
