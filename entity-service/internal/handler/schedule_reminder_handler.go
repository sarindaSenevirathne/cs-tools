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
	"net/http"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/service"
)

// ScheduleReminderHandler serves the Team Schedule reminders' bookkeeping to
// csm-scheduled-tasks. The service admits its service credential only.
type ScheduleReminderHandler struct {
	svc service.ScheduleReminderService
}

// NewScheduleReminderHandler constructs a ScheduleReminderHandler.
func NewScheduleReminderHandler(svc service.ScheduleReminderService) *ScheduleReminderHandler {
	return &ScheduleReminderHandler{svc: svc}
}

// GetReminder handles GET /team-schedule/reminders/{family}/{kind}/{periodStart}:
// 404 when nothing was sent for the period yet.
func (h *ScheduleReminderHandler) GetReminder(w http.ResponseWriter, r *http.Request) {
	rec, err := h.svc.Get(r.Context(), r.PathValue("family"), r.PathValue("kind"), r.PathValue("periodStart"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	if rec == nil {
		apierror.WriteJSON(w, http.StatusNotFound, "nothing was sent for this period")
		return
	}
	writeScheduleJSON(w, http.StatusOK, rec)
}

// RecordReminder handles PUT /team-schedule/reminders/{family}/{kind}/{periodStart}.
func (h *ScheduleReminderHandler) RecordReminder(w http.ResponseWriter, r *http.Request) {
	var req domain.RecordScheduleReminderRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	rec, err := h.svc.Record(r.Context(), r.PathValue("family"), r.PathValue("kind"), r.PathValue("periodStart"), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeScheduleJSON(w, http.StatusOK, rec)
}

// GetRotaAdmins handles GET /team-schedule/rota-admins/{family}.
func (h *ScheduleReminderHandler) GetRotaAdmins(w http.ResponseWriter, r *http.Request) {
	admins, err := h.svc.RotaAdmins(r.Context(), r.PathValue("family"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeScheduleJSON(w, http.StatusOK, map[string]any{"admins": admins})
}

// RotaReminderSettingsHandler serves each family's rota reminder settings:
// read by csm-scheduled-tasks and the Rota Access tab, changed on that tab.
type RotaReminderSettingsHandler struct {
	svc service.RotaReminderSettingsService
}

// NewRotaReminderSettingsHandler constructs a RotaReminderSettingsHandler.
func NewRotaReminderSettingsHandler(svc service.RotaReminderSettingsService) *RotaReminderSettingsHandler {
	return &RotaReminderSettingsHandler{svc: svc}
}

// Get handles GET /team-schedule/reminder-settings/{family}: 404 when none
// were saved.
func (h *RotaReminderSettingsHandler) Get(w http.ResponseWriter, r *http.Request) {
	s, err := h.svc.Get(r.Context(), r.PathValue("family"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	if s == nil {
		apierror.WriteJSON(w, http.StatusNotFound, "no rota reminder settings saved for this family")
		return
	}
	writeScheduleJSON(w, http.StatusOK, s)
}

// Save handles PUT /team-schedule/reminder-settings/{family}.
func (h *RotaReminderSettingsHandler) Save(w http.ResponseWriter, r *http.Request) {
	var req domain.SaveRotaReminderSettingsRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	s, err := h.svc.Save(r.Context(), r.PathValue("family"), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeScheduleJSON(w, http.StatusOK, s)
}

// RotaReminderRequestHandler serves "Send now": a person asks for a rota
// reminder, the scheduled task claims it and reports how it went.
type RotaReminderRequestHandler struct {
	svc service.RotaReminderRequestService
}

// NewRotaReminderRequestHandler constructs a RotaReminderRequestHandler.
func NewRotaReminderRequestHandler(svc service.RotaReminderRequestService) *RotaReminderRequestHandler {
	return &RotaReminderRequestHandler{svc: svc}
}

// Create handles POST /team-schedule/reminder-requests/{family}.
func (h *RotaReminderRequestHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateRotaReminderRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	out, err := h.svc.Create(r.Context(), r.PathValue("family"), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeScheduleJSON(w, http.StatusCreated, out)
}

// List handles GET /team-schedule/reminder-requests/{family}.
func (h *RotaReminderRequestHandler) List(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.List(r.Context(), r.PathValue("family"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeScheduleJSON(w, http.StatusOK, map[string]any{"requests": out})
}

// Claim handles POST /team-schedule/reminder-requests/{family}/claim: 404
// when nothing is waiting.
func (h *RotaReminderRequestHandler) Claim(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Claim(r.Context(), r.PathValue("family"))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	if out == nil {
		apierror.WriteJSON(w, http.StatusNotFound, "no rota reminder request is waiting")
		return
	}
	writeScheduleJSON(w, http.StatusOK, out)
}

// Report handles PUT /team-schedule/reminder-requests/{family}/{id}/result.
func (h *RotaReminderRequestHandler) Report(w http.ResponseWriter, r *http.Request) {
	var req domain.ReportRotaReminderRequest
	if !decodeRequest(w, r, &req) {
		return
	}
	out, err := h.svc.Report(r.Context(), r.PathValue("family"), r.PathValue("id"), req)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeScheduleJSON(w, http.StatusOK, out)
}
