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
import { useLocation, useParams, useSearchParams } from "react-router";
import { Stack } from "@wso2/oxygen-ui";
import { useModifierAwareNavigate } from "@hooks/useModifierAwareNavigate";
import { useDebouncedValue } from "@hooks/useDebouncedValue";
import useGetProjectDetails from "@api/useGetProjectDetails";
import { usePendingVerificationsListSearch } from "@features/support/api/usePendingVerificationsListSearch";
import { useVerifyPendingVerification } from "@features/support/api/useVerifyPendingVerification";
import { useErrorBanner } from "@context/error-banner/ErrorBannerContext";
import { useSuccessBanner } from "@context/success-banner/SuccessBannerContext";
import { buildPendingVerificationRecordPath } from "@features/support/utils/pendingVerification";
import type {
  PendingVerificationAddedReason,
  PendingVerificationView,
} from "@features/support/types/pendingVerification";
import ListPageHeader from "@components/list-view/ListPageHeader";
import ListSearchBar from "@components/list-view/ListSearchBar";
import ListResultsBar from "@components/list-view/ListResultsBar";
import ListPagination from "@components/list-view/ListPagination";
import PendingVerificationsList from "@features/support/components/pending-verifications/PendingVerificationsList";
import CaseStateConfirmDialog from "@features/support/components/case-details/dialogs/CaseStateConfirmDialog";

type VerificationTab = "unverified" | "verified";

// High enough that ListPagination (which hides itself once totalRecords <=
// rowsPerPage) naturally disappears for realistic per-project backlogs,
// matching Figma's ungrouped-by-page "Showing X of X" display, while still
// guarding against a pathologically large one.
const ROWS_PER_PAGE = 50;

/**
 * PendingVerificationsPage lists pending-verification records (Case only, as
 * of the 2026-10-06 scope-down) for the current project, pre-scoped to
 * whichever Support-page stat box was clicked, with inline "View Case"/
 * "Mark Verified" actions per record.
 *
 * @returns {JSX.Element} The rendered page.
 */
export default function PendingVerificationsPage(): JSX.Element {
  const navigate = useModifierAwareNavigate();
  const location = useLocation();
  const { projectId } = useParams<{ projectId: string }>();
  const { showSuccess } = useSuccessBanner();
  const { showError } = useErrorBanner();

  // Scope comes from the URL's own query params (?tab=&addedReason=), not
  // navigation state -- set by SupportPage's stat boxes, and deliberately
  // chosen over state specifically so the Back button from a record's detail
  // page restores the exact same filtered view: `returnTo` there is built
  // from location.pathname + location.search, which already carries these
  // params along for free, whereas state would be silently dropped on the
  // way back (CaseDetailsPage's handleBack only re-attaches `{fromBack:
  // true}`). Absent on a direct URL visit, which just defaults to Unverified
  // with no reason filter.
  const [searchParams] = useSearchParams();
  const activeTab: VerificationTab =
    searchParams.get("tab") === "verified" ? "verified" : "unverified";
  const addedReasonParam = searchParams.get("addedReason");
  const addedReason: PendingVerificationAddedReason | undefined =
    addedReasonParam === "auto-closed" || addedReasonParam === "manual"
      ? addedReasonParam
      : undefined;

  const [searchTerm, setSearchTerm] = useState("");
  const debouncedSearchTerm = useDebouncedValue(searchTerm, 300);
  const [page, setPage] = useState(1);
  const [confirmingRecord, setConfirmingRecord] = useState<PendingVerificationView | null>(null);

  // addedReason only ever narrows the Unverified set -- it describes why an
  // entry is still pending, which has no meaning once something is verified.
  const filters = useMemo(
    () =>
      activeTab === "verified"
        ? { searchQuery: debouncedSearchTerm || undefined, verifiedOnly: true }
        : {
            searchQuery: debouncedSearchTerm || undefined,
            includeVerified: false,
            addedReason,
          },
    [activeTab, debouncedSearchTerm, addedReason],
  );
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

  const records = useMemo(() => data?.pendingVerifications ?? [], [data?.pendingVerifications]);
  const totalRecords = data?.totalRecords ?? 0;
  const hasListRefinement = !!searchTerm;

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

  return (
    <Stack spacing={3} sx={{ minWidth: 0 }}>
      <ListPageHeader
        title="Pending Verification"
        description={
          activeTab === "verified"
            ? "Records you've already signed off on for this project"
            : "Closed records awaiting your sign-off for this project"
        }
        onBack={() => navigate(`/projects/${projectId}/support`)}
      />

      <ListSearchBar
        searchPlaceholder="Search by ID, title, or description..."
        searchTerm={searchTerm}
        onSearchChange={handleSearchChange}
        isFiltersOpen={false}
        onFiltersToggle={() => {}}
        activeFiltersCount={0}
        onClearFilters={() => {}}
        filtersContent={null}
        hideFiltersButton
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
          navigate(buildPendingVerificationRecordPath(projectId, record), {
            state: { returnTo: location.pathname + location.search },
          });
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
