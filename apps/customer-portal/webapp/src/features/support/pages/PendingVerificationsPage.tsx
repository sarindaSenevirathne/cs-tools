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
  const [page, setPage] = useState(1);
  const [confirmingRecord, setConfirmingRecord] = useState<PendingVerificationView | null>(null);
  const [isFiltersOpen, setIsFiltersOpen] = useState(
    () => hasListSearchOrFilters(searchTerm, { recordTypes }),
  );

  // Always fetch both — the search endpoint's includeVerified only adds
  // verified rows alongside unverified ones, it has no "verified only" mode,
  // so both tabs are split client-side from one combined fetch rather than
  // needing two separate queries.
  const filters = useMemo(
    () => ({
      recordTypes: recordTypes.length > 0 ? recordTypes : undefined,
      searchQuery: debouncedSearchTerm || undefined,
      includeVerified: true,
    }),
    [recordTypes, debouncedSearchTerm],
  );

  // Unverified-only, same filters, limit 1 — not used for any rows, only for
  // its server-computed total/autoClosedCount/manualCount, which (unlike the
  // main fetch above) are true full-backlog counts against the current
  // filters, not scoped to whatever page of ROWS_PER_PAGE happens to be
  // loaded. Same pattern as SideBar.tsx's own sidebar-badge count query.
  const statsFilters = useMemo(
    () => ({
      recordTypes: recordTypes.length > 0 ? recordTypes : undefined,
      searchQuery: debouncedSearchTerm || undefined,
      includeVerified: false,
    }),
    [recordTypes, debouncedSearchTerm],
  );
  const statsPagination = useMemo(() => ({ limit: 1, offset: 0 }), []);

  const pagination = useMemo(
    () => ({ limit: ROWS_PER_PAGE, offset: (page - 1) * ROWS_PER_PAGE }),
    [page],
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

  const { data, isLoading, isError } = usePendingVerificationsListSearch(
    projectId || "",
    filters,
    pagination,
    !!projectId && verificationEnabled,
  );

  const { data: statsData, isLoading: isStatsLoading } = usePendingVerificationsListSearch(
    projectId || "",
    statsFilters,
    statsPagination,
    !!projectId && verificationEnabled,
  );

  const allRecords = useMemo(() => data?.pendingVerifications ?? [], [data?.pendingVerifications]);
  const unverifiedRecords = useMemo(() => allRecords.filter((r) => !r.verifiedAt), [allRecords]);
  const verifiedRecords = useMemo(() => allRecords.filter((r) => !!r.verifiedAt), [allRecords]);
  const records = activeTab === "unverified" ? unverifiedRecords : verifiedRecords;
  const totalRecords = records.length;
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
    setPage(1);
  };

  const handleRecordTypesChange = (event: SelectChangeEvent<PendingVerificationRecordType[]>): void => {
    const value = event.target.value;
    setRecordTypes(typeof value === "string" ? (value.split(",") as PendingVerificationRecordType[]) : value);
    setPage(1);
  };

  const handleClearFilters = (): void => {
    setSearchTerm("");
    setRecordTypes([]);
    setPage(1);
  };

  const typeCountByType = useMemo(() => {
    const map = new Map<string, number>();
    for (const tc of data?.typeCounts ?? []) map.set(tc.recordType, tc.count);
    return map;
  }, [data?.typeCounts]);

  const activeFiltersCount = countListSearchAndFilters(searchTerm, { recordTypes });

  // Stats describe the pending (unverified) backlog specifically, matching
  // "Total Pending"/"Auto Marked"/"Manually Marked". Read from statsData's
  // own totalRecords/autoClosedCount/manualCount (the dedicated
  // includeVerified:false, limit:1 query above) rather than derived from
  // unverifiedRecords.length — the latter is scoped to whatever ROWS_PER_PAGE
  // page happens to be loaded, and would silently undercount once a
  // project's filtered unverified backlog exceeds that page size.
  const pendingStats = useMemo(
    () => ({
      total: statsData?.totalRecords ?? 0,
      autoClosed: statsData?.autoClosedCount ?? 0,
      manual: statsData?.manualCount ?? 0,
    }),
    [statsData],
  );

  // Tab badge counts, same pagination-independence reasoning as pendingStats
  // above. data.totalRecords (includeVerified:true) is the server's own
  // COUNT(*) over the full filtered set, run as a separate query from the
  // LIMIT/OFFSET-ed row fetch (pending_verification_repo.go's Search) — so
  // it's already a true combined total, not scoped to ROWS_PER_PAGE, and
  // subtracting statsData's unverified-only total from it gives a true
  // verified-only total with no extra network call.
  const unverifiedTabCount = statsData?.totalRecords ?? 0;
  const verifiedTabCount = Math.max(0, (data?.totalRecords ?? 0) - unverifiedTabCount);

  return (
    <Stack spacing={3} sx={{ minWidth: 0 }}>
      <ListPageHeader
        title="Pending Verification"
        description="Closed records awaiting your sign-off, plus verification history for this project"
      />

      <ListStatGrid
        isLoading={isLoading || isStatsLoading}
        configs={PENDING_VERIFICATION_STAT_CONFIGS}
        stats={pendingStats}
        isError={isError}
        entityName="pending verifications"
      />

      <TabBar
        tabs={[
          { id: "unverified", label: "Unverified", icon: ShieldCheck, count: unverifiedTabCount },
          { id: "verified", label: "Verified", icon: CircleCheck, count: verifiedTabCount, badgeColor: "success.main" },
        ]}
        activeTab={activeTab}
        onTabChange={(id) => {
          setActiveTab(id as VerificationTab);
          setPage(1);
        }}
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
        page={page}
        rowsPerPage={ROWS_PER_PAGE}
        onPageChange={(_e, value) => setPage(value)}
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

