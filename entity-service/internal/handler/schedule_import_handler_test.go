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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/service"
)

// importOnlySchedule answers ImportRota and nothing else.
type importOnlySchedule struct {
	service.ScheduleService
	called bool
	dryRun bool
	code   string
}

func (s *importOnlySchedule) ImportRota(_ context.Context, code string, _ domain.ImportRotaRequest, dryRun bool) (domain.ImportRotaResponse, error) {
	s.called, s.code, s.dryRun = true, code, dryRun
	return domain.ImportRotaResponse{RotaCode: code, DryRun: dryRun}, nil
}

func importRequest(query, body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/team-schedule/rotas/SRE_SAAS/import"+query, strings.NewReader(body))
	r.SetPathValue("code", "SRE_SAAS")
	return r
}

const importBody = `{"schemaVersion":1,"family":"SRE","rotaCode":"SRE_SAAS","period":{"from":"2026-11-01","to":"2026-11-30"},"mode":"merge","assignments":[],"absences":[]}`

func TestImportRota_PassesTheDryRunAndAnswersByIt(t *testing.T) {
	for _, c := range []struct {
		query      string
		wantDryRun bool
		wantStatus int
	}{
		{"?dryRun=true", true, http.StatusOK},
		{"?dryRun=false", false, http.StatusCreated},
		{"", false, http.StatusCreated},
	} {
		svc := &importOnlySchedule{}
		w := httptest.NewRecorder()
		NewScheduleHandler(svc).ImportRota(w, importRequest(c.query, importBody))
		if w.Code != c.wantStatus || !svc.called || svc.dryRun != c.wantDryRun || svc.code != "SRE_SAAS" {
			t.Fatalf("%q: status %d, called %v, dryRun %v, code %q", c.query, w.Code, svc.called, svc.dryRun, svc.code)
		}
	}
}

func TestImportRota_RefusesBeforeTheService(t *testing.T) {
	// A file over the 1 MiB body cap: what a far larger file than the entry cap allows would weigh.
	big := `{"schemaVersion":1,"family":"SRE","rotaCode":"SRE_SAAS","period":{"from":"2026-11-01","to":"2026-11-30"},"mode":"merge","assignments":[` +
		strings.Repeat(`{"email":"user1@example.com","teamKey":"apollo","shiftCode":"SRE_TZ1_L1","from":"2026-11-02","to":"2026-11-06"},`, 12000) +
		`{}]}`
	for _, c := range []struct {
		name, query, body, want string
	}{
		{"a body over 1 MiB", "", big, "request body too large"},
		{"a dryRun that is not a boolean", "?dryRun=maybe", importBody, "dryRun must be true or false"},
		{"a field the file does not have", "", `{"schemaVersion":1,"extra":true}`, "extra"},
	} {
		t.Run(c.name, func(t *testing.T) {
			svc := &importOnlySchedule{}
			w := httptest.NewRecorder()
			NewScheduleHandler(svc).ImportRota(w, importRequest(c.query, c.body))
			if w.Code != http.StatusBadRequest || svc.called {
				t.Fatalf("status %d, service called %v", w.Code, svc.called)
			}
			if !strings.Contains(w.Body.String(), c.want) {
				t.Fatalf("body %s, want it to say %q", w.Body.String(), c.want)
			}
		})
	}
}
