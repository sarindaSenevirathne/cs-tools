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

package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/wso2-open-operations/cs-tools/apps/customer-portal/backend-v2/internal/dto"
	"github.com/wso2-open-operations/cs-tools/apps/customer-portal/backend-v2/internal/entity"
	"github.com/wso2-open-operations/cs-tools/apps/customer-portal/backend-v2/internal/middleware"
)

// entityPendingVerificationClient abstracts the entity-service
// pending-verifications operations used by PendingVerificationHandler.
type entityPendingVerificationClient interface {
	CreatePendingVerification(ctx context.Context, req entity.CreatePendingVerificationRequest) (entity.CreatePendingVerificationResponse, error)
	SearchPendingVerifications(ctx context.Context, req entity.SearchPendingVerificationsRequest) (entity.SearchPendingVerificationsResponse, error)
	VerifyPendingVerification(ctx context.Context, id string) (entity.VerifyPendingVerificationResponse, error)
}

// PendingVerificationHandler handles HTTP requests for the
// pending-verifications resource backing the customer portal's Verification
// feature.
type PendingVerificationHandler struct {
	entity entityPendingVerificationClient
}

// NewPendingVerificationHandler creates a PendingVerificationHandler backed
// by the given entity client.
func NewPendingVerificationHandler(entity entityPendingVerificationClient) *PendingVerificationHandler {
	return &PendingVerificationHandler{entity: entity}
}

// CreatePendingVerification handles POST /pending-verifications.
func (h *PendingVerificationHandler) CreatePendingVerification(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserInfoFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, ErrMsgUnauthorized)
		return
	}

	body, ok := readJSONBody(w, r)
	if !ok {
		return
	}

	var req dto.PendingVerificationCreateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}
	if !uuidRe.MatchString(req.WorkItemID) {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}

	result, err := h.entity.CreatePendingVerification(r.Context(), dto.BuildEntityCreatePendingVerificationRequest(req))
	if err != nil {
		slog.ErrorContext(r.Context(), "entity CreatePendingVerification failed", "userID", user.UserID, "err", summarizeErr(err))
		mapUpstreamError(w, err, "Failed to add record to the pending verification list.")
		return
	}

	writeJSONValue(w, http.StatusCreated, dto.MapPendingVerificationCreate(result))
}

// SearchPendingVerifications handles POST /pending-verifications/search.
func (h *PendingVerificationHandler) SearchPendingVerifications(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserInfoFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, ErrMsgUnauthorized)
		return
	}

	body, ok := readJSONBody(w, r)
	if !ok {
		return
	}

	var req dto.PendingVerificationSearchRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}
	if !uuidRe.MatchString(req.ProjectID) {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}

	result, err := h.entity.SearchPendingVerifications(r.Context(), dto.BuildEntitySearchPendingVerificationsRequest(req))
	if err != nil {
		slog.ErrorContext(r.Context(), "entity SearchPendingVerifications failed", "userID", user.UserID, "err", summarizeErr(err))
		mapUpstreamError(w, err, "Failed to retrieve pending verification list.")
		return
	}

	writeJSONValue(w, http.StatusOK, dto.MapPendingVerificationSearch(result))
}

// VerifyPendingVerification handles PATCH /pending-verifications/{id}. Takes
// no request body -- id comes from the path, and entity-service resolves
// verifiedBy from the caller's own token.
func (h *PendingVerificationHandler) VerifyPendingVerification(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserInfoFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, ErrMsgUnauthorized)
		return
	}

	id := r.PathValue("id")
	if !uuidRe.MatchString(id) {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}

	result, err := h.entity.VerifyPendingVerification(r.Context(), id)
	if err != nil {
		slog.ErrorContext(r.Context(), "entity VerifyPendingVerification failed", "userID", user.UserID, "err", summarizeErr(err))
		mapUpstreamError(w, err, "Failed to mark record as verified.")
		return
	}

	writeJSONValue(w, http.StatusOK, dto.MapPendingVerificationVerify(result))
}
