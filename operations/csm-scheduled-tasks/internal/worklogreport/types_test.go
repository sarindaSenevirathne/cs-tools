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

package worklogreport_test

import (
	"encoding/json"
	"testing"

	"github.com/wso2-open-operations/cs-tools/operations/csm-scheduled-tasks/internal/worklogreport"
)

// The producer sends fractional member counts -- a half-day absence is
// 0.5 of a present person. Declared as int on this side, a report
// carrying one fails to decode entirely and the mail never goes out.
func TestReportDecodesFractionalMemberCounts(t *testing.T) {
	const blob = `{
	  "cadence": "daily",
	  "totals": {"members": 6, "nonLeaveMembers": 5.5},
	  "teams": [{"teamName": "Atlas", "members": 3, "membersForAverage": 1.5}]
	}`
	var r worklogreport.Report
	if err := json.Unmarshal([]byte(blob), &r); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if r.Totals.NonLeaveMembers != 5.5 {
		t.Errorf("nonLeaveMembers = %v, want 5.5", r.Totals.NonLeaveMembers)
	}
	if len(r.Teams) != 1 || r.Teams[0].MembersForAverage != 1.5 {
		t.Errorf("membersForAverage did not survive the round trip: %+v", r.Teams)
	}
}
