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

import { Button, Stack, useTheme, alpha, type Theme } from "@wso2/oxygen-ui";
import { CircleCheck, ShieldCheck } from "@wso2/oxygen-ui-icons-react";
import { type JSX, useState } from "react";
import { useCreatePendingVerification } from "@features/support/api/useCreatePendingVerification";
import { useVerifyPendingVerification } from "@features/support/api/useVerifyPendingVerification";
import { useErrorBanner } from "@context/error-banner/ErrorBannerContext";
import { useSuccessBanner } from "@context/success-banner/SuccessBannerContext";
import AddToVerificationDialog from "@features/support/components/case-details/verifications-tab/AddToVerificationDialog";
import CaseStateConfirmDialog from "@features/support/components/case-details/dialogs/CaseStateConfirmDialog";

const ACTION_BUTTON_ICON_SIZE = 12;

export interface VerificationActionButtonsProps {
  projectId: string;
  workItemId: string;
  /** True once the underlying record is closed — "Add to Verification" only ever applies to a closed record. */
  isRecordClosed?: boolean;
  hasNoVerificationRecord?: boolean;
  isPendingVerification?: boolean;
  pendingVerificationId?: string | null;
  onAddToVerificationSuccess?: () => void;
  onMarkVerifiedSuccess?: () => void;
}

function getActionButtonSx(
  theme: Theme,
  intent: "info" | "success",
): Record<string, unknown> {
  const light = theme.palette[intent].light;
  return {
    borderColor: light,
    bgcolor: alpha(light, 0.1),
    color: light,
    fontSize: "0.7rem",
    minHeight: 0,
    py: 0.5,
    px: 1,
    "&:hover": {
      borderColor: theme.palette[intent].main,
      bgcolor: alpha(light, 0.2),
    },
    textTransform: "none",
  };
}

/**
 * "Add to Verification"/"Mark Verified" buttons and their confirm dialogs —
 * extracted from CaseDetailsActionRow so record types with no use for that
 * component's other case-status/escalation machinery (e.g. Change Request)
 * can render just the verification actions on their own.
 *
 * @param {VerificationActionButtonsProps} props - Record identity and verification state.
 * @returns {JSX.Element} The action buttons and their dialogs (renders nothing visible when neither action applies).
 */
export default function VerificationActionButtons({
  projectId,
  workItemId,
  isRecordClosed = false,
  hasNoVerificationRecord = false,
  isPendingVerification = false,
  pendingVerificationId,
  onAddToVerificationSuccess,
  onMarkVerifiedSuccess,
}: VerificationActionButtonsProps): JSX.Element {
  const theme = useTheme();
  const { showSuccess } = useSuccessBanner();
  const { showError } = useErrorBanner();

  const createPendingVerification = useCreatePendingVerification(projectId, workItemId);
  const verifyPendingVerification = useVerifyPendingVerification(projectId, workItemId);
  const [addToVerificationOpen, setAddToVerificationOpen] = useState(false);
  const [markVerifiedOpen, setMarkVerifiedOpen] = useState(false);

  const showAddToVerificationButton = isRecordClosed && hasNoVerificationRecord;
  const showMarkVerifiedButton = isPendingVerification && !!pendingVerificationId;

  const handleAddToVerificationConfirm = (note?: string): void => {
    createPendingVerification.mutate(
      { workItemId, addedReason: "manual", note },
      {
        onSuccess: () => {
          showSuccess("Record added to verification list.");
          onAddToVerificationSuccess?.();
        },
        onError: (err) => {
          showError(err?.message ?? "Failed to add record to verification list.");
        },
        onSettled: () => setAddToVerificationOpen(false),
      },
    );
  };

  const handleMarkVerifiedConfirm = (): void => {
    if (!pendingVerificationId) return;
    verifyPendingVerification.mutate(pendingVerificationId, {
      onSuccess: () => {
        showSuccess("Record marked as verified.");
        onMarkVerifiedSuccess?.();
      },
      onError: (err) => {
        showError(err?.message ?? "Failed to mark record as verified.");
      },
      onSettled: () => setMarkVerifiedOpen(false),
    });
  };

  return (
    <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap">
      {showAddToVerificationButton && (
        <Button
          variant="outlined"
          size="small"
          startIcon={<ShieldCheck size={ACTION_BUTTON_ICON_SIZE} />}
          onClick={() => setAddToVerificationOpen(true)}
          sx={getActionButtonSx(theme, "info")}
        >
          Add to Verification
        </Button>
      )}
      {showMarkVerifiedButton && (
        <Button
          variant="outlined"
          size="small"
          startIcon={<CircleCheck size={ACTION_BUTTON_ICON_SIZE} />}
          onClick={() => setMarkVerifiedOpen(true)}
          sx={getActionButtonSx(theme, "success")}
        >
          Mark Verified
        </Button>
      )}
      <AddToVerificationDialog
        open={addToVerificationOpen}
        isPending={createPendingVerification.isPending}
        onClose={() => setAddToVerificationOpen(false)}
        onConfirm={handleAddToVerificationConfirm}
      />
      <CaseStateConfirmDialog
        open={markVerifiedOpen}
        title="Mark as Verified"
        actionLabel="mark as verified"
        isPending={verifyPendingVerification.isPending}
        onClose={() => setMarkVerifiedOpen(false)}
        onConfirm={handleMarkVerifiedConfirm}
      />
    </Stack>
  );
}
