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
	"maps"
	"slices"
	"testing"

	"github.com/wso2-open-operations/cs-tools/integrations/csm-notification-service/internal/notifications"
)

func TestBuildIncidentRoutes(t *testing.T) {
	env := map[string]string{
		"CASE_ALERTS_ROOM_SRE_IAAS":    "https://chat.googleapis.com/iaas",
		"CASE_ALERTS_ROOM_SRE_ARTEMIS": "https://chat.googleapis.com/artemis",
		"CASE_ALERTS_ROOM_SRE":         "https://chat.googleapis.com/sre",
	}
	audiences := map[string]string{"Apollo space": "https://chat.googleapis.com/apollo"}
	resolve := func(c Chat) (string, bool) {
		return ChatWebhookURL(c, audienceLookup(audiences), func(k string) string { return env[k] })
	}
	l := LadderConfig{
		Chat: Chat{Audience: "SRE", WebhookURLEnv: "CASE_ALERTS_ROOM_SRE"},
		TeamChats: map[string]Chat{
			"iaas":    {WebhookURLEnv: "CASE_ALERTS_ROOM_SRE_IAAS"},
			"artemis": {WebhookURLEnv: "CASE_ALERTS_ROOM_SRE_ARTEMIS"},
			"apollo":  {Audience: "Apollo space"},
			"missing": {WebhookURLEnv: "CASE_ALERTS_ROOM_UNSET"},
		},
		TeamlessChats: []string{"apollo", "artemis"},
	}
	l.Teams.Aliases = map[string]string{
		"WSO2 IaaS Team":      "iaas",
		"U2 SRE Group":        "artemis",
		"B-Central SRE Group": "artemis",
		"MS/PC SRE Group":     "missing",
		"Other Group":         "nobody",
	}
	l.Teams.DefaultGroups = []string{"Default SRE Group"}

	got := BuildIncidentRoutes(l, resolve)

	want := map[string]string{
		notifications.IncidentAudience("iaas"):                "https://chat.googleapis.com/iaas",
		notifications.IncidentAudience("artemis"):             "https://chat.googleapis.com/artemis",
		notifications.IncidentAudience("apollo"):              "https://chat.googleapis.com/apollo",
		notifications.IncidentAudience("WSO2 IaaS Team"):      "https://chat.googleapis.com/iaas",
		notifications.IncidentAudience("U2 SRE Group"):        "https://chat.googleapis.com/artemis",
		notifications.IncidentAudience("B-Central SRE Group"): "https://chat.googleapis.com/artemis",
		notifications.IncidentFallbackAudience:                "https://chat.googleapis.com/sre",
	}
	if !maps.Equal(got.Spaces, want) {
		t.Errorf("spaces = %v\nwant     %v", got.Spaces, want)
	}
	if fan := got.Fanout["default sre group"]; !slices.Equal(fan, []string{"apollo", "artemis"}) {
		t.Errorf("default group fan-out = %v; want [apollo artemis]", fan)
	}
	if got.Mapped != 6 {
		t.Errorf("mapped = %d; want 6 (3 team keys + 3 aliases)", got.Mapped)
	}
}

func TestBuildIncidentRoutes_NoTeamlessRoomsLeavesDefaultGroupsOnTheSharedRoom(t *testing.T) {
	l := LadderConfig{Chat: Chat{WebhookURLEnv: "SRE_ROOM"}}
	l.Teams.DefaultGroups = []string{"Default SRE Group"}
	got := BuildIncidentRoutes(l, func(c Chat) (string, bool) {
		return ChatWebhookURL(c, nil, func(string) string { return "https://chat.googleapis.com/sre" })
	})
	if len(got.Fanout) != 0 {
		t.Errorf("fan-out = %v; want none", got.Fanout)
	}
	if got.Spaces[notifications.IncidentFallbackAudience] == "" {
		t.Error("the shared SRE room is missing")
	}
}

type audienceLookup map[string]string

func (a audienceLookup) AudienceWebhookURL(audience string) (string, bool) {
	u, ok := a[audience]
	return u, ok
}
