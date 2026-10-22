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

package paging

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/wso2-open-operations/cs-tools/integrations/csm-notification-service/internal/chataudience"
	"github.com/wso2-open-operations/cs-tools/integrations/csm-notification-service/internal/notifications"
)

// l1OnlyRota is a rota with one person on the first tier and nobody on the
// tiers above it, so the SRE ladder's only callable rung is its first, at
// offset 0.
func l1OnlyRota() *stubScheduleReader {
	return &stubScheduleReader{onDuty: []onDutyAssignment{
		held("a-l1", "apollo", "SRE_TZ1_L1", ""),
	}}
}

// The stale-ladder guard exists to drop a replayed trigger, so it must judge
// the trigger's age and not only the ladder's length: the SRE ladder's first
// rung is at offset 0, so when it is the only callable rung the ladder ends at
// the trigger time, always a moment in the past by the time it is evaluated.
func TestEngine_StaleLadderGuardJudgesTheTriggerAge(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name      string
		only      bool          // only the offset-0 rung is callable
		delay     time.Duration // how long after the report the engine evaluates it
		scheduled bool
		cards     int // cards posted when ticked at that moment
	}{
		{"fresh trigger, only the offset-0 rung", true, time.Second, true, 1},
		{"trigger at the grace edge, only the offset-0 rung", true, staleTriggerGrace, true, 1},
		{"old trigger, only the offset-0 rung", true, staleTriggerGrace + time.Second, false, 0},
		{"old trigger, whole ladder elapsed", false, 24 * time.Hour, false, 0},
		{"fresh trigger, ladder with later rungs", false, time.Second, true, 1},
		{"short backlog, ladder with later rungs", false, 7 * time.Minute, true, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chat, store := &fakeChat{}, newMemStore()
			e := ladderEngine(LadderSRE, chat, store)
			if tc.only {
				e.resolver = sreResolver(l1OnlyRota())
			}
			now := testClock.Add(tc.delay)
			e.clock = func() time.Time { return now }
			buf := captureLogs(t)

			if err := e.Handle(ctx, created(t, "Apollo", "HIGH")); err != nil {
				t.Fatal(err)
			}
			_, found, _ := store.Get(ctx, testIncidentID)
			if found != tc.scheduled {
				t.Fatalf("scheduled = %v, want %v", found, tc.scheduled)
			}
			dropped := strings.Contains(buf.String(), "trigger is older than its whole ladder")
			if dropped == tc.scheduled {
				t.Errorf("stale-ladder log present = %v for scheduled = %v", dropped, tc.scheduled)
			}
			if err := e.Tick(ctx, now); err != nil {
				t.Fatal(err)
			}
			if len(chat.posted) != tc.cards {
				t.Errorf("posted %d cards, want %d", len(chat.posted), tc.cards)
			}
			if len(chat.stopped) != 0 {
				t.Errorf("a ladder with a callable rung posted %d no-one-on-call alerts", len(chat.stopped))
			}
		})
	}
}

// A plan with nothing to deliver ends the incident's paging, so it must be
// announced in the log and on the incident, whether nobody holds any level or
// the calls are held.
func TestEngine_PlanWithNothingToDeliverIsAnnounced(t *testing.T) {
	cases := []struct {
		name     string
		resolver Resolver
		why      string
		noteHas  string
	}{
		{"no level has a recipient", StaticResolver{}, "no level has a recipient", "NO_RECIPIENTS"},
		{
			"calls held while the lead pool is unverified",
			poolResolver{StaticResolver: fullResolver()},
			"calls held",
			"CALLS_HELD",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, caller, notes := newMemStore(), &fakeCaller{}, &fakeNotes{}
			e := testEngine(store, caller, notes, enabled())
			e.resolver = tc.resolver
			buf := captureLogs(t)

			if err := e.Handle(context.Background(), createdEvent(t, "CRITICAL", testClock)); err != nil {
				t.Fatal(err)
			}
			if len(store.states) != 0 || len(caller.placed) != 0 {
				t.Errorf("scheduled %d ladders, placed %d calls; want neither", len(store.states), len(caller.placed))
			}
			logs := buf.String()
			if !containsAll(logs, "plan has no reachable recipients", "incidentId="+testIncidentID,
				"priority=", "rule=", "team=Atlas", "why=", tc.why) {
				t.Errorf("warning does not name the incident and the reason:\n%s", logs)
			}
			if len(notes.notes) != 1 || !strings.Contains(notes.notes[0], tc.noteHas) {
				t.Errorf("work notes = %q, want one mentioning %s", notes.notes, tc.noteHas)
			}
		})
	}
}

// With nobody to page, the team's room must hear it too, from the room the
// ladder's own cards use, else Incident Monitor, once per trigger.
func TestEngine_NoOneOnCallPostsAChatAlert(t *testing.T) {
	ctx := context.Background()
	const teamRoom = "WSO2 API Manager"
	// The incident's own space, from its assignment group.
	incidentRoom := notifications.IncidentAudience("Atlas")
	cases := []struct {
		name      string
		build     func(chat *fakeChat, notes *fakeNotes) *Engine
		noRoom    map[string]bool
		delay     time.Duration
		cards     int
		audience  string
		reason    string
		chatErr   error
		wantNote  string
		wantLog   string
		redeliver bool
	}{
		{
			name: "no recipients: one card to the team room, one note",
			build: func(chat *fakeChat, notes *fakeNotes) *Engine {
				e := chatEngine(t, chat, newMemStore(), notes)
				e.resolver = StaticResolver{}
				return e
			},
			cards: 1, audience: teamRoom, reason: "no level has a recipient", wantNote: "NO_RECIPIENTS",
		},
		{
			name: "calls held on a call-only ladder: reason named, incident's own space",
			build: func(chat *fakeChat, notes *fakeNotes) *Engine {
				e := testEngine(newMemStore(), &fakeCaller{}, notes, enabled())
				e.resolver = poolResolver{StaticResolver: fullResolver()}
				e.incidentChat = chat
				return e
			},
			cards: 1, audience: incidentRoom, reason: "calls held: the lead pool is not verified", wantNote: "CALLS_HELD",
		},
		{
			name: "team without a mapped space falls back to Incident Monitor",
			build: func(chat *fakeChat, notes *fakeNotes) *Engine {
				e := chatEngine(t, chat, newMemStore(), notes)
				e.resolver = StaticResolver{}
				e.incidentChat = chat
				return e
			},
			noRoom: map[string]bool{teamRoom: true, incidentRoom: true},
			cards:  1, audience: chataudience.IncidentMonitor, reason: "no level has a recipient", wantNote: "NO_RECIPIENTS",
		},
		{
			name: "no space anywhere: warning and note, no card",
			build: func(chat *fakeChat, notes *fakeNotes) *Engine {
				e := chatEngine(t, chat, newMemStore(), notes)
				e.resolver = StaticResolver{}
				e.incidentChat = chat
				return e
			},
			noRoom:   map[string]bool{teamRoom: true, incidentRoom: true, chataudience.IncidentMonitor: true},
			wantNote: "NO_RECIPIENTS", wantLog: "no Chat space for the no-one-on-call alert",
		},
		{
			name: "chat failure does not suppress the note",
			build: func(chat *fakeChat, notes *fakeNotes) *Engine {
				e := chatEngine(t, chat, newMemStore(), notes)
				e.resolver = StaticResolver{}
				return e
			},
			chatErr:  errors.New("chat unavailable"),
			wantNote: "NO_RECIPIENTS", wantLog: "the no-one-on-call alert was not posted",
		},
		{
			name: "a redelivered trigger does not post a second card",
			build: func(chat *fakeChat, notes *fakeNotes) *Engine {
				e := chatEngine(t, chat, newMemStore(), notes)
				e.resolver = StaticResolver{}
				return e
			},
			cards: 1, audience: teamRoom, reason: "no level has a recipient", wantNote: "NO_RECIPIENTS", redeliver: true,
		},
		{
			name: "a replayed old trigger posts no card but still writes the note",
			build: func(chat *fakeChat, notes *fakeNotes) *Engine {
				e := chatEngine(t, chat, newMemStore(), notes)
				e.resolver = StaticResolver{}
				return e
			},
			delay:    staleTriggerGrace + time.Second,
			wantNote: "NO_RECIPIENTS",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chat, notes := &fakeChat{noRoom: tc.noRoom, err: tc.chatErr}, &fakeNotes{}
			e := tc.build(chat, notes)
			now := testClock.Add(tc.delay)
			e.clock = func() time.Time { return now }
			buf := captureLogs(t)

			deliveries := 1
			if tc.redeliver {
				deliveries = 2
			}
			for range deliveries {
				if err := e.Handle(ctx, createdEvent(t, "CRITICAL", testClock)); err != nil {
					t.Fatalf("a failed alert must not fail the trigger: %v", err)
				}
			}
			if len(chat.stopped) != tc.cards {
				t.Fatalf("posted %d alerts, want %d", len(chat.stopped), tc.cards)
			}
			if tc.cards > 0 {
				got := chat.stopped[0]
				if got.Audience != tc.audience || got.Outcome != "No one on call" ||
					!containsAll(got.Detail, tc.reason, "CRITICAL", "Atlas", "nobody was paged") ||
					got.IncidentRef == "" {
					t.Errorf("alert = %+v, want audience %q naming the incident, priority, team and %q", got, tc.audience, tc.reason)
				}
			}
			if len(chat.posted) != 0 {
				t.Errorf("a plan with nobody to page posted %d rung cards", len(chat.posted))
			}
			if len(notes.notes) < 1 || !strings.Contains(notes.notes[0], tc.wantNote) {
				t.Errorf("work notes = %q, want one mentioning %s", notes.notes, tc.wantNote)
			}
			if tc.wantLog != "" && !strings.Contains(buf.String(), tc.wantLog) {
				t.Errorf("log lacks %q:\n%s", tc.wantLog, buf.String())
			}
		})
	}
}
