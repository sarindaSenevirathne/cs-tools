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

package rules

import "testing"

func TestParseAndMatch(t *testing.T) {
	rs, err := Parse(`[{"name":"a","recipient":"Oncall@Example.com","senderRegex":"@example\\.org$","eventType":"mail.alert.received"}]`)
	if err != nil {
		t.Fatal(err)
	}
	m := Message{From: "x@example.org", Recipients: []string{"oncall@example.com"}, Subject: "DB DOWN"}
	if !rs[0].Matches(m) {
		t.Error("expected match")
	}
	m.From = "x@other.net"
	if rs[0].Matches(m) {
		t.Error("sender should not match")
	}
	m.From = "x@example.org"
	m.Recipients = []string{"other@example.com"}
	if rs[0].Matches(m) {
		t.Error("recipient should not match")
	}
}

func TestParseErrors(t *testing.T) {
	for name, raw := range map[string]string{
		"bad json":     `nope`,
		"empty":        `[]`,
		"null":         `null`,
		"no name":      `[{"recipient":"a@b.c","eventType":"e"}]`,
		"no recipient": `[{"name":"a","eventType":"e"}]`,
		"no event":     `[{"name":"a","recipient":"a@b.c"}]`,
		"bad regex":    `[{"name":"a","recipient":"a@b.c","eventType":"e","senderRegex":"("}]`,
		"duplicate":    `[{"name":"a","recipient":"a@b.c","eventType":"e"},{"name":"a","recipient":"a@b.c","eventType":"e"}]`,
	} {
		if _, err := Parse(raw); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestAddresses(t *testing.T) {
	got := Addresses(`"Team" <Oncall@Example.com>, b@example.com`)
	if len(got) != 2 || got[0] != "oncall@example.com" {
		t.Errorf("got %v", got)
	}
	if Addresses("not an address list") != nil {
		t.Error("expected nil")
	}
}
