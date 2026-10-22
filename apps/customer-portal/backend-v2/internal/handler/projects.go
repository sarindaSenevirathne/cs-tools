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
	"log/slog"
	"net/http"
	"strings"

	"github.com/wso2-open-operations/cs-tools/apps/customer-portal/backend-v2/internal/dto"
	"github.com/wso2-open-operations/cs-tools/apps/customer-portal/backend-v2/internal/entity"
	"github.com/wso2-open-operations/cs-tools/apps/customer-portal/backend-v2/internal/middleware"
)

// patchProjectTargetsClosureState reports whether a PATCH /projects/{id}
// body sets closureState -- entity-service's admin-only direct override of
// a project's overall closure status (see entity-service's own CLAUDE.md
// and UpdateProjectRequest's own doc comment, which this route must never
// forward it through). Reads the top-level keys as a token stream,
// case-insensitively, mirroring apps/csm-portal/backend's own
// projectUpdateTargetsClosureState: entity-service's JSON decoder matches a
// key to a struct field without regard to case, so a check that only looked
// for the one canonical spelling could be walked around by a differently-
// cased key. A body this can't make sense of (not a JSON object) is treated
// as not targeting it -- the strict DisallowUnknownFields decode right
// after this call rejects it on its own merits.
func patchProjectTargetsClosureState(body []byte) bool {
	dec := json.NewDecoder(bytes.NewReader(body))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return false
	}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return false
		}
		key, _ := keyTok.(string)
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return false
		}
		if strings.EqualFold(key, "closureState") {
			return true
		}
	}
	return false
}

// entityProjectClient abstracts the entity-service project operations used by ProjectHandler.
type entityProjectClient interface {
	SearchProjects(ctx context.Context, req entity.SearchProjectsRequest) (entity.SearchProjectsResponse, error)
	GetProject(ctx context.Context, id string) (entity.ProjectDetailsView, error)
	UpdateProject(ctx context.Context, id string, req entity.UpdateProjectRequest) (entity.UpdateProjectResponse, error)
}

// ProjectHandler handles HTTP requests for project operations.
type ProjectHandler struct {
	entity entityProjectClient
}

// NewProjectHandler creates a ProjectHandler backed by the given entity client.
func NewProjectHandler(entity entityProjectClient) *ProjectHandler {
	return &ProjectHandler{entity: entity}
}

// SearchProjects handles POST /projects/search.
func (h *ProjectHandler) SearchProjects(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserInfoFromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, ErrMsgUnauthorized)
		return
	}

	body, ok := readJSONBody(w, r)
	if !ok {
		return
	}

	var req dto.SearchProjectsRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}

	result, err := h.entity.SearchProjects(r.Context(), dto.BuildEntitySearchProjectsRequest(req))
	if err != nil {
		slog.ErrorContext(r.Context(), "entity SearchProjects failed", "userID", user.UserID, "err", summarizeErr(err))
		mapUpstreamError(w, err, "Failed to search projects.")
		return
	}

	writeJSONValue(w, http.StatusOK, dto.MapSearchProjects(result))
}

// GetProject handles GET /projects/{id}.
func (h *ProjectHandler) GetProject(w http.ResponseWriter, r *http.Request) {
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

	result, err := h.entity.GetProject(r.Context(), id)
	if err != nil {
		slog.ErrorContext(r.Context(), "entity GetProject failed", "userID", user.UserID, "projectID", id, "err", summarizeErr(err))
		mapUpstreamError(w, err, "Failed to retrieve project.")
		return
	}

	writeJSONValue(w, http.StatusOK, dto.MapProjectDetails(result))
}

// PatchProject handles PATCH /projects/{id}: the AI chat assistant (Novera)
// settings for a project. The route is gated on projects:update, which only
// customer_admin holds.
//
// EXACTLY ONE FIELD PER REQUEST. entity-service rejects anything else, and the
// Ballerina backend this replaces checked the same thing before calling it
// ("Only one field can be updated at a time."). Checking here turns a round
// trip that comes back 400 into an immediate, specific message.
//
// Both fields are pointers for that check to be possible at all: with plain
// bools, a request setting hasAgent=false is indistinguishable from one that
// omits it, so every request would look like it set both and none would pass.
func (h *ProjectHandler) PatchProject(w http.ResponseWriter, r *http.Request) {
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

	body, ok := readJSONBody(w, r)
	if !ok {
		return
	}

	// closureState gets its own check, ahead of the generic strict decode
	// below: the response is the same 400 any other unknown field gets, but
	// this one is entity-service's admin-only closure-state override (see
	// UpdateProjectRequest's own doc comment), which this route must never
	// be a path to -- so the attempt itself is worth a WARN with the
	// caller's id, not just a silent rejection indistinguishable from a
	// client typo.
	if patchProjectTargetsClosureState(body) {
		slog.WarnContext(r.Context(), "closureState rejected on customer-portal PATCH /projects/{id}",
			"userID", user.UserID, "projectID", id)
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}

	// Strict decode. The Ballerina payload this replaces is a CLOSED record
	// (record {| ... |}), so it rejects a property it does not declare, and
	// openapi.yaml says additionalProperties: false. Silently dropping an
	// unknown field would mean a caller misspelling hasKbReferences gets a 200
	// and no change.
	var req entity.UpdateProjectRequest
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return
	}
	if req.FieldCount() != 1 {
		writeError(w, http.StatusBadRequest, "Only one field can be updated at a time.")
		return
	}

	result, err := h.entity.UpdateProject(r.Context(), id, req)
	if err != nil {
		slog.ErrorContext(r.Context(), "entity UpdateProject failed",
			"userID", user.UserID, "projectID", id, "err", summarizeErr(err))
		mapUpstreamError(w, err, "Failed to update project settings.")
		return
	}

	// The Ballerina resource returns response.project, not the whole envelope.
	writeJSONValue(w, http.StatusOK, dto.MapUpdatedProject(result.Project))
}
