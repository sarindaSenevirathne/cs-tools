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

import { type JSX, useState } from "react";
import {
  Button,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  IconButton,
  TextField,
} from "@wso2/oxygen-ui";
import { X } from "@wso2/oxygen-ui-icons-react";

export interface AddToVerificationDialogProps {
  open: boolean;
  isPending: boolean;
  onClose: () => void;
  onConfirm: (note?: string) => void;
}

/**
 * Confirmation dialog for manually adding a closed case to the pending
 * verification list, with an optional free-text note.
 *
 * @param {AddToVerificationDialogProps} props - Open/pending state and callbacks.
 * @returns {JSX.Element} The dialog.
 */
export default function AddToVerificationDialog({
  open,
  isPending,
  onClose,
  onConfirm,
}: AddToVerificationDialogProps): JSX.Element {
  const [note, setNote] = useState("");

  const handleClose = (): void => {
    if (isPending) return;
    setNote("");
    onClose();
  };

  return (
    <Dialog open={open} onClose={isPending ? undefined : handleClose} maxWidth="xs" fullWidth>
      <DialogTitle
        sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}
      >
        Add to Verification
        <IconButton size="small" onClick={handleClose} aria-label="close" disabled={isPending}>
          <X size={18} />
        </IconButton>
      </DialogTitle>
      <DialogContent>
        <DialogContentText sx={{ mb: 2 }}>
          Add this case to the verification list so it can be reviewed and signed off.
        </DialogContentText>
        <TextField
          label="Note (optional)"
          placeholder="Add any context for the verification history"
          fullWidth
          multiline
          minRows={3}
          value={note}
          onChange={(e) => setNote(e.target.value)}
          disabled={isPending}
        />
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose} disabled={isPending}>
          Cancel
        </Button>
        <Button
          variant="contained"
          onClick={() => onConfirm(note.trim() || undefined)}
          disabled={isPending}
          startIcon={
            isPending ? <CircularProgress size={16} color="inherit" /> : undefined
          }
        >
          {isPending ? "Adding…" : "Add to Verification"}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
