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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/wso2-open-operations/cs-tools/apps/csm-portal/backend/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/apps/csm-portal/backend/internal/middleware"
)

// changeRequestApprovalDecisionPayload is the strict shape accepted by
// DecideChangeRequestApproval: exactly one field, and Decision must be
// "approved" or "rejected".
type changeRequestApprovalDecisionPayload struct {
	Decision string `json:"decision"`
}

// entityChangeRequestClient abstracts the entity service change-request operations.
type entityChangeRequestClient interface {
	CreateChangeRequest(ctx context.Context, body []byte) ([]byte, error)
	SearchChangeRequests(ctx context.Context, body []byte) ([]byte, error)
	AggregateChangeRequests(ctx context.Context, body []byte) ([]byte, error)
	GetChangeRequest(ctx context.Context, id string) ([]byte, error)
	PatchChangeRequest(ctx context.Context, id string, body []byte) ([]byte, error)
	GetChangeRequestApprovals(ctx context.Context, id string) ([]byte, error)
	CreateComment(ctx context.Context, body []byte) ([]byte, error)
	SearchComments(ctx context.Context, body []byte) ([]byte, error)
	DecideChangeRequestApproval(ctx context.Context, id string, body []byte) ([]byte, error)
}

// ChangeRequestHandler handles HTTP requests for change-request operations.
type ChangeRequestHandler struct {
	entity entityChangeRequestClient
	// access backs the inline-image redaction in every read response — see
	// WithAccessGuard and CaseHandler's own field of the same name/reasoning.
	// nil fails that check closed (redacts), never open.
	access *AccessGuard
}

// NewChangeRequestHandler creates a ChangeRequestHandler backed by the given entity client.
func NewChangeRequestHandler(entity entityChangeRequestClient) *ChangeRequestHandler {
	return &ChangeRequestHandler{entity: entity}
}

// WithAccessGuard wires the same guard that authorises every route into this
// handler, so every read response can redact an embedded raw base64 inline
// image (see redactRawBase64Images's own doc comment) for a caller who
// lacks PermDownloadAttachment. Returns h for chaining at the construction
// site.
func (h *ChangeRequestHandler) WithAccessGuard(g *AccessGuard) *ChangeRequestHandler {
	h.access = g
	return h
}

// CreateChangeRequest handles POST /change-requests.
func (h *ChangeRequestHandler) CreateChangeRequest(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserInfoFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, ErrMsgUnauthorized)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, ErrMsgTooLarge)
			return
		}
		writeError(w, http.StatusBadRequest, errMsgReadBody)
		return
	}

	if !json.Valid(body) {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}

	// The change type decides the whole approval flow (Standard: none; Normal:
	// peer then CAB; Emergency: ECAB only), so a create without one of the
	// three is refused here with a message the form can show, rather than
	// forwarded to be rejected with a generic one.
	if msg := validateChangeRequestCreateType(body); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	if msg := validateChangeRequestCustomerGateFlags(body); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	result, err := h.entity.CreateChangeRequest(r.Context(), body)
	if err != nil {
		slog.ErrorContext(r.Context(), "entity CreateChangeRequest failed", "userID", user.UserID, "err", err)
		mapUpstreamErrorGeneric(w, err, "Failed to create change request.")
		return
	}

	writeJSON(w, http.StatusCreated, result)
}

// changeRequestCreatableTypes are the only types a change request may be
// created with (the entity service enforces the same set).
var changeRequestCreatableTypes = []string{"standard", "normal", "emergency"}

// validateChangeRequestCreateType returns a user-facing message when body does
// not carry a valid create-time "type" (standard, normal or emergency), or ""
// when it does. A body that is not a JSON object is left for the upstream to
// reject.
func validateChangeRequestCreateType(body []byte) string {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	const required = "type is required: a change request must be one of standard, normal or emergency"
	raw, ok := payload["type"]
	if !ok {
		return required
	}
	var typ string
	if err := json.Unmarshal(raw, &typ); err != nil || typ == "" {
		return required
	}
	for _, allowed := range changeRequestCreatableTypes {
		if typ == allowed {
			return ""
		}
	}
	return "type is not allowed: a change request must be one of standard, normal or emergency"
}

// changeRequestCustomerGateFields are the creation form's two checkboxes,
// "Customer Approval" and "Customer Review": whether the change needs the
// customer's approval before it is scheduled / the customer's review before it
// is closed. They are forwarded to the entity service as-is; the only thing
// checked here is their type, so a stray string or null is refused with a
// message the form can show instead of a generic upstream decode failure.
var changeRequestCustomerGateFields = []string{"customerApprovalRequired", "customerReviewRequired"}

// validateChangeRequestCustomerGateFlags returns a user-facing message when
// body carries one of the customer gate checkboxes with a value that is not a
// JSON boolean, or "" when it carries none or only booleans. A body that is not
// a JSON object is left for the upstream to reject.
func validateChangeRequestCustomerGateFlags(body []byte) string {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	for _, field := range changeRequestCustomerGateFields {
		raw, ok := payload[field]
		if !ok {
			continue
		}
		if v := string(bytes.TrimSpace(raw)); v != "true" && v != "false" {
			return field + " must be a boolean (true or false)"
		}
	}
	return ""
}

// mapApprovalDecisionError is mapUpstreamErrorGeneric, except a 403 that
// carries the entity service's own reason is shown to the caller. A refusal to
// decide ("the creator of a change request cannot approve it", "members of an
// SRE team cannot give peer approval") is only useful if the approver can read
// why; every other failure keeps the generic mapping.
func mapApprovalDecisionError(w http.ResponseWriter, err error, fallbackMsg string) {
	var apiErr *apierror.Error
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusForbidden {
		if msg := upstreamErrorMessageStrict(apiErr.Body, ""); msg != "" {
			writeError(w, http.StatusForbidden, msg)
			return
		}
	}
	mapUpstreamErrorGeneric(w, err, fallbackMsg)
}

// PatchChangeRequest handles PATCH /change-requests/{id}.
func (h *ChangeRequestHandler) PatchChangeRequest(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserInfoFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, ErrMsgUnauthorized)
		return
	}

	id := r.PathValue("id")
	if id == "" || !uuidRe.MatchString(id) {
		writeError(w, http.StatusBadRequest, ErrMsgInvalidUUID)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, ErrMsgTooLarge)
			return
		}
		writeError(w, http.StatusBadRequest, errMsgReadBody)
		return
	}

	if len(body) > 0 && !json.Valid(body) {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}

	if msg := validateChangeRequestCustomerGateFlags(body); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	result, err := h.entity.PatchChangeRequest(r.Context(), id, body)
	if err != nil {
		slog.ErrorContext(r.Context(), "entity PatchChangeRequest failed", "userID", user.UserID, "id", id, "err", err)
		mapUpstreamError(w, err, "Failed to update change request.")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// GetChangeRequest handles GET /change-requests/{id}.
func (h *ChangeRequestHandler) GetChangeRequest(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserInfoFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, ErrMsgUnauthorized)
		return
	}

	id := r.PathValue("id")
	if id == "" || !uuidRe.MatchString(id) {
		writeError(w, http.StatusBadRequest, ErrMsgInvalidUUID)
		return
	}

	result, err := h.entity.GetChangeRequest(r.Context(), id)
	if err != nil {
		slog.ErrorContext(r.Context(), "entity GetChangeRequest failed", "userID", user.UserID, "id", id, "err", err)
		mapUpstreamErrorGeneric(w, err, "Failed to retrieve change request.")
		return
	}
	if shouldRedactInlineImages(h.access, user.Roles) {
		result = redactRawBase64Images(result)
	}

	writeJSON(w, http.StatusOK, result)
}

// GetChangeRequestApprovals handles GET /change-requests/{id}/approvals.
func (h *ChangeRequestHandler) GetChangeRequestApprovals(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserInfoFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, ErrMsgUnauthorized)
		return
	}

	id := r.PathValue("id")
	if id == "" || !uuidRe.MatchString(id) {
		writeError(w, http.StatusBadRequest, ErrMsgInvalidUUID)
		return
	}

	result, err := h.entity.GetChangeRequestApprovals(r.Context(), id)
	if err != nil {
		slog.ErrorContext(r.Context(), "entity GetChangeRequestApprovals failed", "userID", user.UserID, "id", id, "err", err)
		mapUpstreamErrorGeneric(w, err, "Failed to retrieve change request approvals.")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// CreateChangeRequestComment handles POST /change-requests/{id}/comments.
// Injects referenceId and referenceType into the payload and forwards to the entity
// service's reference-generic POST /comments.
func (h *ChangeRequestHandler) CreateChangeRequestComment(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserInfoFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, ErrMsgUnauthorized)
		return
	}

	id := r.PathValue("id")
	if id == "" || !uuidRe.MatchString(id) {
		writeError(w, http.StatusBadRequest, ErrMsgInvalidUUID)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxCommentBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, ErrMsgTooLarge)
			return
		}
		writeError(w, http.StatusBadRequest, errMsgReadBody)
		return
	}

	if !json.Valid(body) {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}

	if _, err := h.entity.GetChangeRequest(r.Context(), id); err != nil {
		slog.ErrorContext(r.Context(), "entity GetChangeRequest failed during comment guard", "userID", user.UserID, "id", id, "err", err)
		mapUpstreamErrorGeneric(w, err, "Failed to create change request comment.")
		return
	}

	newBody, err := injectReferenceFields(body, id, "change_request")
	if err != nil {
		writeError(w, http.StatusInternalServerError, ErrMsgInternal)
		return
	}

	result, err := h.entity.CreateComment(r.Context(), newBody)
	if err != nil {
		slog.ErrorContext(r.Context(), "entity CreateComment failed", "userID", user.UserID, "id", id, "err", err)
		mapUpstreamErrorGeneric(w, err, "Failed to create change request comment.")
		return
	}

	writeJSON(w, http.StatusCreated, result)
}

// SearchChangeRequestComments handles POST /change-requests/{id}/comments/search.
// Injects referenceId and referenceType into the payload and forwards to the entity
// service's reference-generic POST /comments/search.
func (h *ChangeRequestHandler) SearchChangeRequestComments(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserInfoFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, ErrMsgUnauthorized)
		return
	}

	id := r.PathValue("id")
	if id == "" || !uuidRe.MatchString(id) {
		writeError(w, http.StatusBadRequest, ErrMsgInvalidUUID)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, ErrMsgTooLarge)
			return
		}
		writeError(w, http.StatusBadRequest, errMsgReadBody)
		return
	}

	if !json.Valid(body) {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}

	newBody, err := injectReferenceFields(body, id, "change_request")
	if err != nil {
		writeError(w, http.StatusInternalServerError, ErrMsgInternal)
		return
	}

	result, err := h.entity.SearchComments(r.Context(), newBody)
	if err != nil {
		slog.ErrorContext(r.Context(), "entity SearchComments failed", "userID", user.UserID, "id", id, "err", err)
		mapUpstreamErrorGeneric(w, err, "Failed to search change request comments.")
		return
	}
	if shouldRedactInlineImages(h.access, user.Roles) {
		result = redactRawBase64Images(result)
	}

	writeJSON(w, http.StatusOK, result)
}

// DecideChangeRequestApproval handles POST /change-requests/{id}/approvals/decision. Any user
// with access to the change request may attempt a decision; ServiceNow itself enforces that
// only the caller's own pending approval can be acted on, so this is not a bypass-only endpoint.
func (h *ChangeRequestHandler) DecideChangeRequestApproval(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserInfoFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, ErrMsgUnauthorized)
		return
	}

	id := r.PathValue("id")
	if id == "" || !uuidRe.MatchString(id) {
		writeError(w, http.StatusBadRequest, ErrMsgInvalidUUID)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, ErrMsgTooLarge)
			return
		}
		writeError(w, http.StatusBadRequest, errMsgReadBody)
		return
	}

	var payload changeRequestApprovalDecisionPayload
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil || decoder.More() {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}
	if payload.Decision != "approved" && payload.Decision != "rejected" {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}

	result, err := h.entity.DecideChangeRequestApproval(r.Context(), id, body)
	if err != nil {
		slog.ErrorContext(r.Context(), "entity DecideChangeRequestApproval failed", "userID", user.UserID, "id", id, "err", err)
		mapApprovalDecisionError(w, err, "Failed to submit change request approval decision.")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// SearchChangeRequests handles POST /change-requests/search.
func (h *ChangeRequestHandler) SearchChangeRequests(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserInfoFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, ErrMsgUnauthorized)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, ErrMsgTooLarge)
			return
		}
		writeError(w, http.StatusBadRequest, errMsgReadBody)
		return
	}

	if !json.Valid(body) {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}

	result, err := h.entity.SearchChangeRequests(r.Context(), body)
	if err != nil {
		slog.ErrorContext(r.Context(), "entity SearchChangeRequests failed", "userID", user.UserID, "err", err)
		mapUpstreamErrorGeneric(w, err, "Failed to search change requests.")
		return
	}
	if shouldRedactInlineImages(h.access, user.Roles) {
		result = redactRawBase64Images(result)
	}

	writeJSON(w, http.StatusOK, result)
}

// AggregateChangeRequests handles POST /change-requests/aggregate.
// Server-side aggregation of change requests by a single field (e.g. state,
// assignmentGroup), capped to the top maxGroups buckets with the remainder
// folded into othersCount. The groupBy allowlist is validated upstream by
// the entity service; this layer only forwards the request and passes the
// response through as-is.
func (h *ChangeRequestHandler) AggregateChangeRequests(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserInfoFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, ErrMsgUnauthorized)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, ErrMsgTooLarge)
			return
		}
		writeError(w, http.StatusBadRequest, errMsgReadBody)
		return
	}

	if !json.Valid(body) {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}

	result, err := h.entity.AggregateChangeRequests(r.Context(), body)
	if err != nil {
		slog.ErrorContext(r.Context(), "entity AggregateChangeRequests failed", "userID", user.UserID, "err", err)
		mapUpstreamErrorGeneric(w, err, "Failed to aggregate change requests.")
		return
	}

	writeJSON(w, http.StatusOK, result)
}
