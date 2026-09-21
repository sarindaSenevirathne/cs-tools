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
	"encoding/json"
	"net/http"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/service"
)

// PendingVerificationHandler handles HTTP requests for the
// pending-verifications resource — see domain.PendingVerification's own doc
// comment for what it's for.
type PendingVerificationHandler struct {
	svc service.PendingVerificationService
}

// NewPendingVerificationHandler constructs a PendingVerificationHandler with
// the given service.
func NewPendingVerificationHandler(svc service.PendingVerificationService) *PendingVerificationHandler {
	return &PendingVerificationHandler{svc: svc}
}

// CreatePendingVerification handles POST /pending-verifications.
func (h *PendingVerificationHandler) CreatePendingVerification(w http.ResponseWriter, r *http.Request) {
	var req domain.CreatePendingVerificationRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	resp, err := h.svc.CreatePendingVerification(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

// SearchPendingVerifications handles POST /pending-verifications/search.
func (h *PendingVerificationHandler) SearchPendingVerifications(w http.ResponseWriter, r *http.Request) {
	var req domain.SearchPendingVerificationsRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	resp, err := h.svc.SearchPendingVerifications(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// VerifyPendingVerification handles PATCH /pending-verifications/{id}. Takes
// no request body — id comes from the path, verifiedBy is resolved from the
// caller's own token by the service layer.
func (h *PendingVerificationHandler) VerifyPendingVerification(w http.ResponseWriter, r *http.Request) {
	req := domain.VerifyPendingVerificationRequest{ID: r.PathValue("id")}
	resp, err := h.svc.VerifyPendingVerification(r.Context(), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
