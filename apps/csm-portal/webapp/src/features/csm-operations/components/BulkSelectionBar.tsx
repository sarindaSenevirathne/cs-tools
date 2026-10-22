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

import { Box, Button, CircularProgress, Tooltip, Typography } from "@wso2/oxygen-ui";
import { Pencil, UserCheck, X } from "@wso2/oxygen-ui-icons-react";
import type { JSX } from "react";

interface BulkSelectionBarProps {
  count: number;
  /** What a selected row is called, e.g. "incident" / "change request" (singular). */
  noun: string;
  onEdit: () => void;
  onAssignToMe: () => void;
  onClear: () => void;
  /** True while a bulk update runs: every action waits for it. */
  busy?: boolean;
  /** Why Assign to me is unavailable (e.g. the signed-in user is not known yet), or null. */
  assignToMeDisabledReason?: string | null;
}

/** Read by screen readers, not shown. Pixel strings on purpose: in `sx` a bare `1` means 100%. */
const visuallyHidden = {
  position: "absolute",
  width: "1px",
  height: "1px",
  m: "-1px",
  p: 0,
  overflow: "hidden",
  clip: "rect(0 0 0 0)",
  whiteSpace: "nowrap",
  border: 0,
} as const;

/**
 * The actions for the rows selected in a list: Edit (n), Assign to me (n) and
 * Clear selection, shown only while something is selected. The count is also
 * written to a live region that stays mounted when nothing is selected, so a
 * screen reader hears the first selection as well as every later change (a
 * region inserted with its text already in it is not reliably announced).
 */
export default function BulkSelectionBar({
  count,
  noun,
  onEdit,
  onAssignToMe,
  onClear,
  busy = false,
  assignToMeDisabledReason = null,
}: BulkSelectionBarProps): JSX.Element {
  const summary = `${count} ${noun}${count === 1 ? "" : "s"} selected`;
  const assign = (
    <Button
      size="small"
      variant="outlined"
      startIcon={busy ? <CircularProgress size={14} /> : <UserCheck size={14} />}
      onClick={onAssignToMe}
      disabled={busy || !!assignToMeDisabledReason}
    >
      Assign to me ({count})
    </Button>
  );
  return (
    <>
      <Box component="span" role="status" aria-live="polite" sx={visuallyHidden}>
        {count > 0 ? summary : ""}
      </Box>
      {count > 0 && (
        <Box
          role="region"
          aria-label="Selected rows"
          sx={{
            display: "flex",
            alignItems: "center",
            gap: 1,
            flexWrap: "wrap",
            px: 1.5,
            py: 1,
            border: 1,
            borderColor: "primary.main",
            borderRadius: 1,
            bgcolor: "action.selected",
          }}
        >
          <Typography variant="body2" sx={{ mr: 1, fontWeight: 600 }} aria-hidden>
            {summary}
          </Typography>
          <Button size="small" variant="contained" startIcon={<Pencil size={14} />} onClick={onEdit} disabled={busy}>
            Edit ({count})
          </Button>
          {assignToMeDisabledReason ? (
            <Tooltip title={assignToMeDisabledReason}>
              <span>{assign}</span>
            </Tooltip>
          ) : (
            assign
          )}
          <Button size="small" variant="text" color="inherit" startIcon={<X size={14} />} onClick={onClear} disabled={busy}>
            Clear selection
          </Button>
        </Box>
      )}
    </>
  );
}
