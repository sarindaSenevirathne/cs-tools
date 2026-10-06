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

import { Box, Stack, Typography } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import type { PendingVerificationView } from "@features/support/types/pendingVerification";
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
 * PendingVerificationsList renders a flat list of cards, plus the loading/
 * error/empty states. Pending Verification is scoped to Case only (product
 * decision, 2026-10-06) -- with a single record type, the per-recordType
 * accordion grouping this used to have added a header around every card for
 * no real benefit, so it was removed rather than left always collapsed to
 * one group.
 *
 * @param {PendingVerificationsListProps} props - Records array and loading state.
 * @returns {JSX.Element} The rendered list.
 */
export default function PendingVerificationsList({
  records,
  isLoading,
  isError = false,
  hasListRefinement = false,
  onViewCase,
  onMarkVerified,
}: PendingVerificationsListProps): JSX.Element {
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
      {records.map((record) => (
        <PendingVerificationCard
          key={record.id}
          record={record}
          onViewCase={onViewCase}
          onMarkVerified={onMarkVerified}
        />
      ))}
    </Stack>
  );
}
