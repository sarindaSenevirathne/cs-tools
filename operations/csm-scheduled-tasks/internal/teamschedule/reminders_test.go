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
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The reminder record: a 404 is "nothing sent yet", a record is decoded, a
// send is PUT with its invites, and the rota admins are read by family.
func TestReminders_RecordAndAdmins(t *testing.T) {
	var put struct {
		Invites []ReminderInvite `json:"invites"`
		SentBy  string           `json:"sentBy"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/team-schedule/reminders/CRE/weekend_notice/2026-10-24":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":404,"message":"nothing was sent for this period"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/team-schedule/reminders/CRE/weekend_notice/2026-10-31":
			_, _ = w.Write([]byte(`{"sendCount":2,"invites":[{"uid":"u1","sequence":7,"attendees":["ann@example.com"]}],"sentBy":"rota_weekend_reminder"}`))
		case r.Method == http.MethodPut && r.URL.Path == "/team-schedule/reminders/CRE/weekend_notice/2026-10-24":
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &put); err != nil {
				t.Errorf("PUT body: %v", err)
			}
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodGet && r.URL.Path == "/team-schedule/rota-admins/CRE":
			_, _ = w.Write([]byte(`{"admins":[{"name":"Zoe","email":"zoe@example.com"}]}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusTeapot)
		}
	}))
	defer srv.Close()
	c := newClient(srv.Client(), srv.URL)
	ctx := context.Background()

	if rec, err := c.GetReminder(ctx, "CRE", "weekend_notice", "2026-10-24"); err != nil || rec != nil {
		t.Fatalf("nothing sent yet: %+v, %v", rec, err)
	}
	rec, err := c.GetReminder(ctx, "CRE", "weekend_notice", "2026-10-31")
	if err != nil || rec == nil || rec.SendCount != 2 || rec.Invites[0].Sequence != 7 || rec.Invites[0].Attendees[0] != "ann@example.com" {
		t.Fatalf("record = %+v, %v", rec, err)
	}
	if err := c.RecordReminder(ctx, "CRE", "weekend_notice", "2026-10-24", []ReminderInvite{{UID: "u1", Sequence: 8, Attendees: []string{"ann@example.com"}}}, "rota_weekend_notice"); err != nil {
		t.Fatal(err)
	}
	if put.SentBy != "rota_weekend_notice" || len(put.Invites) != 1 || put.Invites[0].Sequence != 8 {
		t.Fatalf("PUT = %+v", put)
	}
	admins, err := c.RotaAdmins(ctx, "CRE")
	if err != nil || len(admins) != 1 || admins[0].Email != "zoe@example.com" {
		t.Fatalf("admins = %+v, %v", admins, err)
	}
}
