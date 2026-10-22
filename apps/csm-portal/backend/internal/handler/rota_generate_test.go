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

	"github.com/wso2-open-operations/cs-tools/apps/csm-portal/backend/internal/apierror"
)

type fakeRotaGenerateClient struct {
	gotCode   string
	gotBody   string
	gotDryRun bool
	resp      []byte
	err       error
}

func (f *fakeRotaGenerateClient) GenerateRotaMonth(_ context.Context, code string, body []byte) ([]byte, error) {
	f.gotCode, f.gotBody = code, string(body)
	return f.resp, f.err
}

func (f *fakeRotaGenerateClient) GetRotaNightCrew(_ context.Context, code, from, to string) ([]byte, error) {
	f.gotCode, f.gotBody = code, from+".."+to
	return f.resp, f.err
}

func (f *fakeRotaGenerateClient) ImportRota(_ context.Context, code string, dryRun bool, body []byte) ([]byte, error) {
	f.gotCode, f.gotDryRun, f.gotBody = code, dryRun, string(body)
	return f.resp, f.err
}

func generateRequest(body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/team-schedule/rotas/SRE_SAAS/generate", strings.NewReader(body))
	r.SetPathValue("code", "SRE_SAAS")
	return r
}

func TestGenerateRotaMonth_ForwardsTheRequestUntouched(t *testing.T) {
	for _, c := range []struct {
		body       string
		wantStatus int
	}{
		{`{"month":"2026-11","dryRun":true}`, http.StatusOK},
		{`{"month":"2026-11"}`, http.StatusCreated},
		{`{"month":"2026-11","regenerate":true,"from":"2026-11-16"}`, http.StatusCreated},
	} {
		fake := &fakeRotaGenerateClient{resp: []byte(`{"month":"2026-11"}`)}
		w := httptest.NewRecorder()
		NewRotaGenerateHandler(fake).GenerateRotaMonth(w, withUser(generateRequest(c.body)))
		if w.Code != c.wantStatus {
			t.Fatalf("%s: status %d, want %d", c.body, w.Code, c.wantStatus)
		}
		if fake.gotCode != "SRE_SAAS" || fake.gotBody != c.body {
			t.Fatalf("forwarded %q %q", fake.gotCode, fake.gotBody)
		}
	}
}

func TestGenerateRotaMonth_NeedsAUser(t *testing.T) {
	w := httptest.NewRecorder()
	NewRotaGenerateHandler(&fakeRotaGenerateClient{}).GenerateRotaMonth(w, generateRequest(`{"month":"2026-11"}`))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status %d", w.Code)
	}
}

// A refusal keeps entity-service's words: the lead needs to know to
// regenerate, or that the month is over, or that only a lead may generate.
func TestGenerateRotaMonth_KeepsTheRefusalReason(t *testing.T) {
	for _, c := range []struct {
		status int
		msg    string
		want   string
	}{
		{http.StatusConflict, "2026-11 has already been generated; regenerate to replace the generated turns", "regenerate"},
		{http.StatusBadRequest, "that month is over; only the current month or a later one can be generated", "that month is over"},
		{http.StatusForbidden, "generating the SaaS rota needs a lead of one of its teams, or an SRE rota admin", "needs a lead"},
		{http.StatusNotFound, `rota "CRE" cannot be generated; only SRE_SAAS can`, "cannot be generated"},
		{http.StatusInternalServerError, "pq: relation does not exist", "Failed to generate the rota."},
	} {
		fake := &fakeRotaGenerateClient{err: &apierror.Error{StatusCode: c.status, Body: `{"code":1,"message":"` + strings.ReplaceAll(c.msg, `"`, `\"`) + `"}`}}
		w := httptest.NewRecorder()
		NewRotaGenerateHandler(fake).GenerateRotaMonth(w, withUser(generateRequest(`{"month":"2026-11"}`)))
		if w.Code != c.status {
			t.Fatalf("status %d, want %d", w.Code, c.status)
		}
		if !strings.Contains(w.Body.String(), c.want) {
			t.Fatalf("body %s, want it to carry %q", w.Body.String(), c.want)
		}
		if c.status == http.StatusInternalServerError && strings.Contains(w.Body.String(), "relation") {
			t.Fatal("upstream internals leaked")
		}
	}
}

func nightCrewRequest(query string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/team-schedule/rotas/SRE_SAAS/night-crew?"+query, nil)
	r.SetPathValue("code", "SRE_SAAS")
	return r
}

func TestGetRotaNightCrew_ForwardsTheMonths(t *testing.T) {
	fake := &fakeRotaGenerateClient{resp: []byte(`{"rotaCode":"SRE_SAAS","months":[]}`)}
	w := httptest.NewRecorder()
	NewRotaGenerateHandler(fake).GetRotaNightCrew(w, withUser(nightCrewRequest("from=2026-10&to=2026-12")))
	if w.Code != http.StatusOK || fake.gotCode != "SRE_SAAS" || fake.gotBody != "2026-10..2026-12" {
		t.Fatalf("status %d, forwarded %q %q", w.Code, fake.gotCode, fake.gotBody)
	}
}

func TestGetRotaNightCrew_RefusesBeforeAskingUpstream(t *testing.T) {
	w := httptest.NewRecorder()
	NewRotaGenerateHandler(&fakeRotaGenerateClient{}).GetRotaNightCrew(w, nightCrewRequest("from=2026-10&to=2026-12"))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no user: status %d, want 401", w.Code)
	}
	for _, q := range []string{"", "from=2026-10", "from=2026-13&to=2026-12", "from=2026/10&to=2026-12", "from=2026-10&to=26-12"} {
		fake := &fakeRotaGenerateClient{}
		w := httptest.NewRecorder()
		NewRotaGenerateHandler(fake).GetRotaNightCrew(w, withUser(nightCrewRequest(q)))
		if w.Code != http.StatusBadRequest || fake.gotCode != "" {
			t.Fatalf("%q: status %d, upstream asked %v", q, w.Code, fake.gotCode != "")
		}
	}
}

func TestGetRotaNightCrew_KeepsTheRefusalReason(t *testing.T) {
	fake := &fakeRotaGenerateClient{err: &apierror.Error{StatusCode: http.StatusNotFound, Body: `{"code":404,"message":"rota \"SRE_IAAS\" has no night crew; only SRE_SAAS has"}`}}
	w := httptest.NewRecorder()
	NewRotaGenerateHandler(fake).GetRotaNightCrew(w, withUser(nightCrewRequest("from=2026-10&to=2026-12")))
	if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "has no night crew") {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
}

const importFileBody = `{"schemaVersion":1,"family":"SRE","rotaCode":"SRE_SAAS","period":{"from":"2026-11-01","to":"2026-11-30"},"mode":"replace","assignments":[{"email":"user1@example.com","teamKey":"apollo","shiftCode":"SRE_TZ1_L1","from":"2026-11-02","to":"2026-11-06"}],"absences":[]}`

func importRequest(query, body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/team-schedule/rotas/SRE_SAAS/import"+query, strings.NewReader(body))
	r.SetPathValue("code", "SRE_SAAS")
	return r
}

func TestImportRota_ForwardsTheFileUntouched(t *testing.T) {
	for _, c := range []struct {
		query      string
		wantDryRun bool
		wantStatus int
	}{
		{"?dryRun=true", true, http.StatusOK},
		{"?dryRun=false", false, http.StatusCreated},
		{"", false, http.StatusCreated},
	} {
		fake := &fakeRotaGenerateClient{resp: []byte(`{"rotaCode":"SRE_SAAS"}`)}
		w := httptest.NewRecorder()
		NewRotaGenerateHandler(fake).ImportRota(w, withUser(importRequest(c.query, importFileBody)))
		if w.Code != c.wantStatus {
			t.Fatalf("%q: status %d, want %d", c.query, w.Code, c.wantStatus)
		}
		if fake.gotCode != "SRE_SAAS" || fake.gotBody != importFileBody || fake.gotDryRun != c.wantDryRun {
			t.Fatalf("%q: forwarded %q dryRun=%v %q", c.query, fake.gotCode, fake.gotDryRun, fake.gotBody)
		}
	}
}

func TestImportRota_RefusesBeforeAskingUpstream(t *testing.T) {
	w := httptest.NewRecorder()
	NewRotaGenerateHandler(&fakeRotaGenerateClient{}).ImportRota(w, importRequest("", importFileBody))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no user: status %d, want 401", w.Code)
	}
	for _, c := range []struct{ query, body string }{
		{"?dryRun=maybe", importFileBody},
		{"", `{"schemaVersion":`},
	} {
		fake := &fakeRotaGenerateClient{}
		w := httptest.NewRecorder()
		NewRotaGenerateHandler(fake).ImportRota(w, withUser(importRequest(c.query, c.body)))
		if w.Code != http.StatusBadRequest || fake.gotCode != "" {
			t.Fatalf("%q %q: status %d, upstream asked %v", c.query, c.body, w.Code, fake.gotCode != "")
		}
	}
}

// The refusal names the entry at fault, so it has to reach the lead as entity-service wrote it.
func TestImportRota_KeepsTheRefusalReason(t *testing.T) {
	for _, c := range []struct {
		status int
		msg    string
		want   string
	}{
		{http.StatusBadRequest, "the rota file has problems: assignments[0]: nobody on the team schedule has the email user9@example.com", "assignments[0]"},
		{http.StatusForbidden, "assignments[2]: changing artemis's rota needs a lead of that team", "assignments[2]"},
		{http.StatusConflict, "assignments[1]: TZ3 L1 on 2026-11-03 is already held", "already held"},
		{http.StatusNotFound, `no rota "SRE_PAAS"`, "no rota"},
		{http.StatusInternalServerError, "pq: relation does not exist", "Failed to import the rota."},
	} {
		fake := &fakeRotaGenerateClient{err: &apierror.Error{StatusCode: c.status, Body: `{"code":1,"message":"` + strings.ReplaceAll(c.msg, `"`, `\"`) + `"}`}}
		w := httptest.NewRecorder()
		NewRotaGenerateHandler(fake).ImportRota(w, withUser(importRequest("", importFileBody)))
		if w.Code != c.status {
			t.Fatalf("status %d, want %d", w.Code, c.status)
		}
		if !strings.Contains(w.Body.String(), c.want) {
			t.Fatalf("body %s, want it to carry %q", w.Body.String(), c.want)
		}
		if c.status == http.StatusInternalServerError && strings.Contains(w.Body.String(), "relation") {
			t.Fatal("upstream internals leaked")
		}
	}
}
