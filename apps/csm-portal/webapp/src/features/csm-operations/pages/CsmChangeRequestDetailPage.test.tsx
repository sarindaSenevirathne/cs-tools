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

import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import "@testing-library/jest-dom/vitest";
import { useEffect, useState, type JSX } from "react";
import { MemoryRouter, Route, Routes, useLocation } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { UseQueryResult } from "@tanstack/react-query";
import type { BeChangeRequestApproval, BeChangeRequestDetail } from "@api/backend/types";
import { BackendApiError } from "@api/backend/client";
import { CaseTabsProvider, useCaseTabsController } from "@context/case-tabs/CaseTabsContext";
import { CaseTabsBehaviorProvider } from "@context/case-tabs/CaseTabsBehaviorContext";
import { useCaseTabCloseConfirm } from "@features/case-tabs/hooks/useCaseTabCloseConfirm";
import LoggerProvider from "@context/logger/LoggerProvider";

const navigateMock = vi.fn();
const useGetChangeRequestMock = vi.fn();
const patchMutateMock = vi.fn();
const patchMutateAsyncMock = vi.fn<(input: unknown) => Promise<unknown>>();
const postCommentMutateAsyncMock = vi.fn<(input: unknown) => Promise<unknown>>();
const patchResetMock = vi.fn();
const showErrorMock = vi.fn();
const editChangeRequestDialogMock = vi.fn();
let patchIsPending = false;
let patchIsError = false;
let patchError: Error | null = null;

// The backend client reads runtime config (`CSM_PORTAL_BACKEND_BASE_URL`) at
// module load, which isn't present under vitest. The page imports
// `BackendApiError` from it directly, so stub the module with a real class
// (so `instanceof` still works) — same approach as CsmIncidentDetailPage.test.tsx.
// `useBackendApi` also has to be stubbed here (not just `BackendApiError`):
// this page's comment-edit/delete wiring goes through the real, unmocked
// `usePatchComment`/`useDeleteComment` (@features/csm-cases/api/useCsmCaseComments),
// which calls `useBackendApi()` unconditionally on every render — leaving it
// undefined throws "No useBackendApi export" the moment the page mounts.
vi.mock("@api/backend/client", () => ({
  BackendApiError: class BackendApiError extends Error {
    status: number;
    constructor(status: number, message: string) {
      super(message);
      this.status = status;
    }
  },
  useBackendApi: () => ({ get: vi.fn(), post: vi.fn(), patch: vi.fn(), delete: vi.fn() }),
}));

vi.mock("@hooks/useNavTransition", () => ({
  useNavTransition: () => navigateMock,
}));
vi.mock("@context/error-banner/ErrorBannerContext", () => ({
  useErrorBanner: () => ({ showError: showErrorMock }),
}));
// `usePortalAccess()` (for `canDownloadAttachment`) pulls in `useCurrentUser`
// -> `CurrentUserContext` -> `useGetUsersMe`, which reads `@config/apiConfig`
// at module load — unavailable under vitest (see this repo's own testing
// conventions). Mocking `CurrentUserContext` directly short-circuits that
// chain before it ever reaches `apiConfig`, same approach as
// CsmIncidentDetailPage.test.tsx.
let mockCurrentUser: { id: string; email: string } = {
  id: "00000000-0000-0000-0000-00000000000c",
  email: "jane.doe@example.com",
};
vi.mock("@context/current-user/CurrentUserContext", () => ({
  useCurrentUser: () => ({
    user: mockCurrentUser,
    isLoading: false,
    isError: false,
    error: null,
  }),
}));
// The lifecycle describe at the bottom of this file drives a stateful fake
// backend through these same hook mocks: whenever it changes the fake CR it
// calls `notifyFakeBackendChanged()`, which re-renders every mounted consumer of
// the mocked query hooks (standing in for react-query's refetch-after-invalidate).
const fakeBackendListeners = new Set<() => void>();
function notifyFakeBackendChanged(): void {
  act(() => fakeBackendListeners.forEach((l) => l()));
}
function useFakeBackendTick(): void {
  const [, setTick] = useState(0);
  useEffect(() => {
    const listener = (): void => setTick((n) => n + 1);
    fakeBackendListeners.add(listener);
    return () => {
      fakeBackendListeners.delete(listener);
    };
  }, []);
}
vi.mock("@features/csm-operations/api/useGetChangeRequest", () => ({
  useGetChangeRequest: () => {
    useFakeBackendTick();
    return useGetChangeRequestMock();
  },
}));
const useGetChangeRequestApprovalsMock = vi.fn();
vi.mock("@features/csm-operations/api/useGetChangeRequestApprovals", () => ({
  useGetChangeRequestApprovals: () => {
    useFakeBackendTick();
    return useGetChangeRequestApprovalsMock();
  },
}));
const decideApprovalMutateMock = vi.fn();
vi.mock("@features/csm-operations/api/useDecideChangeRequestApproval", () => ({
  useDecideChangeRequestApproval: () => ({ mutate: decideApprovalMutateMock, isPending: false }),
}));
vi.mock("@features/csm-operations/api/usePatchChangeRequest", () => ({
  usePatchChangeRequest: () => ({
    mutate: patchMutateMock,
    mutateAsync: patchMutateAsyncMock,
    reset: patchResetMock,
    isPending: patchIsPending,
    isError: patchIsError,
    error: patchError,
  }),
}));
const approvalsPanelMock = vi.fn();
// The real approvals panel, wrapped so tests can also inspect the props the
// page hands it (e.g. `isCreator`). Its data comes from the mocked
// `useGetChangeRequestApprovals` above, so it renders "No approval stages" in
// every test that doesn't set approvals.
vi.mock("@features/csm-operations/components/ChangeRequestApprovals", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@features/csm-operations/components/ChangeRequestApprovals")>();
  const Actual = actual.default;
  return {
    default: (props: { id: string | undefined; isCreator?: boolean }) => {
      approvalsPanelMock(props);
      return <Actual {...props} />;
    },
  };
});
// Exercised in isolation by EditChangeRequestDialog.test.tsx; here we only
// assert this page wires `saveError` and resets the mutation before opening.
vi.mock("@features/csm-operations/components/EditChangeRequestDialog", () => ({
  default: (props: unknown) => {
    editChangeRequestDialogMock(props);
    return null;
  },
}));
vi.mock("@features/csm-operations/api/useCsmChangeRequestComments", () => ({
  useGetCsmChangeRequestComments: () => ({ data: [] }),
  usePostCsmChangeRequestComment: () => ({
    isPending: false,
    mutate: vi.fn(),
    mutateAsync: postCommentMutateAsyncMock,
  }),
}));
vi.mock("@features/csm-cases/api/useCsmCaseAttachments", () => ({
  useGetCsmCaseAttachments: () => ({ data: [] }),
  usePostCsmCaseAttachment: () => ({ isPending: false, mutate: vi.fn() }),
  useDownloadCsmCaseAttachment: () => vi.fn(),
  // Only reached by the reply composer's upload modal (`CsmUploadAttachmentModal`),
  // not exercised by this file's existing tests — the "reports its own draft
  // state" tests below are the first to actually mount the composer.
  MAX_ATTACHMENT_SIZE_BYTES: 10 * 1024 * 1024,
}));
vi.mock("@features/csm-cases/components/CaseActivitiesFeed", () => ({
  default: () => null,
}));
vi.mock("@features/csm-cases/components/CaseDetailWidgets", () => ({
  AttachmentsWidget: () => null,
}));

// Imported after the mocks above so the module picks them up.
import CsmChangeRequestDetailPage from "@features/csm-operations/pages/CsmChangeRequestDetailPage";

const BASE_CR: BeChangeRequestDetail = {
  id: "chg-1",
  number: "CHG0009988",
  subject: "Upgrade the gateway cluster",
  case: { id: "case-1", name: "CASE0001234" },
  createdOn: "2026-01-01T00:00:00Z",
  state: "new",
  type: "normal",
  assignedTeam: { id: "team-1", name: "Platform" },
};

function mockQueryResult(
  overrides: Partial<UseQueryResult<BeChangeRequestDetail | null, Error>>,
): void {
  useGetChangeRequestMock.mockReturnValue({
    data: null,
    isLoading: false,
    isError: false,
    error: null,
    ...overrides,
  });
}

beforeEach(() => {
  mockCurrentUser = { id: "00000000-0000-0000-0000-00000000000c", email: "jane.doe@example.com" };
  decideApprovalMutateMock.mockReset();
  navigateMock.mockClear();
  patchMutateMock.mockClear();
  showErrorMock.mockClear();
  patchIsPending = false;
  patchIsError = false;
  patchError = null;
  patchResetMock.mockClear();
  editChangeRequestDialogMock.mockClear();
  approvalsPanelMock.mockClear();
  patchMutateAsyncMock.mockReset();
  patchMutateAsyncMock.mockResolvedValue({ id: "chg-1" });
  postCommentMutateAsyncMock.mockReset();
  postCommentMutateAsyncMock.mockResolvedValue({ id: "comment-1" });
  useGetChangeRequestApprovalsMock.mockReturnValue({
    data: null,
    isLoading: false,
    isError: false,
    error: null,
  });
});

/** Surfaces the router's current search string, for the `?tab=` sync tests
 * below. */
function LocationSearchProbe(): JSX.Element {
  const location = useLocation();
  return <div data-testid="search-probe">{location.search}</div>;
}

/**
 * Real `<MemoryRouter>`/`<Routes>` (not a mocked `react-router`) — matches
 * this app's own convention for a hook/page that reads the router itself,
 * and `useQueryParamTabs` needs a real `useSearchParams` to actually
 * read/write the URL.
 */
function renderPage(
  initialEntry = "/operations/change-requests/chg-1",
): ReturnType<typeof render> {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialEntry]}>
        <Routes>
          <Route
            path="/operations/change-requests/:id"
            element={
              <>
                <CsmChangeRequestDetailPage />
                <LocationSearchProbe />
              </>
            }
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe("CsmChangeRequestDetailPage", () => {
  it("renders the linked case as a clickable reference to the case route", () => {
    mockQueryResult({ data: BASE_CR });
    renderPage();

    screen
      .getByText("CASE0001234")
      .closest('[role="button"]')
      ?.dispatchEvent(new MouseEvent("click", { bubbles: true }));

    expect(navigateMock).toHaveBeenCalledWith("/cases/case-1");
  });

  it("renders a dash for the linked case when there is no case reference", () => {
    mockQueryResult({ data: { ...BASE_CR, case: null } });
    renderPage();
    const linkedCaseCell = screen.getByText("Linked case").parentElement!;
    expect(within(linkedCaseCell).getByText("—")).toBeInTheDocument();
  });

  it("shows the Impact meta cell in the Overview grid, alongside the header chip", () => {
    mockQueryResult({ data: { ...BASE_CR, impact: "high" } });
    renderPage();
    const impactCell = screen.getByText("Impact").parentElement!;
    expect(within(impactCell).getByText("High")).toBeInTheDocument();
  });

  it("shows a dash for Impact in the Overview grid when unset", () => {
    mockQueryResult({ data: { ...BASE_CR, impact: undefined } });
    renderPage();
    const impactCell = screen.getByText("Impact").parentElement!;
    expect(within(impactCell).getByText("—")).toBeInTheDocument();
  });

  it("renders the lifecycle stepper for this CR's state", () => {
    mockQueryResult({ data: { ...BASE_CR, state: "implement" } });
    renderPage();
    expect(screen.getByRole("list", { name: /change request lifecycle/i })).toBeInTheDocument();
  });
});

describe("CsmChangeRequestDetailPage — blocking-reason header note", () => {
  it("shows 'Awaiting Peer Approval' when the Assess stage is pending or requested", () => {
    mockQueryResult({ data: { ...BASE_CR, state: "assess" } });
    useGetChangeRequestApprovalsMock.mockReturnValue({
      data: {
        approvals: [
          {
            stage: "Assess",
            approverType: "STATIC_GROUP",
            approverName: null,
            status: "REQUESTED",
            approvers: [],
          },
        ],
      },
      isLoading: false,
      isError: false,
      error: null,
    });
    renderPage();
    expect(screen.getByText("Awaiting Peer Approval")).toBeInTheDocument();
  });

  it("names the CAB stage by its stage label, not the approver group, and never doubles 'approval'", () => {
    mockQueryResult({ data: { ...BASE_CR, state: "authorize" } });
    useGetChangeRequestApprovalsMock.mockReturnValue({
      data: {
        approvals: [
          {
            stage: "Authorize",
            approverType: "STATIC_GROUP",
            approverName: "Devops Approval",
            status: "PENDING",
            approvers: [],
          },
        ],
      },
      isLoading: false,
      isError: false,
      error: null,
    });
    renderPage();
    expect(screen.getByText("Awaiting CAB Approval")).toBeInTheDocument();
  });

  it("shows no blocking-reason note when no stage is pending/requested", () => {
    mockQueryResult({ data: { ...BASE_CR, state: "authorize" } });
    useGetChangeRequestApprovalsMock.mockReturnValue({
      data: {
        approvals: [
          {
            stage: "Assess",
            approverType: "STATIC_GROUP",
            approverName: null,
            status: "APPROVED",
            approvers: [],
          },
        ],
      },
      isLoading: false,
      isError: false,
      error: null,
    });
    renderPage();
    expect(screen.queryByText(/awaiting/i)).not.toBeInTheDocument();
  });

  it("suppresses the blocking-reason note once the CR is closed, even with stale pending approval data", () => {
    mockQueryResult({ data: { ...BASE_CR, state: "closed" } });
    useGetChangeRequestApprovalsMock.mockReturnValue({
      data: {
        approvals: [
          {
            stage: "Assess",
            approverType: "STATIC_GROUP",
            approverName: null,
            status: "REQUESTED",
            approvers: [],
          },
        ],
      },
      isLoading: false,
      isError: false,
      error: null,
    });
    renderPage();
    expect(screen.queryByText(/awaiting/i)).not.toBeInTheDocument();
  });
});

describe("CsmChangeRequestDetailPage — tab lives in the URL", () => {
  it("defaults to the Approval tab when ?tab= is absent", () => {
    mockQueryResult({ data: BASE_CR });
    renderPage();

    expect(screen.getByRole("tab", { name: /approval/i })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });

  it("writes the selected tab to ?tab= when switching tabs", () => {
    mockQueryResult({ data: BASE_CR });
    renderPage();

    fireEvent.click(screen.getByRole("tab", { name: /attachments/i }));

    expect(screen.getByTestId("search-probe")).toHaveTextContent("tab=attachments");
  });

  it("restores the tab named in the URL on a direct/cold load", () => {
    mockQueryResult({ data: BASE_CR });
    renderPage("/operations/change-requests/chg-1?tab=comments");

    expect(screen.getByRole("tab", { name: /comments/i })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });

  it("falls back to Approval for an unrecognised ?tab= value, without crashing", () => {
    mockQueryResult({ data: BASE_CR });
    renderPage("/operations/change-requests/chg-1?tab=not-a-real-tab");

    expect(screen.getByRole("tab", { name: /approval/i })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });
});

describe("CsmChangeRequestDetailPage — Clone", () => {
  it("navigates to the create form with router state built from this record", () => {
    mockQueryResult({
      data: {
        ...BASE_CR,
        description: "<p>Upgrade the gateway.</p>",
        impact: "high",
        assignedEngineer: { id: "user-1", name: "Jane Doe" },
      },
    });
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: /clone/i }));
    expect(navigateMock).toHaveBeenCalledWith(
      "/operations/change-requests/new",
      expect.objectContaining({
        state: expect.objectContaining({
          sourceNumber: "CHG0009988",
          subject: "Upgrade the gateway cluster",
          type: "normal",
          impact: "high",
          assignedEngineerId: "user-1",
          assignedEngineerLabel: "Jane Doe",
        }),
      }),
    );
  });

  it("never puts the deployment, state, or approval fields into the clone's router state", () => {
    mockQueryResult({
      data: {
        ...BASE_CR,
        deployment: { id: "dep-1", name: "prod" },
        state: "closed",
        hasCustomerApproved: true,
      },
    });
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: /clone/i }));
    const [, options] = navigateMock.mock.calls[0];
    const keys = Object.keys(options.state);
    expect(keys).not.toContain("deployment");
    expect(keys).not.toContain("state");
    expect(keys).not.toContain("hasCustomerApproved");
  });
});

describe("CsmChangeRequestDetailPage — Request Approval (New -> Assess)", () => {
  it("shows the Request Approval button when the backend flags 'assess' as a legal next state", () => {
    mockQueryResult({ data: { ...BASE_CR, legalNextStates: ["assess"] } });
    renderPage();
    expect(
      screen.getByRole("button", { name: /request approval/i }),
    ).toBeInTheDocument();
  });

  it("hides the button when legalNextStates is empty (no transition available)", () => {
    mockQueryResult({ data: { ...BASE_CR, legalNextStates: [] } });
    renderPage();
    expect(
      screen.queryByRole("button", { name: /request approval/i }),
    ).not.toBeInTheDocument();
  });

  it("hides the button when legalNextStates is absent — data-driven, no hardcoded state check", () => {
    mockQueryResult({ data: { ...BASE_CR, legalNextStates: undefined } });
    renderPage();
    expect(
      screen.queryByRole("button", { name: /request approval/i }),
    ).not.toBeInTheDocument();
  });

  /**
   * New -> Assess is compulsorily gated on an assigned team, by explicit
   * product decision: the team's own members become the Assess-stage
   * approvers the moment the transition lands, so there is no valid way to
   * enter Assess with no team to assign that stage to. The backend itself
   * rejects this PATCH with no team regardless of what the button does --
   * this is the UI-side half that keeps the click from round-tripping into
   * that rejection.
   */
  it("shows a disabled Request Approval button when the state allows it but there is no assigned team", () => {
    mockQueryResult({
      data: { ...BASE_CR, legalNextStates: ["assess"], assignedTeam: null },
    });
    renderPage();
    const button = screen.getByRole("button", { name: /request approval/i });
    expect(button).toBeDisabled();
    fireEvent.click(button);
    expect(patchMutateMock).not.toHaveBeenCalled();
  });

  it("exposes the blocked reason to keyboard users via a focusable, labeled wrapper", () => {
    mockQueryResult({
      data: { ...BASE_CR, legalNextStates: ["assess"], assignedTeam: null },
    });
    renderPage();
    const button = screen.getByRole("button", { name: /request approval/i });
    const focusTarget = button.closest('[tabindex="0"]');
    expect(focusTarget).not.toBeNull();
    expect(focusTarget).toHaveAttribute(
      "aria-label",
      "Request Approval: Set an assigned team before requesting approval",
    );
  });

  it("leaves Request Approval enabled when both the state and the assigned team allow it", () => {
    mockQueryResult({
      data: { ...BASE_CR, legalNextStates: ["assess"], assignedTeam: { id: "team-1", name: "Platform" } },
    });
    renderPage();
    expect(
      screen.getByRole("button", { name: /request approval/i }),
    ).toBeEnabled();
  });

  it("PATCHes { state: \"assess\" } for this CR when clicked", () => {
    mockQueryResult({ data: { ...BASE_CR, legalNextStates: ["assess"] } });
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: /request approval/i }));
    expect(patchMutateMock).toHaveBeenCalledWith(
      { id: "chg-1", patch: { state: "assess" } },
      expect.objectContaining({ onError: expect.any(Function) }),
    );
  });

  it("surfaces a mutation error via the shared error banner", () => {
    mockQueryResult({ data: { ...BASE_CR, legalNextStates: ["assess"] } });
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: /request approval/i }));
    const [, options] = patchMutateMock.mock.calls[0];
    const err = new Error("boom");
    options.onError(err);
    expect(showErrorMock).toHaveBeenCalledWith(
      "Could not move this change request to Assess.",
      err,
    );
  });

  it("surfaces the backend's real rejection reason for a 4xx state-transition error", () => {
    mockQueryResult({ data: { ...BASE_CR, legalNextStates: ["assess"] } });
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: /request approval/i }));
    const [, options] = patchMutateMock.mock.calls[0];
    const err = new BackendApiError(409, "State transition rejected: approver required");
    options.onError(err);
    expect(showErrorMock).toHaveBeenCalledWith(
      "State transition rejected: approver required",
      err,
    );
  });

  it("falls back to the generic message for a 5xx error even with a body message", () => {
    mockQueryResult({ data: { ...BASE_CR, legalNextStates: ["assess"] } });
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: /request approval/i }));
    const [, options] = patchMutateMock.mock.calls[0];
    const err = new BackendApiError(500, "internal error detail");
    options.onError(err);
    expect(showErrorMock).toHaveBeenCalledWith(
      "Could not move this change request to Assess.",
      err,
    );
  });
});

describe("CsmChangeRequestDetailPage — Request Approval flow: no Schedule, creator rules", () => {
  it("never shows a Schedule button, even if the backend still lists scheduled", () => {
    mockQueryResult({
      data: { ...BASE_CR, state: "authorize", legalNextStates: ["scheduled", "canceled"] },
    });
    renderPage();
    expect(screen.queryByRole("button", { name: /schedule/i })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /change state/i }));
    expect(screen.queryByRole("menuitem", { name: /schedule/i })).not.toBeInTheDocument();
  });

  it("shows Scheduled with no Schedule button once CAB approval has moved the CR there", () => {
    mockQueryResult({
      data: { ...BASE_CR, state: "scheduled", legalNextStates: ["implement", "canceled"] },
    });
    renderPage();
    expect(screen.getAllByText("Scheduled").length).toBeGreaterThan(0);
    expect(screen.queryByRole("button", { name: /schedule$/i })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /start implementation/i })).toBeInTheDocument();
  });

  it("tells the approvals panel the signed-in user is the creator when they are the requester", () => {
    mockQueryResult({
      data: {
        ...BASE_CR,
        state: "assess",
        requestedBy: { id: "00000000-0000-0000-0000-00000000000c", name: "Jane Doe" },
      },
    });
    renderPage();
    expect(approvalsPanelMock).toHaveBeenCalledWith(expect.objectContaining({ isCreator: true }));
  });

  it("recognises the creator from createdBy matching their email", () => {
    mockQueryResult({ data: { ...BASE_CR, state: "assess", createdBy: "Jane.Doe@example.com" } });
    renderPage();
    expect(approvalsPanelMock).toHaveBeenCalledWith(expect.objectContaining({ isCreator: true }));
  });

  it("does not flag a different user as the creator", () => {
    mockQueryResult({
      data: { ...BASE_CR, state: "assess", requestedBy: { id: "someone-else", name: "Other" }, createdBy: "other@example.com" },
    });
    renderPage();
    expect(approvalsPanelMock).toHaveBeenCalledWith(expect.objectContaining({ isCreator: false }));
  });

  it("still lets the creator Cancel the change request", () => {
    mockQueryResult({
      data: {
        ...BASE_CR,
        state: "assess",
        requestedBy: { id: "00000000-0000-0000-0000-00000000000c", name: "Jane Doe" },
        legalNextStates: ["authorize", "canceled"],
      },
    });
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: /change state/i }));
    expect(screen.getByRole("menuitem", { name: /cancel change/i })).toBeEnabled();
  });
});

describe("CsmChangeRequestDetailPage — Edit dialog error wiring", () => {
  it("resets the shared mutation before opening the Edit dialog, so a stale error from elsewhere isn't shown as this save's error", () => {
    mockQueryResult({ data: { ...BASE_CR, legalNextStates: ["assess"] } });
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: /^edit$/i }));
    expect(patchResetMock).toHaveBeenCalled();
  });

  it("passes no saveError to the dialog when the mutation hasn't failed", () => {
    mockQueryResult({ data: { ...BASE_CR, legalNextStates: ["assess"] } });
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: /^edit$/i }));
    const [props] = editChangeRequestDialogMock.mock.calls.at(-1)!;
    expect(props.saveError).toBeNull();
  });

  it("passes the backend's rejection reason as saveError for a 4xx failure", () => {
    patchIsError = true;
    patchError = new BackendApiError(
      400,
      "isCustomerApproved, isCustomerReviewed, and requestApproval are mutually exclusive",
    );
    mockQueryResult({ data: { ...BASE_CR, legalNextStates: ["assess"] } });
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: /^edit$/i }));
    const [props] = editChangeRequestDialogMock.mock.calls.at(-1)!;
    expect(props.saveError).toBe(
      "isCustomerApproved, isCustomerReviewed, and requestApproval are mutually exclusive",
    );
  });

  it("falls back to a generic saveError for a 5xx failure", () => {
    patchIsError = true;
    patchError = new BackendApiError(500, "internal error detail");
    mockQueryResult({ data: { ...BASE_CR, legalNextStates: ["assess"] } });
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: /^edit$/i }));
    const [props] = editChangeRequestDialogMock.mock.calls.at(-1)!;
    expect(props.saveError).toBe("Could not update the change request.");
  });
});

// ---------------------------------------------------------------------------
// Lifecycle transitions. `ChangeRequestActionBar` is exercised in isolation by
// its own test; these cover this page's half of the contract — which patch
// each target produces, and the comment-then-patch ordering the destructive
// ones go through.
// ---------------------------------------------------------------------------

/** Open the action bar's overflow menu. */
function openStateMenu(): void {
  fireEvent.click(screen.getByRole("button", { name: /change state/i }));
}

describe("CsmChangeRequestDetailPage — direct (non-destructive) transitions", () => {
  it("PATCHes { state: target } for a forward move that is not New -> Assess", () => {
    mockQueryResult({
      data: { ...BASE_CR, state: "scheduled", legalNextStates: ["implement"] },
    });
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: /start implementation/i }));
    expect(patchMutateMock).toHaveBeenCalledWith(
      { id: "chg-1", patch: { state: "implement" } },
      expect.objectContaining({ onError: expect.any(Function) }),
    );
  });

  it("sends a plain state PATCH for New -> Assess, same as every other transition", () => {
    mockQueryResult({ data: { ...BASE_CR, legalNextStates: ["assess"] } });
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: /request approval/i }));
    const [{ patch }] = patchMutateMock.mock.calls[0];
    expect(patch).toEqual({ state: "assess" });
  });

  it("sends a state the backend added verbatim, with no frontend change", () => {
    mockQueryResult({
      data: { ...BASE_CR, state: "review", legalNextStates: ["awaiting_vendor"] },
    });
    renderPage();
    openStateMenu();
    fireEvent.click(screen.getByRole("menuitem", { name: /^awaiting vendor$/i }));
    expect(patchMutateMock).toHaveBeenCalledWith(
      { id: "chg-1", patch: { state: "awaiting_vendor" } },
      expect.anything(),
    );
  });

  it("surfaces the backend's real 4xx rejection reason for a transition", () => {
    mockQueryResult({
      data: { ...BASE_CR, state: "scheduled", legalNextStates: ["implement"] },
    });
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: /start implementation/i }));
    const [, options] = patchMutateMock.mock.calls[0];
    const err = new BackendApiError(409, "Change window has not opened yet");
    options.onError(err);
    expect(showErrorMock).toHaveBeenCalledWith("Change window has not opened yet", err);
  });

  it("falls back to a target-specific generic message for a 5xx", () => {
    mockQueryResult({
      data: { ...BASE_CR, state: "scheduled", legalNextStates: ["implement"] },
    });
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: /start implementation/i }));
    const [, options] = patchMutateMock.mock.calls[0];
    const err = new BackendApiError(500, "internal error detail");
    options.onError(err);
    expect(showErrorMock).toHaveBeenCalledWith(
      "Could not move this change request to Implement.",
      err,
    );
  });
});

describe("CsmChangeRequestDetailPage — destructive transitions need a reason first", () => {
  /** Render a CR that can be canceled, and open the confirmation dialog. */
  function openCancelDialog(): void {
    mockQueryResult({
      data: { ...BASE_CR, state: "implement", legalNextStates: ["review", "canceled"] },
    });
    renderPage();
    openStateMenu();
    fireEvent.click(screen.getByRole("menuitem", { name: /cancel change/i }));
  }

  function typeReason(text: string): void {
    fireEvent.change(screen.getByLabelText(/reason/i), { target: { value: text } });
  }

  it("opens the confirmation dialog instead of patching immediately", () => {
    openCancelDialog();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(patchMutateMock).not.toHaveBeenCalled();
    expect(patchMutateAsyncMock).not.toHaveBeenCalled();
  });

  it("posts the reason as a comment BEFORE patching the state", async () => {
    openCancelDialog();
    typeReason("Latency regression in production.");
    fireEvent.click(screen.getByRole("button", { name: /^cancel change$/i }));

    await waitFor(() => expect(patchMutateAsyncMock).toHaveBeenCalled());
    expect(postCommentMutateAsyncMock).toHaveBeenCalledWith({
      changeRequestId: "chg-1",
      // Posted verbatim: the backing store for these notes is plain text.
      bodyHtml: "Latency regression in production.",
      internal: true,
    });
    expect(patchMutateAsyncMock).toHaveBeenCalledWith({
      id: "chg-1",
      patch: { state: "canceled" },
    });
    // Ordering, not just co-occurrence: an unexplained cancellation is worse
    // than a failed one, so the comment must land first.
    expect(
      postCommentMutateAsyncMock.mock.invocationCallOrder[0],
    ).toBeLessThan(patchMutateAsyncMock.mock.invocationCallOrder[0]);
  });

  it("closes the dialog once both halves succeed", async () => {
    openCancelDialog();
    typeReason("Latency regression in production.");
    fireEvent.click(screen.getByRole("button", { name: /^cancel change$/i }));
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  it("does NOT patch the state when the reason comment fails", async () => {
    postCommentMutateAsyncMock.mockRejectedValueOnce(
      new BackendApiError(403, "Comments are disabled on this change request"),
    );
    openCancelDialog();
    typeReason("Latency regression in production.");
    fireEvent.click(screen.getByRole("button", { name: /^cancel change$/i }));

    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent(
        /comments are disabled on this change request/i,
      ),
    );
    expect(patchMutateAsyncMock).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("tells the engineer the reason was recorded when only the state change failed", async () => {
    patchMutateAsyncMock.mockRejectedValueOnce(
      new BackendApiError(409, "Cancellation is not permitted from this state"),
    );
    openCancelDialog();
    typeReason("Latency regression in production.");
    fireEvent.click(screen.getByRole("button", { name: /^cancel change$/i }));

    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent(
        /reason was recorded as a comment, but the state did not change/i,
      ),
    );
    expect(screen.getByRole("alert")).toHaveTextContent(
      /cancellation is not permitted from this state/i,
    );
    expect(screen.getByRole("alert")).toHaveTextContent(/don't need to retype it/i);
  });

  it("retrying after a failed patch re-sends only the state change, never the comment twice", async () => {
    patchMutateAsyncMock.mockRejectedValueOnce(new BackendApiError(409, "Rejected"));
    openCancelDialog();
    typeReason("Latency regression in production.");
    fireEvent.click(screen.getByRole("button", { name: /^cancel change$/i }));
    await waitFor(() => expect(screen.getByRole("alert")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("button", { name: /^cancel change$/i }));
    await waitFor(() => expect(patchMutateAsyncMock).toHaveBeenCalledTimes(2));
    expect(postCommentMutateAsyncMock).toHaveBeenCalledTimes(1);
  });

  it("posts the reason verbatim as plain text, with no markup added around it", async () => {
    // The backing store for these notes is a plain-text field: production
    // entries carry raw newlines and no escaped entities, so anything the
    // portal wraps in markup shows up as literal tags at the source. What the
    // engineer typed is what gets written, character for character.
    const typed = "Latency < 50ms breached & customer impacted.\nCanceled by Jane Doe.";
    openCancelDialog();
    typeReason(typed);
    fireEvent.click(screen.getByRole("button", { name: /^cancel change$/i }));

    await waitFor(() => expect(postCommentMutateAsyncMock).toHaveBeenCalled());
    const posted = postCommentMutateAsyncMock.mock.calls[0][0] as {
      bodyHtml: string;
    };
    expect(posted.bodyHtml).toBe(typed);
  });

  it("routes the cancel transition through the same dialog", () => {
    mockQueryResult({
      data: { ...BASE_CR, state: "scheduled", legalNextStates: ["implement", "canceled"] },
    });
    renderPage();
    openStateMenu();
    fireEvent.click(screen.getByRole("menuitem", { name: /cancel change/i }));
    expect(
      screen.getByRole("heading", { name: /cancel this change request/i }),
    ).toBeInTheDocument();
    expect(patchMutateAsyncMock).not.toHaveBeenCalled();
  });
});

/**
 * Wraps the real page in a real open case-tab (`CaseTabsProvider` +
 * `useCaseTabCloseConfirm`), exposing an "open"/"close-this-tab" trigger —
 * for the `hasDraft`/close-confirm regression test below, which needs the
 * real `useReportCaseTabDraft` wiring inside the page to actually reach the
 * tab strip's own close-confirm dialog, not just a mocked stand-in for it.
 */
function CloseTabHarness({ caseId }: { caseId: string }): JSX.Element {
  const { openTab, tabs } = useCaseTabsController();
  const { requestClose, dialog } = useCaseTabCloseConfirm();
  return (
    <div>
      <button
        onClick={() =>
          openTab(caseId, "change_request", `/operations/change-requests/${caseId}`)
        }
      >
        open-tab
      </button>
      <button
        onClick={() => {
          const tab = tabs.find((t) => t.caseId === caseId);
          if (tab) requestClose(tab);
        }}
      >
        close-tab
      </button>
      {dialog}
    </div>
  );
}

function renderPageWithOpenTab(
  initialEntry = "/operations/change-requests/chg-1",
): ReturnType<typeof render> {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <LoggerProvider>
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={[initialEntry]}>
          <CaseTabsBehaviorProvider>
            <CaseTabsProvider>
              <CloseTabHarness caseId="chg-1" />
              <Routes>
                <Route
                  path="/operations/change-requests/:id"
                  element={<CsmChangeRequestDetailPage />}
                />
              </Routes>
            </CaseTabsProvider>
          </CaseTabsBehaviorProvider>
        </MemoryRouter>
      </QueryClientProvider>
    </LoggerProvider>,
  );
}

describe("CsmChangeRequestDetailPage — reports its own draft state to the tab strip", () => {
  // Regression test for bug: this page only called `useReportCaseTabMeta`,
  // not `useReportCaseTabDraft` (unlike `CsmCaseDetailPage`, which calls
  // both) — its tab's `hasDraft` never became `true`, so closing a change
  // request's tab with a reply half-written skipped the discard-confirm
  // dialog entirely, unlike a case tab in the same situation.
  it("closing this change request's tab with an open (unsent) reply asks for confirmation, same as a case tab does", () => {
    localStorage.setItem("csm.caseTabs.enabled", "1");
    mockQueryResult({ data: BASE_CR });
    renderPageWithOpenTab();

    fireEvent.click(screen.getByText("open-tab"));
    // The reply composer only renders on the Comments tab — "approval" is
    // this page's own default.
    fireEvent.click(screen.getByRole("tab", { name: /comments/i }));
    fireEvent.click(screen.getByText("Add a comment…"));

    fireEvent.click(screen.getByText("close-tab"));
    expect(screen.getByText("Close this case tab?")).toBeInTheDocument();
  });

  it("closing this change request's tab with no reply open closes it immediately, without confirming", () => {
    localStorage.setItem("csm.caseTabs.enabled", "1");
    mockQueryResult({ data: BASE_CR });
    renderPageWithOpenTab();

    fireEvent.click(screen.getByText("open-tab"));
    fireEvent.click(screen.getByText("close-tab"));
    expect(screen.queryByText("Close this case tab?")).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Lifecycle verification per change type
//
//   Normal    New -> Request Approval -> Assess [Peer Approval]
//                 -> Authorize [CAB Approval] -> (auto) Scheduled
//                 -> Implement -> Review -> Closed
//   Emergency New -> Request Approval -> Authorize [ECAB Approval only]
//                 -> (auto) Scheduled
//   Standard  New -> Request Approval -> (auto) Scheduled, no approvals
//
// A small stateful fake of the backend contract sits behind the mocked
// hooks above: the page's own PATCH / approve calls mutate it, and the page is
// re-rendered from it, so every assertion is on what the user actually sees
// after each step. The contract assumed: `legalNextStates` never lists
// `authorize`, and lists `scheduled` only from `customer_approval` (where it
// records the customer's approval); Request Approval is
// `PATCH {state:"assess"}`; approving Peer adds a CAB stage; approving
// CAB/ECAB (or Standard's Request Approval) moves the CR to `customer_approval`
// when `customerApprovalRequired`, else straight to `scheduled`; Review offers
// `customer_review` when `customerReviewRequired`, else `closed`;
// `customer_review` -> `closed`.
// ---------------------------------------------------------------------------

const LC_CREATOR = { id: "u-creator", email: "casey@example.com", name: "Casey Creator" };
const LC_PEER = { id: "u-peer", email: "pat@example.com", name: "Pat Peer" };
const LC_CAB = { id: "u-cab", email: "cam@example.com", name: "Cam Cab" };
const LC_ECAB = { id: "u-ecab", email: "eli@example.com", name: "Eli Ecab" };

interface LcFake {
  cr: BeChangeRequestDetail;
  approvals: BeChangeRequestApproval[];
}
let lc: LcFake;
/** Whether the fake backend sends `approvers[].canDecide` (Postgres source) or
 * omits it (older backend / ServiceNow source), exercising the UI fallback. */
let lcEmitsCanDecide = true;

function lcLegalNextStates(
  state: string,
  flags: { approval: boolean; review: boolean } = {
    approval: lc?.cr.customerApprovalRequired ?? false,
    review: lc?.cr.customerReviewRequired ?? false,
  },
): string[] {
  switch (state) {
    case "new":
      return ["assess", "canceled"];
    case "assess":
      return ["authorize", "canceled"]; // authorize = the approval path, never a button
    case "authorize":
      return ["canceled"];
    case "customer_approval":
      return ["scheduled", "canceled"]; // scheduled = "Record customer approval"
    case "scheduled":
      return ["implement", "canceled"];
    case "implement":
      return ["review", "canceled"];
    case "review":
      return flags.review ? ["customer_review", "canceled"] : ["closed", "canceled"];
    case "customer_review":
      return ["closed", "canceled"];
    default:
      return [];
  }
}

/** Where a CR lands once its internal approval is granted. */
function lcAfterInternalApproval(): string {
  return lc.cr.customerApprovalRequired ? "customer_approval" : "scheduled";
}

function lcStage(name: string, group: string, who: { id: string; name: string }): BeChangeRequestApproval {
  return {
    stage: name,
    approverType: "STATIC_GROUP",
    approverName: group,
    status: "REQUESTED",
    approvers: [{ id: who.id, name: who.name, status: "REQUESTED" }],
  };
}

function lcSetState(state: string): void {
  lc.cr = { ...lc.cr, state, legalNextStates: lcLegalNextStates(state) };
}

function lcPublish(): void {
  useGetChangeRequestMock.mockReturnValue({ data: lc.cr, isLoading: false, isError: false, error: null });
  useGetChangeRequestApprovalsMock.mockReturnValue({
    data: {
      approvals: structuredClone(lc.approvals).map((stage) => ({
        ...stage,
        approvers: stage.approvers.map((a) =>
          lcEmitsCanDecide
            ? {
                ...a,
                // true only on the caller's own REQUESTED row, and never for the creator
                canDecide:
                  a.id === mockCurrentUser.id &&
                  a.status === "REQUESTED" &&
                  mockCurrentUser.id !== lc.cr.requestedBy?.id,
              }
            : a,
        ),
      })),
    },
    isLoading: false,
    isError: false,
    error: null,
  });
  notifyFakeBackendChanged();
}

function lcSeed(
  type: "normal" | "emergency" | "standard",
  flags: { approval: boolean; review: boolean } = { approval: false, review: false },
): void {
  lcEmitsCanDecide = true;
  lc = {
    cr: {
      ...BASE_CR,
      type,
      state: "new",
      requestedBy: { id: LC_CREATOR.id, name: LC_CREATOR.name },
      createdBy: LC_CREATOR.email,
      customerApprovalRequired: flags.approval,
      customerReviewRequired: flags.review,
      legalNextStates: lcLegalNextStates("new", flags),
    },
    approvals: [],
  };
  // The page's own PATCH (Request Approval, Start implementation, ...) drives the fake.
  patchMutateMock.mockImplementation((input: { patch: { state?: string } }) => {
    const target = input.patch.state;
    if (target === "assess") {
      if (lc.cr.type === "standard") lcSetState(lcAfterInternalApproval());
      else if (lc.cr.type === "emergency") {
        lcSetState("authorize");
        lc.approvals = [lcStage("ECAB Approval", "ECAB", LC_ECAB)];
      } else {
        lcSetState("assess");
        lc.approvals = [lcStage("Peer Approval", "Peers", LC_PEER)];
      }
    } else if (target === "scheduled" && lc.cr.state === "customer_approval") {
      lcSetState("scheduled"); // the customer's approval was recorded
    } else if (target && target !== "scheduled" && target !== "authorize" && target !== "customer_approval") {
      lcSetState(target);
    } else {
      throw new Error(`illegal manual transition to ${String(target)}`);
    }
    lcPublish();
  });
  // The approvals panel's Approve/Reject drives the fake as the signed-in user.
  decideApprovalMutateMock.mockImplementation((input: { decision: "approved" | "rejected" }) => {
    const current = lc.approvals.find((a) => a.status === "REQUESTED");
    const row = current?.approvers.find((a) => a.id === mockCurrentUser.id && a.status === "REQUESTED");
    if (!current || !row || mockCurrentUser.id === lc.cr.requestedBy?.id) {
      throw new Error("403: only a non-creator approver with a pending row may decide");
    }
    row.status = input.decision === "approved" ? "APPROVED" : "REJECTED";
    current.status = row.status;
    if (input.decision === "approved") {
      if (current.stage === "Peer Approval") {
        lcSetState("authorize");
        lc.approvals = [...lc.approvals, lcStage("CAB Approval", "CAB", LC_CAB)];
      } else {
        lcSetState(lcAfterInternalApproval()); // CAB / ECAB approval moves the CR on itself
      }
    }
    lcPublish();
  });
  lcPublish();
}

/** Re-opens the page as another signed-in user (a fresh mount, like a new session). */
function lcOpenAs(user: { id: string; email: string }, view?: ReturnType<typeof render>): ReturnType<typeof render> {
  view?.unmount();
  mockCurrentUser = { id: user.id, email: user.email };
  lcPublish(); // canDecide is per caller, so the fake re-serves the approvals for this user
  return renderPage();
}

/** The lifecycle stepper's current step label (`aria-current="step"`). */
function currentStep(): string {
  const list = screen.getByRole("list", { name: /change request lifecycle/i });
  return within(list).getByRole("listitem", { current: "step" }).textContent ?? "";
}

function expectNoManualSchedule(): void {
  expect(screen.queryByRole("button", { name: /schedule/i })).not.toBeInTheDocument();
  expect(screen.queryByRole("menuitem", { name: /schedule/i })).not.toBeInTheDocument();
  expect(screen.queryByText(/move to assess/i)).not.toBeInTheDocument();
}

function approvalsRow(name: string): HTMLElement {
  return screen.getByText(name).closest("tr") as HTMLElement;
}

/** The labels of the lifecycle stepper's steps, in order. */
function stepLabels(): string[] {
  const list = screen.getByRole("list", { name: /change request lifecycle/i });
  return within(list)
    .getAllByRole("listitem")
    .map((li) => li.textContent ?? "");
}

/** Cell value (Yes/No) beside a label on the Approval tab. */
function metaValue(label: string): string {
  return within(screen.getByText(label).parentElement!).getByText(/^(Yes|No)$/).textContent ?? "";
}

/**
 * Drives one Normal change through its whole life with the given Customer
 * Approval / Customer Review settings, asserting what is visible after every
 * step: the stepper's current step, the header note, the Approval tab flags and
 * which actions exist. The CR never gets a Schedule button.
 */
function runNormalLifecycle(approval: boolean, review: boolean): void {
  lcSeed("normal", { approval, review });

  // New: the creator sees Request Approval, no Schedule, no Move to Assess.
  let view = lcOpenAs(LC_CREATOR);
  expect(currentStep()).toBe("New");
  expect(metaValue("Customer approval required")).toBe(approval ? "Yes" : "No");
  expect(metaValue("Customer review required")).toBe(review ? "Yes" : "No");
  // The optional customer steps are on the line only when their checkbox is on.
  expect(stepLabels().includes("Customer Approval")).toBe(approval);
  expect(stepLabels().includes("Customer Review")).toBe(review);
  expect(screen.getByRole("button", { name: "Request Approval" })).toBeInTheDocument();
  expectNoManualSchedule();
  fireEvent.click(screen.getByRole("button", { name: "Request Approval" }));
  expect(patchMutateMock).toHaveBeenCalledWith({ id: "chg-1", patch: { state: "assess" } }, expect.anything());

  // Assess: Peer Approval pending; creator has no Approve/Reject, a notice, and can Cancel.
  expect(currentStep()).toBe("Assess");
  expect(screen.getByText("Awaiting Peer Approval")).toBeInTheDocument();
  expect(within(approvalsRow("Pat Peer")).getByText("Peer Approval")).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: /^approve$/i })).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: /^reject$/i })).not.toBeInTheDocument();
  expect(screen.getByRole("alert")).toHaveTextContent(/you created this change request/i);
  fireEvent.click(screen.getByRole("button", { name: /change state/i }));
  expect(screen.getByRole("menuitem", { name: /cancel change/i })).toBeEnabled();
  expectNoManualSchedule();

  // A peer approves -> Authorize, CAB Approval is the next, separate stage.
  view = lcOpenAs(LC_PEER, view);
  fireEvent.click(screen.getByRole("button", { name: /^approve$/i }));
  expect(currentStep()).toBe("Authorize");
  expect(screen.getByText("Awaiting CAB Approval")).toBeInTheDocument();
  expect(within(approvalsRow("Cam Cab")).getByText("CAB Approval")).toBeInTheDocument();
  expect(within(approvalsRow("Pat Peer")).getByText("Peer Approval")).toBeInTheDocument();
  expect(within(approvalsRow("Pat Peer")).getByText("Approved")).toBeInTheDocument();
  expectNoManualSchedule();

  // A CAB member approves -> Customer Approval when required, else Scheduled.
  view = lcOpenAs(LC_CAB, view);
  expect(screen.getByRole("button", { name: /^approve$/i })).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: /^approve$/i }));
  expect(patchMutateMock).toHaveBeenCalledTimes(1); // only Request Approval was ever a manual PATCH so far

  view = lcOpenAs(LC_CREATOR, view);
  if (approval) {
    expect(currentStep()).toBe("Customer Approval");
    expect(screen.getByText("Awaiting customer approval")).toBeInTheDocument();
    // Neither Start implementation nor a Schedule button yet: only recording the approval (and Cancel).
    expect(screen.queryByRole("button", { name: /start implementation/i })).not.toBeInTheDocument();
    expectNoManualSchedule();
    expect(screen.getByRole("button", { name: "Record customer approval" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /change state/i }));
    expect(screen.getByRole("menuitem", { name: /cancel change/i })).toBeInTheDocument();
    fireEvent.keyDown(screen.getByRole("menu"), { key: "Escape" });
    fireEvent.click(screen.getByRole("button", { name: "Record customer approval" }));
    expect(patchMutateMock).toHaveBeenLastCalledWith({ id: "chg-1", patch: { state: "scheduled" } }, expect.anything());
  } else {
    expect(screen.queryByText("Record customer approval")).not.toBeInTheDocument();
  }

  // Scheduled: nothing awaited, no Schedule button, ready to implement.
  expect(currentStep()).toBe("Scheduled");
  expect(screen.queryByText(/awaiting/i)).not.toBeInTheDocument();
  expect(screen.queryByText("Record customer approval")).not.toBeInTheDocument();
  expectNoManualSchedule();

  // Engineer-driven tail.
  fireEvent.click(screen.getByRole("button", { name: /^start implementation$/i }));
  expect(currentStep()).toBe("Implement");
  fireEvent.click(screen.getByRole("button", { name: /^mark implemented$/i }));
  expect(currentStep()).toBe("Review");
  if (review) {
    // Review offers only "Send for customer review" (no Close) when required.
    expect(screen.getByRole("button", { name: /^send for customer review$/i })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^close$/i })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /change state/i }));
    expect(screen.queryByRole("menuitem", { name: /^close$/i })).not.toBeInTheDocument();
    fireEvent.keyDown(screen.getByRole("menu"), { key: "Escape" });
    fireEvent.click(screen.getByRole("button", { name: /^send for customer review$/i }));
    expect(currentStep()).toBe("Customer Review");
    expect(screen.getByText("Awaiting customer review")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^send for customer review$/i })).not.toBeInTheDocument();
  } else {
    // Review offers only Close (no customer review) when not required.
    expect(screen.queryByRole("button", { name: /send for customer review/i })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /change state/i }));
    expect(screen.queryByRole("menuitem", { name: /customer review/i })).not.toBeInTheDocument();
    fireEvent.keyDown(screen.getByRole("menu"), { key: "Escape" });
  }
  fireEvent.click(screen.getByRole("button", { name: /^close$/i }));
  expect(currentStep()).toBe("Closed");
  expect(screen.queryByText(/awaiting/i)).not.toBeInTheDocument();
  expectNoManualSchedule();
  view.unmount();
}

describe("CsmChangeRequestDetailPage — lifecycle: Normal (Request Approval -> Peer -> CAB -> [Customer Approval] -> Scheduled -> Implement -> Review -> [Customer Review] -> Closed)", () => {
  it.each([
    [false, false],
    [true, false],
    [false, true],
    [true, true],
  ])(
    "shows the right state, stage, header note and controls after every step (customerApprovalRequired=%s, customerReviewRequired=%s)",
    (approval, review) => {
      runNormalLifecycle(approval, review);
    },
  );

  it("lets a non-creator approver both Approve and Reject, and never shows them the creator notice", () => {
    lcSeed("normal");
    patchMutateMock({ id: "chg-1", patch: { state: "assess" } });

    lcOpenAs(LC_PEER);
    expect(screen.getByRole("button", { name: /^approve$/i })).toBeEnabled();
    expect(screen.getByRole("button", { name: /^reject$/i })).toBeEnabled();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("disables Approve/Reject for the creator even when the backend wrongly gives them a pending row and sends no canDecide", () => {
    lcSeed("normal");
    lcEmitsCanDecide = false;
    patchMutateMock({ id: "chg-1", patch: { state: "assess" } });
    lc.approvals = [lcStage("Peer Approval", "Peers", { id: LC_CREATOR.id, name: "Casey Creator" })];
    lcPublish();

    lcOpenAs(LC_CREATOR);
    expect(screen.getByRole("button", { name: /^approve$/i })).toBeDisabled();
    expect(screen.getByRole("button", { name: /^reject$/i })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: /^approve$/i }));
    expect(decideApprovalMutateMock).not.toHaveBeenCalled();
  });
});

describe("CsmChangeRequestDetailPage — lifecycle: backend canDecide on approvers", () => {
  it("disables Approve/Reject for a non-creator whose own pending row the backend marks canDecide=false (e.g. an SRE on the peer stage)", () => {
    lcSeed("normal");
    patchMutateMock({ id: "chg-1", patch: { state: "assess" } });
    lcOpenAs(LC_PEER);
    // Override just this row: the backend reports the caller may not decide it.
    useGetChangeRequestApprovalsMock.mockReturnValue({
      data: {
        approvals: [
          {
            ...lc.approvals[0]!,
            approvers: [{ ...lc.approvals[0]!.approvers[0]!, canDecide: false }],
          },
        ],
      },
      isLoading: false,
      isError: false,
      error: null,
    });
    notifyFakeBackendChanged();
    expect(screen.getByRole("button", { name: /^approve$/i })).toBeDisabled();
    expect(screen.getByRole("button", { name: /^reject$/i })).toBeDisabled();
  });

  it("enables Approve/Reject only on the caller's own pending row when canDecide=true is sent", () => {
    lcSeed("normal");
    patchMutateMock({ id: "chg-1", patch: { state: "assess" } });
    lcOpenAs(LC_PEER);
    expect(screen.getByRole("button", { name: /^approve$/i })).toBeEnabled();
    expect(screen.getAllByRole("button", { name: /^approve$/i })).toHaveLength(1);
  });
});

describe("CsmChangeRequestDetailPage — lifecycle: Emergency (Request Approval -> ECAB only -> auto Scheduled)", () => {
  it("has no Peer or CAB stage and lands on Scheduled when ECAB approves", () => {
    lcSeed("emergency");

    let view = lcOpenAs(LC_CREATOR);
    expect(currentStep()).toBe("New");
    fireEvent.click(screen.getByRole("button", { name: "Request Approval" }));

    expect(currentStep()).toBe("Authorize");
    expect(screen.getByText("Awaiting ECAB Approval")).toBeInTheDocument();
    expect(within(approvalsRow("Eli Ecab")).getByText("ECAB Approval")).toBeInTheDocument();
    expect(screen.queryByText("Peer Approval")).not.toBeInTheDocument();
    expect(screen.queryByText("CAB Approval")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^approve$/i })).not.toBeInTheDocument(); // creator
    expectNoManualSchedule();

    view = lcOpenAs(LC_ECAB, view);
    fireEvent.click(screen.getByRole("button", { name: /^approve$/i }));
    expect(currentStep()).toBe("Scheduled");
    expect(screen.queryByText(/awaiting/i)).not.toBeInTheDocument();
    expectNoManualSchedule();
    expect(screen.getByRole("button", { name: /^start implementation$/i })).toBeInTheDocument();
    view.unmount();
  });
});

describe("CsmChangeRequestDetailPage — lifecycle: Emergency with Customer Approval (ECAB -> Customer Approval -> Scheduled)", () => {
  it("waits in Customer Approval after ECAB approves, and only 'Record customer approval' moves it on", () => {
    lcSeed("emergency", { approval: true, review: false });

    let view = lcOpenAs(LC_CREATOR);
    fireEvent.click(screen.getByRole("button", { name: "Request Approval" }));
    expect(currentStep()).toBe("Authorize");
    expect(screen.getByText("Awaiting ECAB Approval")).toBeInTheDocument();
    expectNoManualSchedule();

    view = lcOpenAs(LC_ECAB, view);
    fireEvent.click(screen.getByRole("button", { name: /^approve$/i }));
    expect(currentStep()).toBe("Customer Approval");
    expect(screen.getByText("Awaiting customer approval")).toBeInTheDocument();
    expect(screen.queryByText("Peer Approval")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /start implementation/i })).not.toBeInTheDocument();
    expectNoManualSchedule();
    expect(screen.getByRole("button", { name: "Record customer approval" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Record customer approval" }));
    expect(patchMutateMock).toHaveBeenLastCalledWith({ id: "chg-1", patch: { state: "scheduled" } }, expect.anything());
    expect(currentStep()).toBe("Scheduled");
    expect(screen.queryByText(/awaiting/i)).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^start implementation$/i })).toBeInTheDocument();
    expectNoManualSchedule();
    view.unmount();
  });
});

describe("CsmChangeRequestDetailPage — lifecycle: Standard with Customer Approval (Request Approval -> Customer Approval -> Scheduled)", () => {
  it("goes to Customer Approval, not straight to Scheduled, and has no approval stages", () => {
    lcSeed("standard", { approval: true, review: false });

    const view = lcOpenAs(LC_CREATOR);
    expect(currentStep()).toBe("New");
    fireEvent.click(screen.getByRole("button", { name: "Request Approval" }));

    expect(currentStep()).toBe("Customer Approval");
    expect(screen.getByText("Awaiting customer approval")).toBeInTheDocument();
    expect(screen.getByText(/no approval stages recorded/i)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /start implementation/i })).not.toBeInTheDocument();
    expectNoManualSchedule();

    fireEvent.click(screen.getByRole("button", { name: "Record customer approval" }));
    expect(patchMutateMock).toHaveBeenLastCalledWith({ id: "chg-1", patch: { state: "scheduled" } }, expect.anything());
    expect(currentStep()).toBe("Scheduled");
    expect(screen.queryByText(/awaiting/i)).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^start implementation$/i })).toBeInTheDocument();
    view.unmount();
  });
});

describe("CsmChangeRequestDetailPage — lifecycle: Standard (Request Approval -> auto Scheduled)", () => {
  it("goes straight to Scheduled with no approval stages and no Schedule button", () => {
    lcSeed("standard");

    const view = lcOpenAs(LC_CREATOR);
    expect(currentStep()).toBe("New");
    fireEvent.click(screen.getByRole("button", { name: "Request Approval" }));

    expect(currentStep()).toBe("Scheduled");
    expect(screen.getByText(/no approval stages recorded/i)).toBeInTheDocument();
    expect(screen.queryByText(/awaiting/i)).not.toBeInTheDocument();
    expectNoManualSchedule();
    expect(screen.getByRole("button", { name: /^start implementation$/i })).toBeInTheDocument();
    view.unmount();
  });
});

describe("CsmChangeRequestDetailPage — lifecycle: defensive against a backend that still offers scheduled", () => {
  it("never renders a Schedule or Authorize action even if legalNextStates lists them", () => {
    lcSeed("normal");
    lcSetState("authorize");
    lc.cr = { ...lc.cr, legalNextStates: ["scheduled", "authorize", "canceled"] };
    lc.approvals = [lcStage("CAB Approval", "CAB", LC_CAB)];
    lcPublish();

    lcOpenAs(LC_CAB);
    fireEvent.click(screen.getByRole("button", { name: /change state/i }));
    expect(screen.getByRole("menuitem", { name: /cancel change/i })).toBeInTheDocument();
    expectNoManualSchedule();
    expect(screen.queryByRole("menuitem", { name: /authorize/i })).not.toBeInTheDocument();
  });
});

describe("CsmChangeRequestDetailPage — lifecycle: scheduled is only ever 'Record customer approval'", () => {
  it("a customer_approval CR that is Canceled from the menu goes through the reason dialog", () => {
    lcSeed("normal", { approval: true, review: false });
    lcSetState("customer_approval");
    lcPublish();
    lcOpenAs(LC_CREATOR);
    expect(currentStep()).toBe("Customer Approval");
    fireEvent.click(screen.getByRole("button", { name: /change state/i }));
    fireEvent.click(screen.getByRole("menuitem", { name: /cancel change/i }));
    // Destructive, so the reason dialog opens instead of patching.
    expect(patchMutateMock).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("never offers 'Record customer approval' from any other state, even if the backend lists scheduled", () => {
    for (const state of ["new", "assess", "authorize", "scheduled", "implement", "review", "customer_review"]) {
      lcSeed("normal", { approval: true, review: true });
      lcSetState(state);
      lc.cr = { ...lc.cr, legalNextStates: ["scheduled"] };
      lcPublish();
      const view = lcOpenAs(LC_CREATOR);
      expect(screen.queryByText("Record customer approval")).not.toBeInTheDocument();
      expectNoManualSchedule();
      view.unmount();
    }
  });
});

describe("CsmChangeRequestDetailPage — customer approval / review flags on the Approval tab", () => {
  it("shows both flags read-only as Yes/No, separate from the customer's confirmation outcome", () => {
    mockQueryResult({
      data: {
        ...BASE_CR,
        customerApprovalRequired: true,
        customerReviewRequired: false,
        hasCustomerApproved: false,
        hasCustomerReviewed: true,
      },
    });
    renderPage();
    expect(metaValue("Customer approval required")).toBe("Yes");
    expect(metaValue("Customer review required")).toBe("No");
    expect(metaValue("Customer approved")).toBe("No");
    expect(metaValue("Customer reviewed")).toBe("Yes");
    // Read-only: no checkbox anywhere on the page itself.
    expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
  });

  it("shows No for both when the backend omits the flags", () => {
    mockQueryResult({ data: BASE_CR });
    renderPage();
    expect(metaValue("Customer approval required")).toBe("No");
    expect(metaValue("Customer review required")).toBe("No");
  });

  it("passes the flags through to the Edit dialog's CR", () => {
    mockQueryResult({
      data: { ...BASE_CR, state: "assess", customerApprovalRequired: true, customerReviewRequired: true },
    });
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: /^edit$/i }));
    const props = editChangeRequestDialogMock.mock.calls.at(-1)![0] as { cr: BeChangeRequestDetail };
    expect(props.cr.customerApprovalRequired).toBe(true);
    expect(props.cr.customerReviewRequired).toBe(true);
  });

  it("carries the flags into the Clone router state but never the customer's confirmation", () => {
    mockQueryResult({
      data: {
        ...BASE_CR,
        customerApprovalRequired: true,
        customerReviewRequired: true,
        hasCustomerApproved: true,
        hasCustomerReviewed: true,
      },
    });
    renderPage();
    fireEvent.click(screen.getByRole("button", { name: /^clone$/i }));
    const [, options] = navigateMock.mock.calls.at(-1)!;
    const state = (options as { state: Record<string, unknown> }).state;
    expect(state.customerApprovalRequired).toBe(true);
    expect(state.customerReviewRequired).toBe(true);
    expect(Object.keys(state)).not.toContain("hasCustomerApproved");
    expect(Object.keys(state)).not.toContain("hasCustomerReviewed");
  });
});

describe("CsmChangeRequestDetailPage — blocking reason for the customer states", () => {
  it("shows 'Awaiting customer approval' in the header for a customer_approval CR with no pending approver", () => {
    mockQueryResult({ data: { ...BASE_CR, state: "customer_approval", customerApprovalRequired: true } });
    renderPage();
    expect(screen.getByText("Awaiting customer approval")).toBeInTheDocument();
  });

  it("shows 'Awaiting customer review' in the header for a customer_review CR", () => {
    mockQueryResult({ data: { ...BASE_CR, state: "customer_review", customerReviewRequired: true } });
    renderPage();
    expect(screen.getByText("Awaiting customer review")).toBeInTheDocument();
  });
});
