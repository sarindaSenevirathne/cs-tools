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
	"fmt"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
)

// validPendingVerificationAddedReason mirrors the DB's
// pending_verification_added_reason_enum; add an entry here whenever a new
// enum value is added there.
var validPendingVerificationAddedReason = map[domain.PendingVerificationAddedReason]bool{
	domain.PendingVerificationAddedReasonAutoClosed: true,
	domain.PendingVerificationAddedReasonManual:     true,
}

// validPendingVerificationPreviousStatus mirrors the DB's
// pending_verification_previous_status_enum.
var validPendingVerificationPreviousStatus = map[domain.PendingVerificationPreviousStatus]bool{
	domain.PendingVerificationPreviousStatusSolutionProposed: true,
	domain.PendingVerificationPreviousStatusAwaitingInfo:     true,
}

// validPendingVerificationRecordType is the six work_item_type_enum values
// this feature is scoped to (§2 of the requirements doc) — deliberately
// narrower than every value work_item_type_enum actually has (e.g. INCIDENT,
// PROBLEM are unrelated features), so a caller can't filter this list by a
// record type the feature was never meant to cover.
var validPendingVerificationRecordType = map[string]bool{
	"CASE":                     true,
	"ENGAGEMENT":               true,
	"SECURITY_REPORT_ANALYSIS": true,
	"SERVICE_REQUEST":          true,
	"ANNOUNCEMENT":             true,
	"CHANGE_REQUEST":           true,
}

// addedBy/verifiedBy always come from resolveCallerEmail (account_contact_service.go's
// shared helper) — this service has no other identity layer, and a
// client-supplied value in the request body would let one caller attribute
// an entry to someone else.

type pendingVerificationService struct {
	repo repository.PendingVerificationRepository
}

// NewPendingVerificationService constructs a PendingVerificationService
// backed by Postgres.
func NewPendingVerificationService(repo repository.PendingVerificationRepository) PendingVerificationService {
	return &pendingVerificationService{repo: repo}
}

// CreatePendingVerification implements PendingVerificationService.
func (s *pendingVerificationService) CreatePendingVerification(ctx context.Context, req domain.CreatePendingVerificationRequest) (domain.CreatePendingVerificationResponse, error) {
	if err := validateUUIDs("workItemId", []string{req.WorkItemID}); err != nil {
		return domain.CreatePendingVerificationResponse{}, err
	}
	if !validPendingVerificationAddedReason[req.AddedReason] {
		return domain.CreatePendingVerificationResponse{}, &apierror.ValidationError{Msg: fmt.Sprintf("addedReason must be one of auto-closed/manual, got %q", req.AddedReason)}
	}

	// previousStatus is required iff addedReason is auto-closed — mirrors
	// the table's own chk_previous_status_matches_reason CHECK constraint,
	// checked here too so the caller gets a clear 400 instead of a raw
	// constraint-violation error.
	isAutoClosed := req.AddedReason == domain.PendingVerificationAddedReasonAutoClosed
	if isAutoClosed && req.PreviousStatus == nil {
		return domain.CreatePendingVerificationResponse{}, &apierror.ValidationError{Msg: "previousStatus is required when addedReason is auto-closed"}
	}
	if !isAutoClosed && req.PreviousStatus != nil {
		return domain.CreatePendingVerificationResponse{}, &apierror.ValidationError{Msg: "previousStatus must not be set when addedReason is manual"}
	}
	if req.PreviousStatus != nil && !validPendingVerificationPreviousStatus[*req.PreviousStatus] {
		return domain.CreatePendingVerificationResponse{}, &apierror.ValidationError{Msg: fmt.Sprintf("previousStatus must be one of Solution Proposed/Awaiting Info, got %q", *req.PreviousStatus)}
	}

	email, err := resolveCallerEmail(ctx)
	if err != nil {
		return domain.CreatePendingVerificationResponse{}, err
	}
	req.AddedBy = email

	pv, err := s.repo.Create(ctx, req)
	if err != nil {
		return domain.CreatePendingVerificationResponse{}, err
	}

	return domain.CreatePendingVerificationResponse{
		Message:             "Added to Pending Verification",
		PendingVerification: pv,
	}, nil
}

// SearchPendingVerifications implements PendingVerificationService.
func (s *pendingVerificationService) SearchPendingVerifications(ctx context.Context, req domain.SearchPendingVerificationsRequest) (domain.SearchPendingVerificationsResponse, error) {
	if err := normalizePagination(&req.Pagination); err != nil {
		return domain.SearchPendingVerificationsResponse{}, err
	}
	if err := validateUUIDs("filters.projectId", []string{req.Filters.ProjectID}); err != nil {
		return domain.SearchPendingVerificationsResponse{}, err
	}
	if req.Filters.WorkItemID != nil {
		if err := validateUUIDs("filters.workItemId", []string{*req.Filters.WorkItemID}); err != nil {
			return domain.SearchPendingVerificationsResponse{}, err
		}
	}
	if err := validateSearchQuery(req.Filters.SearchQuery); err != nil {
		return domain.SearchPendingVerificationsResponse{}, err
	}
	for _, t := range req.Filters.WorkItemTypes {
		if !validPendingVerificationRecordType[t] {
			return domain.SearchPendingVerificationsResponse{}, &apierror.ValidationError{Msg: fmt.Sprintf("filters.workItemTypes contains an unsupported record type: %q", t)}
		}
	}

	return s.repo.Search(ctx, req)
}

// VerifyPendingVerification implements PendingVerificationService.
func (s *pendingVerificationService) VerifyPendingVerification(ctx context.Context, req domain.VerifyPendingVerificationRequest) (domain.VerifyPendingVerificationResponse, error) {
	if err := validateUUIDs("id", []string{req.ID}); err != nil {
		return domain.VerifyPendingVerificationResponse{}, err
	}

	email, err := resolveCallerEmail(ctx)
	if err != nil {
		return domain.VerifyPendingVerificationResponse{}, err
	}
	req.VerifiedBy = email

	pv, err := s.repo.Verify(ctx, req)
	if err != nil {
		return domain.VerifyPendingVerificationResponse{}, err
	}

	return domain.VerifyPendingVerificationResponse{
		Message:             fmt.Sprintf("Marked as verified — %s has been removed from your pending verification list.", pv.RecordNumber),
		PendingVerification: pv,
	}, nil
}
