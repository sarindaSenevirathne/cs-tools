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
  Avatar,
  Box,
  Chip,
  CircularProgress,
  Stack,
  Typography,
  alpha,
  useTheme,
} from "@wso2/oxygen-ui";
import { Clock, ShieldCheck } from "@wso2/oxygen-ui-icons-react";
import { type JSX } from "react";
import { usePendingVerificationsSearch } from "@features/support/api/usePendingVerificationsSearch";
import useGetUserDetails from "@features/settings/api/useGetUserDetails";
import useGetProjectContacts from "@features/settings/api/useGetProjectContacts";
import { formatRelativeTime } from "@features/support/utils/support";
import type { PendingVerificationView } from "@features/support/types/pendingVerification";
import type { ProjectContact } from "@features/settings/types/users";

type Props = {
  projectId: string;
  caseId: string;
};

function formatDate(iso?: string | null): string {
  if (!iso) return "—";
  return new Date(iso).toLocaleString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

interface ActorDisplay {
  name: string;
  role?: string;
  initials: string;
}

// Resolves a bare email into a display name/role by cross-referencing the
// project's own contacts (the same data SettingsUserManagement/escalation
// lead-checks already use) — "You" when it's the current user, a role label
// only when one of the contact's own role booleans is set (never guessed).
function resolveActorDisplay(
  email: string,
  contacts: ProjectContact[] | undefined,
  currentUserEmail: string,
): ActorDisplay {
  const isSelf = !!currentUserEmail && email.toLowerCase() === currentUserEmail.toLowerCase();
  const contact = contacts?.find((c) => c.email.toLowerCase() === email.toLowerCase());
  const fullName = contact ? [contact.firstName, contact.lastName].filter(Boolean).join(" ").trim() : "";
  const name = isSelf ? "You" : fullName || email;
  const role = contact?.isCsAdmin
    ? "Customer Admin"
    : contact?.isLead
      ? "Project Lead"
      : contact?.isSecurityContact
        ? "Security Contact"
        : undefined;
  const initialsSource = fullName || email;
  const initials =
    initialsSource
      .split(/[\s@.]+/)
      .filter(Boolean)
      .slice(0, 2)
      .map((s) => s[0]?.toUpperCase())
      .join("") || "?";
  return { name, role, initials };
}

function VerificationTimelineItem({
  record,
  contacts,
  currentUserEmail,
  isLast,
}: {
  record: PendingVerificationView;
  contacts: ProjectContact[] | undefined;
  currentUserEmail: string;
  isLast: boolean;
}): JSX.Element {
  const theme = useTheme();
  const isVerified = !!record.verifiedAt;
  const intentColor = isVerified ? theme.palette.success.main : theme.palette.primary.main;
  const intentLight = isVerified ? theme.palette.success.light : theme.palette.primary.light;
  const actor = resolveActorDisplay(
    isVerified ? (record.verifiedBy ?? record.addedBy) : record.addedBy,
    contacts,
    currentUserEmail,
  );

  return (
    <Box sx={{ display: "flex", gap: 2 }}>
      {/* Left: dot + connecting line */}
      <Box sx={{ display: "flex", flexDirection: "column", alignItems: "center", width: 12, flexShrink: 0, pt: 1.5 }}>
        <Box sx={{ width: 8, height: 8, borderRadius: "50%", bgcolor: intentColor, flexShrink: 0 }} />
        {!isLast && (
          <Box sx={{ width: 2, flex: 1, mt: 0.5, bgcolor: "divider", minHeight: 40 }} />
        )}
      </Box>

      {/* Right: content card */}
      <Box
        sx={{
          flex: 1,
          minWidth: 0,
          mb: isLast ? 0 : 3,
          p: 2,
          border: "1px solid",
          borderColor: "divider",
          borderRadius: 1,
        }}
      >
        <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 1, mb: 1.5 }}>
          <Chip
            label={isVerified ? "Verified" : "Added to Verification"}
            size="small"
            variant="outlined"
            sx={{
              fontWeight: 600,
              fontSize: "0.75rem",
              color: intentColor,
              borderColor: alpha(intentColor, 0.5),
              bgcolor: alpha(intentLight, 0.12),
            }}
          />
          <Typography variant="caption" color="text.secondary">
            {formatRelativeTime(isVerified ? (record.verifiedAt ?? undefined) : record.addedAt)}
          </Typography>
        </Box>

        <Stack direction="row" spacing={1.5} alignItems="center" sx={{ mb: 1 }}>
          <Avatar sx={{ width: 28, height: 28, fontSize: "0.75rem", bgcolor: alpha(theme.palette.primary.main, 0.15), color: theme.palette.primary.dark }}>
            {actor.initials}
          </Avatar>
          <Box>
            <Typography variant="body2" fontWeight={600} color="text.primary">
              {actor.name}
              {actor.role && (
                <Typography component="span" variant="body2" color="text.secondary">
                  {" "}
                  · {actor.role}
                </Typography>
              )}
            </Typography>
          </Box>
        </Stack>

        <Box sx={{ display: "flex", alignItems: "center", gap: 0.75, mb: record.note ? 1.5 : 0 }}>
          <Clock size={13} color={theme.palette.text.secondary} />
          <Typography variant="caption" color="text.secondary">
            {formatDate(isVerified ? record.verifiedAt : record.addedAt)}
          </Typography>
        </Box>

        {!isVerified && record.note && (
          <Box sx={{ bgcolor: "action.hover", borderRadius: 1, px: 1.5, py: 1 }}>
            <Typography variant="body2" color="text.secondary">
              {record.note}
            </Typography>
          </Box>
        )}
      </Box>
    </Box>
  );
}

/**
 * Verifications history timeline panel for a case, matching the Figma
 * design's icon-box header + timeline card structure. Shares its query key
 * with the badge/action-row lookup in CaseDetailsContent, so mounting both
 * dedupes to one network call.
 *
 * @param {Props} props - projectId and caseId.
 * @returns {JSX.Element} The verifications history panel.
 */
export default function CaseVerificationsPanel({ projectId, caseId }: Props): JSX.Element {
  const theme = useTheme();
  const { data: userDetails } = useGetUserDetails();
  const currentUserEmail = userDetails?.email?.toLowerCase() ?? "";
  const { data: contacts } = useGetProjectContacts(projectId);

  const { data, isLoading, isError } = usePendingVerificationsSearch(projectId, caseId);

  if (isLoading) {
    return (
      <Box sx={{ display: "flex", justifyContent: "center", py: 6 }}>
        <CircularProgress size={28} />
      </Box>
    );
  }

  if (isError) {
    return (
      <Typography variant="body2" color="error" sx={{ py: 4, textAlign: "center" }}>
        Failed to load verification history.
      </Typography>
    );
  }

  const records = data?.pendingVerifications ?? [];

  return (
    <Box>
      <Stack direction="row" spacing={1.5} alignItems="center" sx={{ mb: 3 }}>
        <Box
          sx={{
            width: 40,
            height: 40,
            borderRadius: 1.5,
            bgcolor: alpha(theme.palette.primary.main, 0.12),
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            flexShrink: 0,
          }}
        >
          <ShieldCheck size={20} color={theme.palette.primary.main} />
        </Box>
        <Box>
          <Typography variant="subtitle1" fontWeight={600}>
            Verifications
          </Typography>
          <Typography variant="body2" color="text.secondary">
            Full history of verification actions taken on this case.
          </Typography>
        </Box>
      </Stack>

      {records.length === 0 ? (
        <Box
          sx={{
            display: "flex",
            flexDirection: "column",
            alignItems: "center",
            py: 6,
            gap: 1,
          }}
        >
          <ShieldCheck size={32} color={theme.palette.text.disabled} />
          <Typography variant="body2" color="text.secondary">
            No verification history for this case yet.
          </Typography>
        </Box>
      ) : (
        <Box>
          {records.map((record, index) => (
            <VerificationTimelineItem
              key={record.id}
              record={record}
              contacts={contacts}
              currentUserEmail={currentUserEmail}
              isLast={index === records.length - 1}
            />
          ))}
        </Box>
      )}
    </Box>
  );
}
