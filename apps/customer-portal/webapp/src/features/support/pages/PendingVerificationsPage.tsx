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

import { useEffect, useMemo, useState, type JSX } from "react";
import { useParams } from "react-router";
import {
  Box,
  Checkbox,
  FormControl,
  InputLabel,
  MenuItem,
  Select,
  Stack,
  Typography,
  type SelectChangeEvent,
} from "@wso2/oxygen-ui";
import { ShieldCheck, CircleCheck } from "@wso2/oxygen-ui-icons-react";
import { useModifierAwareNavigate } from "@hooks/useModifierAwareNavigate";
import { useDebouncedValue } from "@hooks/useDebouncedValue";
import useGetProjectDetails from "@api/useGetProjectDetails";
import { usePendingVerificationsListSearch } from "@features/support/api/usePendingVerificationsListSearch";
import { useVerifyPendingVerification } from "@features/support/api/useVerifyPendingVerification";
import { useErrorBanner } from "@context/error-banner/ErrorBannerContext";
import { useSuccessBanner } from "@context/success-banner/SuccessBannerContext";
import { PENDING_VERIFICATION_STAT_CONFIGS } from "@features/support/constants/supportConstants";
import {
  buildPendingVerificationRecordPath,
  RECORD_TYPE_ICONS,
} from "@features/support/utils/pendingVerification";
import { hasListSearchOrFilters, countListSearchAndFilters } from "@features/support/utils/listView";
import type {
  PendingVerificationRecordType,
  PendingVerificationView,
} from "@features/support/types/pendingVerification";
import ListPageHeader from "@components/list-view/ListPageHeader";
import ListStatGrid from "@components/list-view/ListStatGrid";
import ListSearchBar from "@components/list-view/ListSearchBar";
import ListResultsBar from "@components/list-view/ListResultsBar";
import ListPagination from "@components/list-view/ListPagination";
import PendingVerificationsList from "@features/support/components/pending-verifications/PendingVerificationsList";
import CaseStateConfirmDialog from "@features/support/components/case-details/dialogs/CaseStateConfirmDialog";
import TabBar from "@components/tab-bar/TabBar";

type VerificationTab = "unverified" | "verified";

const RECORD_TYPES: PendingVerificationRecordType[] = [
  "Case",
  "Security Report",
  "Engagement",
  "Service Request",
  "Change Request",
  "Announcement",
];

// High enough that ListPagination (which hides itself once totalRecords <=
// rowsPerPage) naturally disappears for realistic per-project backlogs,
// matching Figma's ungrouped-by-page "Showing X of X" display, while still
// guarding against a pathologically large one.
const ROWS_PER_PAGE = 50;

/**
 * PendingVerificationsPage lists all pending-verification records for the
 * current project, grouped by record type, with inline "View Case"/
 * "Mark Verified" actions per record.
 *
 * @returns {JSX.Element} The rendered page.
 */
export default function PendingVerificationsPage(): JSX.Element {
  const navigate = useModifierAwareNavigate();
  const { projectId } = useParams<{ projectId: string }>();
  const { showSuccess } = useSuccessBanner();
  const { showError } = useErrorBanner();

  const [searchTerm, setSearchTerm] = useState("");
  const debouncedSearchTerm = useDebouncedValue(searchTerm, 300);
  const [recordTypes, setRecordTypes] = useState<PendingVerificationRecordType[]>([]);
  const [activeTab, setActiveTab] = useState<VerificationTab>("unverified");
  // Independent per-tab page state — paginating Unverified to page 2, then
  // switching to Verified and back, lands you back on Unverified page 2
  // rather than snapping to page 1.
  const [unverifiedPage, setUnverifiedPage] = useState(1);
  const [verifiedPage, setVerifiedPage] = useState(1);
  const [confirmingRecord, setConfirmingRecord] = useState<PendingVerificationView | null>(null);
  const [isFiltersOpen, setIsFiltersOpen] = useState(
    () => hasListSearchOrFilters(searchTerm, { recordTypes }),
  );

  const sharedFilters = useMemo(
    () => ({
      recordTypes: recordTypes.length > 0 ? recordTypes : undefined,
      searchQuery: debouncedSearchTerm || undefined,
    }),
    [recordTypes, debouncedSearchTerm],
  );

  // Two independent, always-live queries (not gated on activeTab) — each is
  // both its own tab's row source AND its own true, unpaginated count source
  // (totalRecords/autoClosedCount/manualCount are a separate COUNT(*) query
  // server-side, unaffected by LIMIT/OFFSET — see pending_verification_repo.go's
  // Search). Keeping both always-on is what lets both tab badges and the
  // summary tiles stay accurate regardless of which tab is currently open.
  const unverifiedFilters = useMemo(
    () => ({ ...sharedFilters, includeVerified: false }),
    [sharedFilters],
  );
  const verifiedFilters = useMemo(
    () => ({ ...sharedFilters, verifiedOnly: true }),
    [sharedFilters],
  );
  const unverifiedPagination = useMemo(
    () => ({ limit: ROWS_PER_PAGE, offset: (unverifiedPage - 1) * ROWS_PER_PAGE }),
    [unverifiedPage],
  );
  const verifiedPagination = useMemo(
    () => ({ limit: ROWS_PER_PAGE, offset: (verifiedPage - 1) * ROWS_PER_PAGE }),
    [verifiedPage],
  );

  const { data: projectDetails } = useGetProjectDetails(projectId || "");
  const verificationEnabled = !!projectDetails?.verificationEnabled;

  // Guards direct URL access even with the sidebar item hidden — mirrors
  // CaseDetailsPage's own redirect-on-misroute useEffect.
  useEffect(() => {
    if (!projectId || !projectDetails) return;
    if (!verificationEnabled) {
      navigate(`/projects/${projectId}/dashboard`, { replace: true });
    }
  }, [projectId, projectDetails, verificationEnabled, navigate]);

  const unverifiedQuery = usePendingVerificationsListSearch(
    projectId || "",
    unverifiedFilters,
    unverifiedPagination,
    !!projectId && verificationEnabled,
  );
  const verifiedQuery = usePendingVerificationsListSearch(
    projectId || "",
    verifiedFilters,
    verifiedPagination,
    !!projectId && verificationEnabled,
  );

  const activeQuery = activeTab === "unverified" ? unverifiedQuery : verifiedQuery;
  const { data, isLoading, isError } = activeQuery;
  const records = useMemo(() => data?.pendingVerifications ?? [], [data?.pendingVerifications]);
  const totalRecords = data?.totalRecords ?? 0;
  const hasListRefinement = !!searchTerm || recordTypes.length > 0;

  // Single mutation instance, re-parameterized by whichever record is
  // currently pending confirmation — avoids instantiating one hook per card.
  const verifyPendingVerification = useVerifyPendingVerification(
    projectId || "",
    confirmingRecord?.workItemId ?? "",
  );

  const handleMarkVerifiedConfirm = (): void => {
    if (!confirmingRecord) return;
    verifyPendingVerification.mutate(confirmingRecord.id, {
      onSuccess: () => showSuccess("Case marked as verified."),
      onError: (err) => showError(err?.message ?? "Failed to mark case as verified."),
      onSettled: () => setConfirmingRecord(null),
    });
  };

  const handleSearchChange = (value: string): void => {
    setSearchTerm(value);
    setUnverifiedPage(1);
    setVerifiedPage(1);
  };

  const handleRecordTypesChange = (event: SelectChangeEvent<PendingVerificationRecordType[]>): void => {
    const value = event.target.value;
    setRecordTypes(typeof value === "string" ? (value.split(",") as PendingVerificationRecordType[]) : value);
    setUnverifiedPage(1);
    setVerifiedPage(1);
  };

  const handleClearFilters = (): void => {
    setSearchTerm("");
    setRecordTypes([]);
    setUnverifiedPage(1);
    setVerifiedPage(1);
  };

  // The active tab's own type breakdown — correctly reflects only what that
  // tab would actually show (each tab now has its own real typeCounts,
  // rather than always showing the combined-both-tabs breakdown).
  const typeCountByType = useMemo(() => {
    const map = new Map<string, number>();
    for (const tc of data?.typeCounts ?? []) map.set(tc.recordType, tc.count);
    return map;
  }, [data?.typeCounts]);

  const activeFiltersCount = countListSearchAndFilters(searchTerm, { recordTypes });

  // Stats describe the pending (unverified) backlog specifically, matching
  // "Total Pending"/"Auto Marked"/"Manually Marked" — read straight from
  // unverifiedQuery's own totalRecords/autoClosedCount/manualCount, a true
  // full-backlog count against the current filters regardless of which page
  // is loaded (see the comment on unverifiedFilters/verifiedFilters above).
  const pendingStats = useMemo(
    () => ({
      total: unverifiedQuery.data?.totalRecords ?? 0,
      autoClosed: unverifiedQuery.data?.autoClosedCount ?? 0,
      manual: unverifiedQuery.data?.manualCount ?? 0,
    }),
    [unverifiedQuery.data],
  );

  const unverifiedTabCount = unverifiedQuery.data?.totalRecords ?? 0;
  const verifiedTabCount = verifiedQuery.data?.totalRecords ?? 0;

  return (
    <Stack spacing={3} sx={{ minWidth: 0 }}>
      <ListPageHeader
        title="Pending Verification"
        description="Closed records awaiting your sign-off, plus verification history for this project"
      />

      <ListStatGrid
        isLoading={unverifiedQuery.isLoading}
        configs={PENDING_VERIFICATION_STAT_CONFIGS}
        stats={pendingStats}
        isError={unverifiedQuery.isError}
        entityName="pending verifications"
      />

      <TabBar
        tabs={[
          { id: "unverified", label: "Unverified", icon: ShieldCheck, count: unverifiedTabCount },
          { id: "verified", label: "Verified", icon: CircleCheck, count: verifiedTabCount, badgeColor: "success.main" },
        ]}
        activeTab={activeTab}
        onTabChange={(id) => setActiveTab(id as VerificationTab)}
      />

      <ListSearchBar
        searchPlaceholder="Search by ID, title, or description..."
        searchTerm={searchTerm}
        onSearchChange={handleSearchChange}
        isFiltersOpen={isFiltersOpen}
        onFiltersToggle={() => setIsFiltersOpen((prev) => !prev)}
        activeFiltersCount={activeFiltersCount}
        onClearFilters={handleClearFilters}
        filtersContent={
          <Stack direction={{ xs: "column", sm: "row" }} spacing={2} alignItems={{ xs: "stretch", sm: "center" }}>
            <FormControl size="small" sx={{ minWidth: 260 }}>
              <InputLabel id="pending-verification-type-label">Type</InputLabel>
              <Select<PendingVerificationRecordType[]>
                multiple
                labelId="pending-verification-type-label"
                label="Type"
                value={recordTypes}
                onChange={handleRecordTypesChange}
                renderValue={(selected) => (selected.length === 0 ? "All types" : selected.join(", "))}
              >
                {RECORD_TYPES.map((type) => {
                  const Icon = RECORD_TYPE_ICONS[type];
                  const count = typeCountByType.get(type);
                  return (
                    <MenuItem key={type} value={type}>
                      <Checkbox checked={recordTypes.includes(type)} size="small" />
                      <Box sx={{ display: "flex", alignItems: "center", gap: 1, flex: 1, minWidth: 0 }}>
                        <Icon size={14} />
                        <Typography variant="body2" sx={{ flex: 1 }}>
                          {type}
                        </Typography>
                        {count !== undefined && (
                          <Typography variant="caption" color="text.secondary">
                            {count}
                          </Typography>
                        )}
                      </Box>
                    </MenuItem>
                  );
                })}
              </Select>
            </FormControl>
          </Stack>
        }
      />

      <ListResultsBar
        shownCount={records.length}
        totalCount={totalRecords}
        entityLabel="records"
      />

      <PendingVerificationsList
        records={records}
        isLoading={isLoading}
        isError={isError}
        hasListRefinement={hasListRefinement}
        onViewCase={(record) => {
          if (!projectId) return;
          navigate(buildPendingVerificationRecordPath(projectId, record));
        }}
        onMarkVerified={(record) => setConfirmingRecord(record)}
      />

      <ListPagination
        totalRecords={totalRecords}
        page={activeTab === "unverified" ? unverifiedPage : verifiedPage}
        rowsPerPage={ROWS_PER_PAGE}
        onPageChange={(_e, value) =>
          activeTab === "unverified" ? setUnverifiedPage(value) : setVerifiedPage(value)
        }
        onRowsPerPageChange={() => {
          /* fixed page size for this list */
        }}
      />

      <CaseStateConfirmDialog
        open={!!confirmingRecord}
        title="Mark as Verified"
        actionLabel="mark as verified"
        isPending={verifyPendingVerification.isPending}
        onClose={() => setConfirmingRecord(null)}
        onConfirm={handleMarkVerifiedConfirm}
      />
    </Stack>
  );
}

