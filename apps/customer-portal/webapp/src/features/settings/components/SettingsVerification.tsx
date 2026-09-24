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
// software distributed under the License is distributed on
// an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

import {
  Box,
  Chip,
  FormControlLabel,
  Paper,
  Switch,
  Typography,
  alpha,
  colors,
} from "@wso2/oxygen-ui";
import { ShieldCheck } from "@wso2/oxygen-ui-icons-react";
import { useCallback, useState, useMemo, type JSX } from "react";
import useGetProjectDetails from "@api/useGetProjectDetails";
import { usePatchProject } from "@features/settings/api/usePatchProject";
import { useDarkMode } from "@utils/useDarkMode";
import { useErrorBanner } from "@context/error-banner/ErrorBannerContext";
import { useSuccessBanner } from "@context/success-banner/SuccessBannerContext";
import {
  SETTINGS_VERIFICATION_ADMIN_ONLY_HINT,
  SETTINGS_VERIFICATION_HEADER_BODY,
  SETTINGS_VERIFICATION_HEADER_TITLE,
  SETTINGS_VERIFICATION_PATCH_ERROR,
  SETTINGS_VERIFICATION_SECTION_TITLE,
  SETTINGS_VERIFICATION_SUCCESS_MESSAGE,
  SETTINGS_VERIFICATION_TOGGLE_DESCRIPTION,
  SETTINGS_VERIFICATION_TOGGLE_LABEL,
} from "@features/settings/constants/settingsConstants";
import type { SettingsVerificationProps } from "@features/settings/types/settings";

/**
 * Pending Verification settings: gates the feature on/off for the project.
 *
 * Deliberately does not reload the page on a successful patch, unlike
 * SettingsAiAssistant — there is no SDK re-init need here, so it relies on
 * usePatchProject's own query invalidation plus an in-place success banner.
 *
 * @param {SettingsVerificationProps} props - Component props.
 * @returns {JSX.Element} The component.
 */
export default function SettingsVerification({
  projectId,
  canEdit = true,
}: SettingsVerificationProps): JSX.Element {
  const { showError } = useErrorBanner();
  const { showSuccess } = useSuccessBanner();
  const { data: projectDetails, isLoading: isProjectDetailsLoading } =
    useGetProjectDetails(projectId);
  const patchProject = usePatchProject(projectId);
  const [verificationOverride, setVerificationOverride] = useState<
    boolean | null
  >(null);

  const projectVerificationEnabled = useMemo(
    () => projectDetails?.verificationEnabled,
    [projectDetails],
  );

  const verificationEnabled =
    verificationOverride ?? projectVerificationEnabled ?? false;

  const handleVerificationToggle = useCallback(
    (checked: boolean) => {
      setVerificationOverride(checked);
      patchProject.mutate(
        { verificationEnabled: checked },
        {
          onSuccess: () => {
            showSuccess(SETTINGS_VERIFICATION_SUCCESS_MESSAGE);
          },
          onError: (err) => {
            showError(
              err instanceof Error ? err.message : SETTINGS_VERIFICATION_PATCH_ERROR,
            );
            setVerificationOverride(null);
          },
        },
      );
    },
    [patchProject, showSuccess, showError],
  );

  const isDarkMode = useDarkMode();

  const disabledForSwitches =
    !canEdit || isProjectDetailsLoading || patchProject.isPending;

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 3 }}>
      <Paper
        sx={{
          p: 2.5,
          bgcolor: alpha(colors.blue[500], 0.06),
          border: "1px solid",
          borderColor: alpha(colors.blue[500], 0.2),
        }}
      >
        <Box sx={{ display: "flex", alignItems: "flex-start", gap: 2 }}>
          <Paper
            sx={{
              width: 40,
              height: 40,
              bgcolor: alpha(colors.blue[500], 0.12),
              color: colors.blue[700],
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              flexShrink: 0,
              border: "none",
            }}
          >
            <ShieldCheck size={20} color={colors.blue[600]} />
          </Paper>
          <Box sx={{ flex: 1 }}>
            <Typography variant="h6" color="text.primary" sx={{ mb: 0.5 }}>
              {SETTINGS_VERIFICATION_HEADER_TITLE}
            </Typography>
            <Typography variant="body2" color="text.secondary">
              {SETTINGS_VERIFICATION_HEADER_BODY}
            </Typography>
          </Box>
        </Box>
      </Paper>

      <Box>
        <Box
          sx={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            mb: 2,
            gap: 2,
          }}
        >
          <Typography variant="h6" color="text.primary">
            {SETTINGS_VERIFICATION_SECTION_TITLE}
          </Typography>
          {!canEdit && (
            <Typography
              variant="caption"
              color="text.secondary"
              sx={{ fontStyle: "italic", textAlign: "right", flexShrink: 0 }}
            >
              {SETTINGS_VERIFICATION_ADMIN_ONLY_HINT}
            </Typography>
          )}
        </Box>
        <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
          <Paper
            sx={{
              p: 2.5,
              ...(!canEdit && {
                bgcolor: isDarkMode
                  ? alpha(colors.grey[600], 0.6)
                  : alpha(colors.grey[300], 0.0),
                border: "1px solid",
                borderColor: isDarkMode
                  ? alpha(colors.grey[600], 0.3)
                  : alpha(colors.grey[400], 0.3),
                opacity: 0.8,
              }),
            }}
          >
            <Box
              sx={{
                display: "flex",
                alignItems: "flex-start",
                justifyContent: "space-between",
                gap: 2,
              }}
            >
              <Box sx={{ flex: 1 }}>
                <Box
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    gap: 1.5,
                    mb: 1,
                  }}
                >
                  <ShieldCheck
                    size={20}
                    color={
                      !canEdit
                        ? isDarkMode
                          ? colors.grey[600]
                          : colors.grey[400]
                        : colors.orange[600]
                    }
                  />
                  <Typography
                    variant="body1"
                    id="pending-verification-label"
                    color={!canEdit ? "text.disabled" : "text.primary"}
                  >
                    {SETTINGS_VERIFICATION_TOGGLE_LABEL}
                  </Typography>
                  <Chip
                    label={verificationEnabled ? "Active" : "Inactive"}
                    size="small"
                    color={!canEdit ? "default" : verificationEnabled ? "success" : "default"}
                    variant="outlined"
                    sx={{
                      height: 20,
                      fontSize: "0.7rem",
                      ...(!canEdit && {
                        color: "text.disabled",
                        borderColor: isDarkMode
                          ? alpha(colors.grey[600], 0.5)
                          : "action.disabled",
                      }),
                    }}
                  />
                </Box>
                <Typography variant="body2" color={!canEdit ? "text.disabled" : "text.secondary"}>
                  {SETTINGS_VERIFICATION_TOGGLE_DESCRIPTION}
                </Typography>
              </Box>
              <FormControlLabel
                control={
                  <Switch
                    checked={verificationEnabled}
                    disabled={disabledForSwitches}
                    onChange={(_, checked) => handleVerificationToggle(checked)}
                    color="warning"
                    inputProps={{
                      "aria-labelledby": "pending-verification-label",
                    }}
                    sx={{
                      ...(!canEdit && {
                        "& .MuiSwitch-track": {
                          bgcolor: isDarkMode
                            ? alpha(colors.grey[700], 0.8)
                            : alpha(colors.grey[400], 0.35),
                          opacity: 1,
                        },
                        "& .MuiSwitch-thumb": {
                          bgcolor: isDarkMode
                            ? colors.grey[600]
                            : colors.grey[300],
                        },
                      }),
                    }}
                  />
                }
                label=""
              />
            </Box>
          </Paper>
        </Box>
      </Box>
    </Box>
  );
}
