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

package notifications

import (
	"slices"
	"testing"
)

func TestSetIncidentRouteSpaces_TheMappingWinsOverINCIDENT_CHAT_SPACES(t *testing.T) {
	c := NewGoogleChatClient(GoogleChatConfig{AudienceSpaces: []GoogleChatAudienceSpace{
		{Audience: IncidentAudience("WSO2 IaaS Team"), WebhookURL: "https://chat.googleapis.com/env"},
		{Audience: "Incident Monitor", WebhookURL: "https://chat.googleapis.com/monitor"},
	}})
	if u, _ := c.AudienceWebhookURL(IncidentAudience("WSO2 IaaS Team")); u != "https://chat.googleapis.com/env" {
		t.Fatalf("before the mapping: %q", u)
	}
	c.SetIncidentRouteSpaces(map[string]string{
		IncidentAudience("WSO2 IaaS Team"): "https://chat.googleapis.com/iaas",
		"Incident Monitor":                 "https://chat.googleapis.com/ignored",
	})
	if u, _ := c.AudienceWebhookURL(IncidentAudience("WSO2 IaaS Team")); u != "https://chat.googleapis.com/iaas" {
		t.Errorf("mapped group resolves to %q; want the team room", u)
	}
	if u, _ := c.AudienceWebhookURL("Incident Monitor"); u != "https://chat.googleapis.com/monitor" {
		t.Errorf("a non-incident audience was replaced: %q", u)
	}
	c.SetIncidentRouteSpaces(nil)
	if u, _ := c.AudienceWebhookURL(IncidentAudience("WSO2 IaaS Team")); u != "https://chat.googleapis.com/env" {
		t.Errorf("clearing the mapping should fall back to the env space, got %q", u)
	}
}

func TestIncidentAudiences(t *testing.T) {
	routes := &IncidentRoutes{}
	routes.Set(map[string][]string{"Default SRE Group": {"apollo", "artemis"}})
	has := func(a string) bool {
		return a == IncidentAudience("apollo") || a == IncidentAudience("WSO2 IaaS Team") || a == IncidentFallbackAudience
	}

	got, fb := IncidentAudiences(routes, nil, "WSO2 IaaS Team", has)
	if !slices.Equal(got, []string{IncidentAudience("WSO2 IaaS Team")}) || fb {
		t.Errorf("mapped group = %v, fallback %v", got, fb)
	}
	got, _ = IncidentAudiences(routes, nil, "Default SRE Group", has)
	if !slices.Equal(got, []string{IncidentAudience("apollo")}) {
		t.Errorf("default group = %v; want only the room that exists", got)
	}
	got, fb = IncidentAudiences(routes, nil, "U2 SRE Group", has)
	if !slices.Equal(got, []string{IncidentFallbackAudience}) || !fb {
		t.Errorf("unmapped group = %v, fallback %v", got, fb)
	}
	if got, _ := IncidentAudiences(routes, nil, "  ", has); got != nil {
		t.Errorf("no group = %v; want none", got)
	}
	var none *IncidentRoutes
	if got := none.Groups(nil, "X"); !slices.Equal(got, []string{"X"}) {
		t.Errorf("nil routes = %v", got)
	}
}
