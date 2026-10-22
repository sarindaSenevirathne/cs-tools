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

package teamschedule

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// Nothing saved is a 404 and reads as nil; saved settings are decoded; any
// other failure is an error, never "nothing saved".
func TestReminderSettingsFor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/team-schedule/reminder-settings/SRE":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":404,"message":"no rota reminder settings saved for this family"}`))
		case "/team-schedule/reminder-settings/CRE":
			_, _ = w.Write([]byte(`{"weekdayEnabled":true,"weekendEnabled":false,"invitesEnabled":true,
				"weekendRequired":{"CRE_WEEKEND":2},"windows":{"CRE_WEEKEND_NIGHT":{"start":"08:00","end":"18:00","timeZone":"America/Los_Angeles"}},
				"copyTo":["lead@example.com"]}`))
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	c := newClient(srv.Client(), srv.URL)
	ctx := context.Background()

	if s, err := c.ReminderSettingsFor(ctx, "SRE"); err != nil || s != nil {
		t.Fatalf("nothing saved: %+v, %v", s, err)
	}
	s, err := c.ReminderSettingsFor(ctx, "CRE")
	if err != nil || s == nil || !s.WeekdayEnabled || s.WeekendEnabled || s.WeekendRequired["CRE_WEEKEND"] != 2 ||
		s.Windows["CRE_WEEKEND_NIGHT"].TimeZone != "America/Los_Angeles" || s.CopyTo[0] != "lead@example.com" {
		t.Fatalf("settings = %+v, %v", s, err)
	}
	if s, err := c.ReminderSettingsFor(ctx, "SME"); err == nil {
		t.Fatalf("a failure read as %+v", s)
	}
}

// Claiming: nothing waiting is a 404 and reads as nil; a claim is decoded; a
// report is PUT with its status and result.
func TestReminderRequests_ClaimAndReport(t *testing.T) {
	var reported string
	waiting := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/team-schedule/reminder-requests/CRE/claim":
			if !waiting {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"code":404,"message":"no rota reminder request is waiting"}`))
				return
			}
			_, _ = w.Write([]byte(`{"id":"r1","family":"CRE","kind":"weekend","status":"SENDING","requestedBy":"ann@example.com"}`))
		case r.Method == http.MethodPut && r.URL.Path == "/team-schedule/reminder-requests/CRE/r1/result":
			body, _ := io.ReadAll(r.Body)
			reported = string(body)
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusTeapot)
		}
	}))
	defer srv.Close()
	c := newClient(srv.Client(), srv.URL)
	ctx := context.Background()
	req, err := c.ClaimReminderRequest(ctx, "CRE")
	if err != nil || req == nil || req.Kind != "weekend" || req.RequestedBy != "ann@example.com" {
		t.Fatalf("claim = %+v, %v", req, err)
	}
	if err := c.ReportReminderRequest(ctx, "CRE", "r1", "SENT", "Sent 5 emails."); err != nil || reported != `{"result":"Sent 5 emails.","status":"SENT"}` {
		t.Fatalf("report %q, %v", reported, err)
	}
	waiting = false
	if req, err := c.ClaimReminderRequest(ctx, "CRE"); err != nil || req != nil {
		t.Fatalf("nothing waiting: %+v, %v", req, err)
	}
}

// Only entity-service's own "nothing there" answer reads as nothing: a 404
// from a route that does not exist yet (this task deployed before the
// entity-service that adds it) fails, so the weekend notice is not sent every
// evening and switched-off reminders do not go out.
func TestNotFound_OnlyEntityServicesOwnAnswerMeansNothing(t *testing.T) {
	bare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r) // "404 page not found", as Go's mux answers an unknown route
	}))
	defer bare.Close()
	c := newClient(bare.Client(), bare.URL)
	ctx := context.Background()
	if rec, err := c.GetReminder(ctx, "CRE", "weekend_notice", "2026-10-24"); err == nil {
		t.Errorf("GetReminder on a missing route = %+v, nil", rec)
	}
	if s, err := c.ReminderSettingsFor(ctx, "CRE"); err == nil {
		t.Errorf("ReminderSettingsFor on a missing route = %+v, nil", s)
	}
	if r, err := c.ClaimReminderRequest(ctx, "CRE"); err == nil {
		t.Errorf("ClaimReminderRequest on a missing route = %+v, nil", r)
	}
}

// The messages this client recognises are the ones entity-service writes; read
// from its handler, so a reworded message there fails here instead of turning
// every "nothing there" into a failed run.
func TestNotFound_MessagesMatchEntityService(t *testing.T) {
	src, err := os.ReadFile("../../../../entity-service/internal/handler/schedule_reminder_handler.go")
	if err != nil {
		t.Skip("entity-service is not next to this module here")
	}
	for _, m := range []string{nothingSentMessage, noSettingsMessage, nothingWaitingMessage} {
		if !strings.Contains(string(src), `http.StatusNotFound, "`+m+`")`) {
			t.Errorf("entity-service no longer answers %q", m)
		}
	}
}
