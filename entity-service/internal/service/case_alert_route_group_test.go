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

package service

import (
	"encoding/json"
	"testing"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
)

// A route picked from the assignment groups stores the group's id and the
// group's own name, whatever groupName says.
func TestCaseAlertSaveRoute_ByGroupID(t *testing.T) {
	repo := newFakeCaseAlertRepo()
	svc, _ := newCaseAlerts(repo, testCaseAlertKey)
	got, err := svc.SaveRoute(asPerson("sre.rota@wso2.com"), domain.SaveCaseAlertRouteRequest{
		Ladder: "SRE", GroupID: "77777777-7777-4777-8777-000000000001", GroupName: "typo", TeamKey: "apollo",
	})
	if err != nil {
		t.Fatal(err)
	}
	if repo.route == nil || repo.route.GroupID != "77777777-7777-4777-8777-000000000001" || repo.route.GroupName != "wso2 iaas team" {
		t.Fatalf("saved %+v; want the group's id and its own name", repo.route)
	}
	if got.GroupID == "" {
		t.Errorf("response carries no groupId: %+v", got)
	}

	for name, id := range map[string]string{"not a uuid": "nope", "no such group": "77777777-7777-4777-8777-000000000099"} {
		_, err := svc.SaveRoute(asPerson("sre.rota@wso2.com"), domain.SaveCaseAlertRouteRequest{Ladder: "SRE", GroupID: id, TeamKey: "apollo"})
		wantAPIError[*apierror.ValidationError](t, err, name)
	}
}

// The tab lists the assignment groups, and warns about the SRE ones nothing
// covers: no route, no team named as them.
func TestCaseAlertSettings_AssignmentGroupsAndUnmapped(t *testing.T) {
	repo := newFakeCaseAlertRepo()
	repo.routes = map[string]repository.CaseAlertRouteRow{
		routeSRE: {ID: routeSRE, Ladder: "SRE", GroupName: "u2 sre group", TeamKey: "artemis", Enabled: true},
	}
	svc, _ := newCaseAlerts(repo, testCaseAlertKey)
	got, err := svc.Get(asPerson("sre.rota@wso2.com"))
	if err != nil {
		t.Fatal(err)
	}
	if got.RouteOptions == nil || len(got.RouteOptions.AssignmentGroups) != 3 {
		t.Fatalf("assignment groups = %+v; want the three support groups", got.RouteOptions)
	}
	// Apollo is a team's name and U2 SRE Group has a route; WSO2 IaaS Team has neither.
	if len(got.UnmappedGroups) != 1 || got.UnmappedGroups[0].Name != "WSO2 IaaS Team" {
		t.Errorf("unmapped = %+v; want only WSO2 IaaS Team", got.UnmappedGroups)
	}

	cre, err := svc.Get(asPerson("cre.rota@wso2.com"))
	if err != nil {
		t.Fatal(err)
	}
	if cre.UnmappedGroups != nil {
		t.Errorf("a CRE-only viewer sees the SRE warning: %+v", cre.UnmappedGroups)
	}
}

// A group the mounted file already routes (an alias to a team, or a default
// group) is not unmapped: the alert service reports that routing and the
// warning reads it.
func TestCaseAlertSettings_FileRoutedGroupsAreNotUnmapped(t *testing.T) {
	repo := newFakeCaseAlertRepo()
	mode := "db"
	repo.mode = &mode
	repo.summary = []byte(`{"running":{
		"ladders":{"SRE":{"enabled":{"value":true,"source":"file"}}},
		"groups":{"SRE":{"aliases":{"WSO2 IaaS Team":"iaas"},"defaultGroups":["U2 SRE Group"],"teamlessRooms":[]}}
	}}`)
	svc, _ := newCaseAlerts(repo, testCaseAlertKey)
	got, err := svc.Get(asPerson("sre.rota@wso2.com"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.UnmappedGroups) != 0 {
		t.Errorf("unmapped = %+v; want none: the file routes every group", got.UnmappedGroups)
	}
	if got.Running == nil || len(got.Running.Groups["SRE"]) == 0 {
		t.Errorf("running.groups.SRE not passed on: %+v", got.Running)
	}

	// Without a report the warning falls back to routes and team names.
	repo.summary = nil
	got, err = svc.Get(asPerson("sre.rota@wso2.com"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.UnmappedGroups) != 2 {
		t.Errorf("unmapped = %+v; want U2 SRE Group and WSO2 IaaS Team", got.UnmappedGroups)
	}
}

func TestReportedSRERouting(t *testing.T) {
	if reportedSRERouting(nil) != nil {
		t.Error("no report should mean no routing")
	}
	r := &domain.CaseAlertRunning{Groups: map[string]json.RawMessage{
		"SRE": json.RawMessage(`{"aliases":{" Apollo SRE Group ":"apollo"},"defaultGroups":["MS/PC SRE Group"]}`),
		"CRE": json.RawMessage(`{"aliases":{"cre only":"x"}}`),
	}}
	got := reportedSRERouting(r)
	if !got["apollo sre group"] || !got["ms/pc sre group"] || got["cre only"] {
		t.Errorf("routing = %v", got)
	}
	if reportedSRERouting(&domain.CaseAlertRunning{Groups: map[string]json.RawMessage{"SRE": json.RawMessage(`not json`)}}) != nil {
		t.Error("an unreadable report should be ignored")
	}
}
