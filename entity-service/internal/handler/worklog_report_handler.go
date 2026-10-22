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
	"time"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/service"
)

// WorklogReportHandler serves the CS Worklogs daily and weekly reports.
type WorklogReportHandler struct {
	svc service.WorklogReportService
}

// NewWorklogReportHandler constructs a WorklogReportHandler.
func NewWorklogReportHandler(svc service.WorklogReportService) *WorklogReportHandler {
	return &WorklogReportHandler{svc: svc}
}

// worklogReportMaxSpanDays caps the range a single request may ask for.
// The two real reports span one day and seven; anything beyond a quarter
// is a mistake or a scrape, and the query reads every time card in range.
const worklogReportMaxSpanDays = 92

// GetWorklogReport handles
// GET /reports/worklogs?cadence=daily|weekly&from=YYYY-MM-DD&to=YYYY-MM-DD.
//
// Dates are plain calendar dates, not timestamps: time_card.work_date is a
// DATE, and the caller decides which calendar day it means. The scheduled
// task computes them in the report's own timezone -- the ServiceNow job
// runs at 16:00 UTC, which is 21:30 in Asia/Colombo, so "the week that
// just ended" is a question only the caller can answer.
//
// For the daily report `to` may be omitted, in which case it equals
// `from`.
func (h *WorklogReportHandler) GetWorklogReport(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	cadence := domain.WorklogReportCadence(q.Get("cadence"))
	switch cadence {
	case domain.WorklogReportCadenceDaily, domain.WorklogReportCadenceWeekly:
	case "":
		apierror.WriteJSON(w, http.StatusBadRequest, "cadence is required and must be daily or weekly")
		return
	default:
		apierror.WriteJSON(w, http.StatusBadRequest, "cadence must be daily or weekly")
		return
	}

	from, ok := parseWorklogDate(w, q.Get("from"), "from")
	if !ok {
		return
	}
	to := from
	if raw := q.Get("to"); raw != "" {
		if to, ok = parseWorklogDate(w, raw, "to"); !ok {
			return
		}
	}
	if to.Before(from) {
		apierror.WriteJSON(w, http.StatusBadRequest, "to must not be before from")
		return
	}
	// Inclusive: from and to are both reported on, so 92 days is a gap
	// of 91 between the endpoints. Comparing the gap against 92 let 93
	// days through while the message and the spec both said 92.
	if to.Sub(from) >= worklogReportMaxSpanDays*24*time.Hour {
		apierror.WriteJSON(w, http.StatusBadRequest, "the range must not exceed 92 days")
		return
	}

	report, err := h.svc.BuildReport(r.Context(), domain.WorklogReportRequest{
		Cadence: cadence,
		From:    from,
		To:      to,
	})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(report)
}

func parseWorklogDate(w http.ResponseWriter, raw, field string) (time.Time, bool) {
	if raw == "" {
		apierror.WriteJSON(w, http.StatusBadRequest, field+" is required, as YYYY-MM-DD")
		return time.Time{}, false
	}
	d, err := time.Parse("2006-01-02", raw)
	if err != nil {
		apierror.WriteJSON(w, http.StatusBadRequest, field+" must be a date in YYYY-MM-DD form")
		return time.Time{}, false
	}
	return d, true
}
