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

package gmail

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientFlow(t *testing.T) {
	var modified bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/users/me/labels" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"labels":[]}`))
		case r.URL.Path == "/users/me/labels" && r.Method == http.MethodPost:
			_, _ = w.Write([]byte(`{"id":"Label_1"}`))
		case r.URL.Path == "/users/me/messages":
			if !strings.Contains(r.URL.Query().Get("q"), `-label:"processed"`) {
				t.Errorf("query lacks processed-label exclusion: %q", r.URL.Query().Get("q"))
			}
			_, _ = w.Write([]byte(`{"messages":[{"id":"m1"}]}`))
		case r.URL.Path == "/users/me/messages/m1":
			_, _ = w.Write([]byte(`{"internalDate":"1700000000000","payload":{"headers":[
				{"name":"From","value":"Sender <S@Example.org>"},
				{"name":"To","value":"group@example.com"},
				{"name":"Subject","value":"hi"},
				{"name":"Message-ID","value":"<id@x>"}]}}`))
		case r.URL.Path == "/users/me/messages/m1/modify":
			modified = true
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, err := New(context.Background(), Config{ProcessedLabel: "processed", APIBaseURL: srv.URL, HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := c.EnsureLabel(ctx); err != nil {
		t.Fatal(err)
	}
	ids, err := c.ListUnprocessed(ctx, "x")
	if err != nil || len(ids) != 1 {
		t.Fatalf("ids=%v err=%v", ids, err)
	}
	m, err := c.Get(ctx, "m1")
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != "<id@x>" || m.From != "s@example.org" || m.Recipients[0] != "group@example.com" || m.ReceivedAt == "" {
		t.Errorf("unexpected message %+v", m)
	}
	if err := c.MarkProcessed(ctx, "m1"); err != nil || !modified {
		t.Errorf("mark failed: %v modified=%v", err, modified)
	}
}

func TestListUnprocessedPagesOldestFirst(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pageToken") == "" {
			_, _ = w.Write([]byte(`{"messages":[{"id":"new"},{"id":"mid"}],"nextPageToken":"p2"}`))
			return
		}
		_, _ = w.Write([]byte(`{"messages":[{"id":"old"}]}`))
	}))
	defer srv.Close()
	c, _ := New(context.Background(), Config{ProcessedLabel: "processed", APIBaseURL: srv.URL, HTTPClient: srv.Client()})
	ids, err := c.ListUnprocessed(context.Background(), "x")
	if err != nil || len(ids) != 3 || ids[0] != "old" || ids[2] != "new" {
		t.Errorf("ids=%v err=%v", ids, err)
	}
}
