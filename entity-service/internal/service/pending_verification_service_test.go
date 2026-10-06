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

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
)

// fakePendingVerificationRepo is a minimal repository.PendingVerificationRepository
// -- each method is a func field, nil-checked, so a test only wires up the
// ones it actually exercises; calling an unwired method panics loudly rather
// than silently returning a zero value, same convention as this package's
// other fake repos.
type fakePendingVerificationRepo struct {
	create          func(ctx context.Context, req domain.CreatePendingVerificationRequest) (domain.PendingVerification, error)
	getWorkItemType func(ctx context.Context, workItemID string) (string, error)
	search          func(ctx context.Context, req domain.SearchPendingVerificationsRequest) (domain.SearchPendingVerificationsResponse, error)
	verify          func(ctx context.Context, req domain.VerifyPendingVerificationRequest) (domain.PendingVerification, error)
}

func (f *fakePendingVerificationRepo) Create(ctx context.Context, req domain.CreatePendingVerificationRequest) (domain.PendingVerification, error) {
	if f.create == nil {
		panic("fakePendingVerificationRepo.Create: not expected to be called by this test")
	}
	return f.create(ctx, req)
}

func (f *fakePendingVerificationRepo) GetWorkItemType(ctx context.Context, workItemID string) (string, error) {
	if f.getWorkItemType == nil {
		panic("fakePendingVerificationRepo.GetWorkItemType: not expected to be called by this test")
	}
	return f.getWorkItemType(ctx, workItemID)
}

func (f *fakePendingVerificationRepo) Search(ctx context.Context, req domain.SearchPendingVerificationsRequest) (domain.SearchPendingVerificationsResponse, error) {
	if f.search == nil {
		panic("fakePendingVerificationRepo.Search: not expected to be called by this test")
	}
	return f.search(ctx, req)
}

func (f *fakePendingVerificationRepo) Verify(ctx context.Context, req domain.VerifyPendingVerificationRequest) (domain.PendingVerification, error) {
	if f.verify == nil {
		panic("fakePendingVerificationRepo.Verify: not expected to be called by this test")
	}
	return f.verify(ctx, req)
}

const (
	pvTestWorkItemID  = "6fa0b42d-1bfa-a694-a002-c9d3604bcb77"
	pvTestEntryID     = "d121847c-58a4-4266-bf83-cffba008e1de"
	pvTestCallerEmail = "jane.doe@example.com"
)

func assertValidationError(t *testing.T, err error) {
	t.Helper()
	var ve *apierror.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *apierror.ValidationError, got %T: %v", err, err)
	}
}

// --- CreatePendingVerification ---------------------------------------------

func TestPendingVerificationService_Create_InvalidWorkItemID(t *testing.T) {
	svc := NewPendingVerificationService(&fakePendingVerificationRepo{})
	_, err := svc.CreatePendingVerification(context.Background(), domain.CreatePendingVerificationRequest{
		WorkItemID:  "not-a-uuid",
		AddedReason: domain.PendingVerificationAddedReasonManual,
	})
	assertValidationError(t, err)
}

func TestPendingVerificationService_Create_InvalidAddedReason(t *testing.T) {
	svc := NewPendingVerificationService(&fakePendingVerificationRepo{})
	_, err := svc.CreatePendingVerification(context.Background(), domain.CreatePendingVerificationRequest{
		WorkItemID:  pvTestWorkItemID,
		AddedReason: "SOMETHING_ELSE",
	})
	assertValidationError(t, err)
}

func TestPendingVerificationService_Create_AutoClosedRequiresPreviousStatus(t *testing.T) {
	svc := NewPendingVerificationService(&fakePendingVerificationRepo{})
	_, err := svc.CreatePendingVerification(context.Background(), domain.CreatePendingVerificationRequest{
		WorkItemID:  pvTestWorkItemID,
		AddedReason: domain.PendingVerificationAddedReasonAutoClosed,
		// PreviousStatus deliberately omitted.
	})
	assertValidationError(t, err)
}

func TestPendingVerificationService_Create_ManualRejectsPreviousStatus(t *testing.T) {
	status := domain.PendingVerificationPreviousStatusAwaitingInfo
	svc := NewPendingVerificationService(&fakePendingVerificationRepo{})
	_, err := svc.CreatePendingVerification(context.Background(), domain.CreatePendingVerificationRequest{
		WorkItemID:     pvTestWorkItemID,
		AddedReason:    domain.PendingVerificationAddedReasonManual,
		PreviousStatus: &status,
	})
	assertValidationError(t, err)
}

func TestPendingVerificationService_Create_InvalidPreviousStatusValue(t *testing.T) {
	bad := domain.PendingVerificationPreviousStatus("CLOSED")
	svc := NewPendingVerificationService(&fakePendingVerificationRepo{})
	_, err := svc.CreatePendingVerification(context.Background(), domain.CreatePendingVerificationRequest{
		WorkItemID:     pvTestWorkItemID,
		AddedReason:    domain.PendingVerificationAddedReasonAutoClosed,
		PreviousStatus: &bad,
	})
	assertValidationError(t, err)
}

// TestPendingVerificationService_Create_RejectsNonCase locks in the
// 2026-10-06 Case-only scope-down: CreatePendingVerification must reject a
// workItemId whose own work_item.type isn't CASE, before ever reaching the
// repository's Create/INSERT.
func TestPendingVerificationService_Create_RejectsNonCase(t *testing.T) {
	repo := &fakePendingVerificationRepo{
		getWorkItemType: func(context.Context, string) (string, error) {
			return "ENGAGEMENT", nil
		},
		create: func(context.Context, domain.CreatePendingVerificationRequest) (domain.PendingVerification, error) {
			t.Fatal("Create must not be called for a non-Case work item")
			return domain.PendingVerification{}, nil
		},
	}
	svc := NewPendingVerificationService(repo)
	ctx := contextWithUserIDToken(fakeJWTWithEmail(t, pvTestCallerEmail))
	_, err := svc.CreatePendingVerification(ctx, domain.CreatePendingVerificationRequest{
		WorkItemID:  pvTestWorkItemID,
		AddedReason: domain.PendingVerificationAddedReasonManual,
	})
	assertValidationError(t, err)
}

// TestPendingVerificationService_Create_PropagatesWorkItemLookupError
// confirms a work item the caller can't see (RLS-filtered to zero rows,
// surfaced by the repo as NotFoundError/ValidationError) is propagated
// as-is, never masked as a different error.
func TestPendingVerificationService_Create_PropagatesWorkItemLookupError(t *testing.T) {
	wantErr := &apierror.ValidationError{Msg: "workItemId does not reference an existing record"}
	repo := &fakePendingVerificationRepo{
		getWorkItemType: func(context.Context, string) (string, error) {
			return "", wantErr
		},
	}
	svc := NewPendingVerificationService(repo)
	ctx := contextWithUserIDToken(fakeJWTWithEmail(t, pvTestCallerEmail))
	_, err := svc.CreatePendingVerification(ctx, domain.CreatePendingVerificationRequest{
		WorkItemID:  pvTestWorkItemID,
		AddedReason: domain.PendingVerificationAddedReasonManual,
	})
	if !errors.Is(err, error(wantErr)) {
		t.Fatalf("err = %v, want the exact error GetWorkItemType returned (%v), propagated unchanged", err, wantErr)
	}
}

func TestPendingVerificationService_Create_RequiresCallerToken(t *testing.T) {
	repo := &fakePendingVerificationRepo{
		getWorkItemType: func(context.Context, string) (string, error) { return "CASE", nil },
	}
	svc := NewPendingVerificationService(repo)
	ctx := contextWithUserIDToken("") // no x-user-id-token header
	_, err := svc.CreatePendingVerification(ctx, domain.CreatePendingVerificationRequest{
		WorkItemID:  pvTestWorkItemID,
		AddedReason: domain.PendingVerificationAddedReasonManual,
	})
	var unauthorized *apierror.UnauthorizedError
	if !errors.As(err, &unauthorized) {
		t.Fatalf("expected *apierror.UnauthorizedError, got %T: %v", err, err)
	}
}

// TestPendingVerificationService_Create_HappyPath confirms: the work item
// type check passes for CASE, addedBy is resolved from the caller's own
// token (never trusted from the request), and the repo's response is
// returned unchanged inside the standard success envelope.
func TestPendingVerificationService_Create_HappyPath(t *testing.T) {
	var gotReq domain.CreatePendingVerificationRequest
	wantPV := domain.PendingVerification{ID: pvTestEntryID, WorkItemID: pvTestWorkItemID, RecordNumber: "CS0441241"}
	repo := &fakePendingVerificationRepo{
		getWorkItemType: func(_ context.Context, workItemID string) (string, error) {
			if workItemID != pvTestWorkItemID {
				t.Fatalf("GetWorkItemType called with %q, want %q", workItemID, pvTestWorkItemID)
			}
			return "CASE", nil
		},
		create: func(_ context.Context, req domain.CreatePendingVerificationRequest) (domain.PendingVerification, error) {
			gotReq = req
			return wantPV, nil
		},
	}
	svc := NewPendingVerificationService(repo)
	ctx := contextWithUserIDToken(fakeJWTWithEmail(t, pvTestCallerEmail))

	resp, err := svc.CreatePendingVerification(ctx, domain.CreatePendingVerificationRequest{
		WorkItemID:  pvTestWorkItemID,
		AddedReason: domain.PendingVerificationAddedReasonManual,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotReq.AddedBy != pvTestCallerEmail {
		t.Fatalf("Create called with AddedBy=%q, want the caller's own email %q -- never client-supplied", gotReq.AddedBy, pvTestCallerEmail)
	}
	if resp.Message != "Added to Pending Verification" {
		t.Fatalf("Message = %q", resp.Message)
	}
	if resp.PendingVerification != wantPV {
		t.Fatalf("PendingVerification = %+v, want %+v", resp.PendingVerification, wantPV)
	}
}

// --- SearchPendingVerifications ---------------------------------------------

func TestPendingVerificationService_Search_RequiresProjectID(t *testing.T) {
	svc := NewPendingVerificationService(&fakePendingVerificationRepo{})
	_, err := svc.SearchPendingVerifications(context.Background(), domain.SearchPendingVerificationsRequest{
		Filters: domain.PendingVerificationSearchFilters{ProjectID: "not-a-uuid"},
	})
	assertValidationError(t, err)
}

func TestPendingVerificationService_Search_InvalidWorkItemIDFilter(t *testing.T) {
	badID := "not-a-uuid"
	svc := NewPendingVerificationService(&fakePendingVerificationRepo{})
	_, err := svc.SearchPendingVerifications(context.Background(), domain.SearchPendingVerificationsRequest{
		Filters: domain.PendingVerificationSearchFilters{ProjectID: pvTestWorkItemID, WorkItemID: &badID},
	})
	assertValidationError(t, err)
}

func TestPendingVerificationService_Search_SearchQueryTooLong(t *testing.T) {
	long := make([]byte, 201)
	for i := range long {
		long[i] = 'a'
	}
	svc := NewPendingVerificationService(&fakePendingVerificationRepo{})
	_, err := svc.SearchPendingVerifications(context.Background(), domain.SearchPendingVerificationsRequest{
		Filters: domain.PendingVerificationSearchFilters{ProjectID: pvTestWorkItemID, SearchQuery: string(long)},
	})
	assertValidationError(t, err)
}

func TestPendingVerificationService_Search_UnsupportedWorkItemType(t *testing.T) {
	svc := NewPendingVerificationService(&fakePendingVerificationRepo{})
	_, err := svc.SearchPendingVerifications(context.Background(), domain.SearchPendingVerificationsRequest{
		Filters: domain.PendingVerificationSearchFilters{ProjectID: pvTestWorkItemID, WorkItemTypes: []string{"INCIDENT"}},
	})
	assertValidationError(t, err)
}

// TestPendingVerificationService_Search_InvalidAddedReason locks in the new
// addedReason filter's own validation, added alongside the Support page's
// Auto-closed/Manual stat boxes.
func TestPendingVerificationService_Search_InvalidAddedReason(t *testing.T) {
	bad := domain.PendingVerificationAddedReason("SOMETHING_ELSE")
	svc := NewPendingVerificationService(&fakePendingVerificationRepo{})
	_, err := svc.SearchPendingVerifications(context.Background(), domain.SearchPendingVerificationsRequest{
		Filters: domain.PendingVerificationSearchFilters{ProjectID: pvTestWorkItemID, AddedReason: &bad},
	})
	assertValidationError(t, err)
}

func TestPendingVerificationService_Search_LimitExceedsMax(t *testing.T) {
	svc := NewPendingVerificationService(&fakePendingVerificationRepo{})
	_, err := svc.SearchPendingVerifications(context.Background(), domain.SearchPendingVerificationsRequest{
		Pagination: domain.Pagination{Limit: 500},
		Filters:    domain.PendingVerificationSearchFilters{ProjectID: pvTestWorkItemID},
	})
	assertValidationError(t, err)
}

// TestPendingVerificationService_Search_HappyPath confirms a valid request,
// including the new addedReason filter, reaches the repository unchanged and
// its response is returned as-is.
func TestPendingVerificationService_Search_HappyPath(t *testing.T) {
	autoClosed := domain.PendingVerificationAddedReasonAutoClosed
	var gotReq domain.SearchPendingVerificationsRequest
	wantResp := domain.SearchPendingVerificationsResponse{Total: 1}
	repo := &fakePendingVerificationRepo{
		search: func(_ context.Context, req domain.SearchPendingVerificationsRequest) (domain.SearchPendingVerificationsResponse, error) {
			gotReq = req
			return wantResp, nil
		},
	}
	svc := NewPendingVerificationService(repo)

	resp, err := svc.SearchPendingVerifications(context.Background(), domain.SearchPendingVerificationsRequest{
		Filters: domain.PendingVerificationSearchFilters{ProjectID: pvTestWorkItemID, AddedReason: &autoClosed},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotReq.Filters.AddedReason == nil || *gotReq.Filters.AddedReason != autoClosed {
		t.Fatalf("Filters.AddedReason = %v, want %q", gotReq.Filters.AddedReason, autoClosed)
	}
	if gotReq.Pagination.Limit != defaultLimit {
		t.Fatalf("Pagination.Limit = %d, want the normalized default %d", gotReq.Pagination.Limit, defaultLimit)
	}
	if resp.Total != wantResp.Total {
		t.Fatalf("response.Total = %d, want %d -- the repo's response must pass through unchanged", resp.Total, wantResp.Total)
	}
}

// --- VerifyPendingVerification -----------------------------------------------

func TestPendingVerificationService_Verify_InvalidID(t *testing.T) {
	svc := NewPendingVerificationService(&fakePendingVerificationRepo{})
	_, err := svc.VerifyPendingVerification(context.Background(), domain.VerifyPendingVerificationRequest{ID: "not-a-uuid"})
	assertValidationError(t, err)
}

func TestPendingVerificationService_Verify_RequiresCallerToken(t *testing.T) {
	svc := NewPendingVerificationService(&fakePendingVerificationRepo{})
	ctx := contextWithUserIDToken("")
	_, err := svc.VerifyPendingVerification(ctx, domain.VerifyPendingVerificationRequest{ID: pvTestEntryID})
	var unauthorized *apierror.UnauthorizedError
	if !errors.As(err, &unauthorized) {
		t.Fatalf("expected *apierror.UnauthorizedError, got %T: %v", err, err)
	}
}

// TestPendingVerificationService_Verify_HappyPath confirms verifiedBy is
// resolved from the caller's own token (never trusted from the request) and
// the success message embeds the verified record's own number.
func TestPendingVerificationService_Verify_HappyPath(t *testing.T) {
	var gotReq domain.VerifyPendingVerificationRequest
	wantPV := domain.PendingVerification{ID: pvTestEntryID, RecordNumber: "CS0441241"}
	repo := &fakePendingVerificationRepo{
		verify: func(_ context.Context, req domain.VerifyPendingVerificationRequest) (domain.PendingVerification, error) {
			gotReq = req
			return wantPV, nil
		},
	}
	svc := NewPendingVerificationService(repo)
	ctx := contextWithUserIDToken(fakeJWTWithEmail(t, pvTestCallerEmail))

	resp, err := svc.VerifyPendingVerification(ctx, domain.VerifyPendingVerificationRequest{ID: pvTestEntryID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotReq.VerifiedBy != pvTestCallerEmail {
		t.Fatalf("Verify called with VerifiedBy=%q, want the caller's own email %q -- never client-supplied", gotReq.VerifiedBy, pvTestCallerEmail)
	}
	if resp.Message != "Marked as verified — CS0441241 has been removed from your pending verification list." {
		t.Fatalf("Message = %q", resp.Message)
	}
	if resp.PendingVerification != wantPV {
		t.Fatalf("PendingVerification = %+v, want %+v", resp.PendingVerification, wantPV)
	}
}

// TestPendingVerificationService_Verify_PropagatesNotFound confirms a
// missing-or-already-verified entry (the repo's NotFoundError) is
// propagated as-is, not masked.
func TestPendingVerificationService_Verify_PropagatesNotFound(t *testing.T) {
	wantErr := &apierror.NotFoundError{Msg: "pending_verification not found, or already verified"}
	repo := &fakePendingVerificationRepo{
		verify: func(context.Context, domain.VerifyPendingVerificationRequest) (domain.PendingVerification, error) {
			return domain.PendingVerification{}, wantErr
		},
	}
	svc := NewPendingVerificationService(repo)
	ctx := contextWithUserIDToken(fakeJWTWithEmail(t, pvTestCallerEmail))

	_, err := svc.VerifyPendingVerification(ctx, domain.VerifyPendingVerificationRequest{ID: pvTestEntryID})
	var notFound *apierror.NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("expected *apierror.NotFoundError, got %T: %v", err, err)
	}
}
