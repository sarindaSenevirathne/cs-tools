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

package dispatch

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/wso2-open-operations/cs-tools/integrations/csm-notification-service/internal/eventbus"
	"github.com/wso2-open-operations/cs-tools/integrations/csm-notification-service/internal/notifications"
)

// The Alert Settings mapping decides where an incident's card goes: the
// group's SRE team room, else the shared SRE room; a default group goes to
// the teamless rooms.

func incidentCreatedFor(group string) string {
	return strings.Replace(defaultGroupIncidentCreated, `"team":"Default SRE Group"`, `"team":"`+group+`"`, 1)
}

// mappedSpaces is what applyIncidentRoutes registers for: WSO2 IaaS Team ->
// iaas's room, the apollo and artemis team keys, and the shared SRE room.
func mappedSpaces(audience string) bool {
	switch audience {
	case notifications.IncidentAudience("WSO2 IaaS Team"),
		notifications.IncidentAudience("apollo"),
		notifications.IncidentAudience("artemis"),
		notifications.IncidentFallbackAudience:
		return true
	}
	return false
}

func TestDispatcher_IncidentRoutes(t *testing.T) {
	routes := &notifications.IncidentRoutes{}
	routes.Set(map[string][]string{"Default SRE Group": {"apollo", "artemis"}})

	cases := []struct {
		name  string
		group string
		env   map[string][]string
		want  []string
	}{
		{"a mapped group goes to its team's room", "WSO2 IaaS Team", nil,
			[]string{notifications.IncidentAudience("WSO2 IaaS Team")}},
		{"an unmapped group goes to the shared SRE room", "U2 SRE Group", nil,
			[]string{notifications.IncidentFallbackAudience}},
		{"a default group goes to the teamless rooms", "Default SRE Group", nil,
			[]string{notifications.IncidentAudience("apollo"), notifications.IncidentAudience("artemis")}},
		{"INCIDENT_CHAT_FANOUT still wins for a group it names", "Default SRE Group",
			map[string][]string{"Default SRE Group": {"WSO2 IaaS Team"}},
			[]string{notifications.IncidentAudience("WSO2 IaaS Team")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chat := &mockGoogleChatSender{hasAudienceSpace: mappedSpaces}
			d := NewDispatcher(&mockEmailSender{}, chat, &mockCallSender{}, &mockLinkResolver{}, true, false, nil, true, "", nil).
				WithIncidentChatFanout(tc.env).
				WithIncidentRoutes(routes)
			if err := d.Handle(context.Background(), eventbus.Record{Value: []byte(incidentCreatedFor(tc.group))}); err != nil {
				t.Fatalf("Handle error = %v", err)
			}
			if got := createdAudiences(chat); !slices.Equal(got, tc.want) {
				t.Errorf("card went to %v; want %v", got, tc.want)
			}
		})
	}
}

// With no mapping loaded and no shared room, an unmapped group still gets no
// card: nothing changes for a deployment that has not saved any routes.
func TestDispatcher_IncidentRoutes_NothingLoadedKeepsTheOldBehaviour(t *testing.T) {
	chat := &mockGoogleChatSender{hasAudienceSpace: saasSpaces}
	d := NewDispatcher(&mockEmailSender{}, chat, &mockCallSender{}, &mockLinkResolver{}, true, false, nil, true, "", nil).
		WithIncidentRoutes(&notifications.IncidentRoutes{})
	if err := d.Handle(context.Background(), eventbus.Record{Value: []byte(incidentCreatedFor("WSO2 IaaS Team"))}); err != nil {
		t.Fatalf("Handle error = %v", err)
	}
	if got := createdAudiences(chat); len(got) != 0 {
		t.Errorf("card went to %v; want none", got)
	}
}
