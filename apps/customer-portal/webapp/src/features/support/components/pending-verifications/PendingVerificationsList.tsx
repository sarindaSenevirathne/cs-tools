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
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Box,
  Chip,
  Stack,
  Typography,
  colors,
} from "@wso2/oxygen-ui";
import { ChevronDown } from "@wso2/oxygen-ui-icons-react";
import { useMemo, useState, type JSX } from "react";
import type { PendingVerificationView } from "@features/support/types/pendingVerification";
import { RECORD_TYPE_ICONS } from "@features/support/utils/pendingVerification";
import PendingVerificationCard from "@features/support/components/pending-verifications/PendingVerificationCard";
import ListSkeleton from "@components/list-view/ListSkeleton";
import ErrorIndicator from "@components/error-indicator/ErrorIndicator";
import EmptyIcon from "@components/empty-state/EmptyIcon";
import SearchNoResultsIcon from "@components/empty-state/SearchNoResultsIcon";

export interface PendingVerificationsListProps {
  records: PendingVerificationView[];
  isLoading: boolean;
  isError?: boolean;
  hasListRefinement?: boolean;
  onViewCase?: (record: PendingVerificationView) => void;
  onMarkVerified?: (record: PendingVerificationView) => void;
}

/**
 * PendingVerificationsList groups records by recordType into expandable/
 * collapsible Accordion sections (mirroring DeploymentCard.tsx's controlled-
 * accordion pattern), each with a count chip, plus the loading/error/empty
 * states.
 *
 * @param {PendingVerificationsListProps} props - Records array and loading state.
 * @returns {JSX.Element} The rendered grouped list.
 */
export default function PendingVerificationsList({
  records,
  isLoading,
  isError = false,
  hasListRefinement = false,
  onViewCase,
  onMarkVerified,
}: PendingVerificationsListProps): JSX.Element {
  const groups = useMemo(() => {
    const byType = new Map<string, PendingVerificationView[]>();
    for (const record of records) {
      const list = byType.get(record.recordType) ?? [];
      list.push(record);
      byType.set(record.recordType, list);
    }
    return Array.from(byType.entries());
  }, [records]);

  const [collapsedGroups, setCollapsedGroups] = useState<Record<string, boolean>>({});

  if (isLoading) {
    return <ListSkeleton />;
  }

  if (isError) {
    return (
      <Box sx={{ textAlign: "center", py: 6 }}>
        <ErrorIndicator entityName="pending verifications" size="medium" />
        <Typography variant="body2" color="text.secondary" sx={{ mt: 2 }}>
          Failed to load pending verifications. Please try again.
        </Typography>
      </Box>
    );
  }

  if (records.length === 0) {
    if (hasListRefinement) {
      return (
        <Box sx={{ display: "flex", flexDirection: "column", alignItems: "center", py: 6 }}>
          <SearchNoResultsIcon style={{ width: 200, maxWidth: "100%", height: "auto", marginBottom: 16 }} />
          <Typography variant="body1" color="text.secondary">
            No records found. Try adjusting your filters or search query.
          </Typography>
        </Box>
      );
    }
    return (
      <Box sx={{ display: "flex", flexDirection: "column", alignItems: "center", py: 6 }}>
        <EmptyIcon style={{ width: 200, maxWidth: "100%", height: "auto", marginBottom: 16 }} />
        <Typography variant="body1" color="text.secondary">
          No records pending verification.
        </Typography>
      </Box>
    );
  }

  return (
    <Stack spacing={2}>
      {groups.map(([recordType, groupRecords]) => {
        const expanded = !collapsedGroups[recordType];
        return (
          <Accordion
            key={recordType}
            expanded={expanded}
            onChange={(_, isExpanded) =>
              setCollapsedGroups((prev) => ({ ...prev, [recordType]: !isExpanded }))
            }
            disableGutters
            elevation={0}
            TransitionProps={{ unmountOnExit: true }}
            sx={{
              border: "1px solid",
              borderColor: "divider",
              borderRadius: 1,
              overflow: "hidden",
              "&:before": { display: "none" },
              "&.Mui-expanded": { margin: 0 },
            }}
          >
            <AccordionSummary
              expandIcon={<ChevronDown size={18} color={colors.grey?.[500] ?? "#6B7280"} />}
              sx={{
                px: 2,
                bgcolor: "action.hover",
                "& .MuiAccordionSummary-content": { m: 0, alignItems: "center", gap: 1 },
              }}
            >
              {(() => {
                const GroupIcon =
                  RECORD_TYPE_ICONS[recordType as keyof typeof RECORD_TYPE_ICONS];
                return GroupIcon ? <GroupIcon size={16} /> : null;
              })()}
              <Typography variant="subtitle2" fontWeight={600} color="primary.main">
                {recordType}
              </Typography>
              <Chip
                label={groupRecords.length}
                size="small"
                sx={{ height: 20, fontSize: "0.7rem" }}
              />
            </AccordionSummary>
            <AccordionDetails sx={{ p: 2, display: "flex", flexDirection: "column", gap: 2 }}>
              {groupRecords.map((record) => (
                <PendingVerificationCard
                  key={record.id}
                  record={record}
                  onViewCase={onViewCase}
                  onMarkVerified={onMarkVerified}
                />
              ))}
            </AccordionDetails>
          </Accordion>
        );
      })}
    </Stack>
  );
}
