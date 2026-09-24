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

import { Chip, alpha, useTheme } from "@wso2/oxygen-ui";
import { CircleCheck, ShieldCheck } from "@wso2/oxygen-ui-icons-react";
import type { JSX } from "react";

export interface VerificationStatusChipProps {
  isPendingVerification?: boolean;
  isVerified?: boolean;
}

/**
 * "Pending Verification"/"Verified" status chip — extracted from
 * CaseDetailsHeader so Announcement and Change Request's own bespoke headers
 * can show the same chip without duplicating its styling. Renders nothing
 * when neither flag is set.
 *
 * @param {VerificationStatusChipProps} props - Which state (if any) to show.
 * @returns {JSX.Element | null} The chip, or null.
 */
export default function VerificationStatusChip({
  isPendingVerification = false,
  isVerified = false,
}: VerificationStatusChipProps): JSX.Element | null {
  const theme = useTheme();

  if (isPendingVerification) {
    return (
      <Chip
        icon={<ShieldCheck size={11} />}
        label="Pending Verification"
        size="small"
        variant="outlined"
        sx={{
          color: theme.palette.warning.dark,
          borderColor: alpha(theme.palette.warning.main, 0.5),
          bgcolor: alpha(theme.palette.warning.light, 0.12),
          fontWeight: 500,
          height: 20,
          fontSize: "0.7rem",
          "& .MuiChip-icon": { color: theme.palette.warning.main, ml: "4px", mr: "2px" },
          "& .MuiChip-label": { pl: "4px", pr: "8px" },
        }}
      />
    );
  }

  if (isVerified) {
    return (
      <Chip
        icon={<CircleCheck size={11} />}
        label="Verified"
        size="small"
        variant="outlined"
        sx={{
          color: theme.palette.success.dark,
          borderColor: alpha(theme.palette.success.main, 0.5),
          bgcolor: alpha(theme.palette.success.light, 0.12),
          fontWeight: 500,
          height: 20,
          fontSize: "0.7rem",
          "& .MuiChip-icon": { color: theme.palette.success.main, ml: "4px", mr: "2px" },
          "& .MuiChip-label": { pl: "4px", pr: "8px" },
        }}
      />
    );
  }

  return null;
}
