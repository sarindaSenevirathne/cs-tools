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

package poller

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/wso2-open-operations/cs-tools/integrations/csm-mail-paging-service/internal/rules"
)

type fakeMailbox struct {
	msgs   map[string]rules.Message
	marked []string
}

func (f *fakeMailbox) ListUnprocessed(context.Context, string) ([]string, error) {
	var ids []string
	for id := range f.msgs {
		ids = append(ids, id)
	}
	return ids, nil
}
func (f *fakeMailbox) Get(_ context.Context, id string) (rules.Message, error) {
	return f.msgs[id], nil
}
func (f *fakeMailbox) MarkProcessed(_ context.Context, id string) error {
	f.marked = append(f.marked, id)
	return nil
}

type fakePub struct {
	records [][]byte
	err     error
}

func (f *fakePub) Publish(_ context.Context, _, v []byte) error {
	if f.err != nil {
		return f.err
	}
	f.records = append(f.records, v)
	return nil
}

func testRules(t *testing.T) []rules.Rule {
	t.Helper()
	rs, err := rules.Parse(`[{"name":"r","recipient":"group@example.com","eventType":"mail.alert.received"}]`)
	if err != nil {
		t.Fatal(err)
	}
	return rs
}

func TestPollPublishesAndMarks(t *testing.T) {
	mb := &fakeMailbox{msgs: map[string]rules.Message{
		"1": {ID: "<a@x>", From: "s@example.org", Recipients: []string{"group@example.com"}, Subject: "DB down"},
	}}
	pub := &fakePub{}
	p := New(mb, pub, testRules(t), time.Second, "2d")
	if err := p.Poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(pub.records) != 1 || len(mb.marked) != 1 {
		t.Fatalf("published %d, marked %d", len(pub.records), len(mb.marked))
	}
	var env Envelope
	if err := json.Unmarshal(pub.records[0], &env); err != nil {
		t.Fatal(err)
	}
	var pl Payload
	_ = json.Unmarshal(env.Payload, &pl)
	if env.Type != "mail.alert.received" || env.EntityID != "<a@x>" || pl.DedupeKey != "<a@x>/r" || pl.Subject != "DB down" {
		t.Errorf("unexpected event %+v %+v", env, pl)
	}
}

func TestPollPublishFailureLeavesMessageUnmarked(t *testing.T) {
	mb := &fakeMailbox{msgs: map[string]rules.Message{
		"1": {ID: "<a@x>", Recipients: []string{"group@example.com"}},
	}}
	p := New(mb, &fakePub{err: errors.New("down")}, testRules(t), time.Second, "2d")
	if err := p.Poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(mb.marked) != 0 {
		t.Error("message must stay unprocessed so the next pass retries it")
	}
}

func TestPollNoMatchStillMarks(t *testing.T) {
	mb := &fakeMailbox{msgs: map[string]rules.Message{"1": {ID: "<b@x>", Recipients: []string{"other@example.com"}}}}
	pub := &fakePub{}
	p := New(mb, pub, testRules(t), time.Second, "2d")
	_ = p.Poll(context.Background())
	if len(pub.records) != 0 || len(mb.marked) != 1 {
		t.Errorf("published %d, marked %d", len(pub.records), len(mb.marked))
	}
}

func TestSearchQuery(t *testing.T) {
	got := searchQuery(testRules(t), "2d")
	want := "{to:group@example.com cc:group@example.com deliveredto:group@example.com} newer_than:2d"
	if got != want {
		t.Errorf("got %q", got)
	}
}
