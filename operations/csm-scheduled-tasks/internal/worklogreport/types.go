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

// Package worklogreport holds the wire types entity-service returns for the
// CS Worklogs daily and weekly reports, and the client that fetches them.
//
// The types are a copy of entity-service's domain shapes rather than an
// import: the two components are separate Go modules, the same arrangement
// every other report sub-cron here uses.
package worklogreport

// Cadence distinguishes the two reports.
type Cadence string

const (
	CadenceDaily  Cadence = "daily"
	CadenceWeekly Cadence = "weekly"
)

// Report is one rendered report as entity-service returns it.
type Report struct {
	Cadence Cadence      `json:"cadence"`
	From    string       `json:"from"`
	To      string       `json:"to"`
	Totals  Totals       `json:"totals"`
	ByType  []TypeBucket `json:"byType"`
	Teams   []TeamReport `json:"teams"`
	// Recipients is the audience entity-service derives from the team
	// registry: team leads on To, team group addresses on Cc.
	Recipients Recipients `json:"recipients"`
}

// Recipients is the derived audience for a report.
type Recipients struct {
	To []string `json:"to"`
	Cc []string `json:"cc"`
}

// Totals is the report's summary block. TotalHours is ApprovedHours plus
// PendingHours; rejected time is excluded upstream.
type Totals struct {
	Members                   int     `json:"members"`
	Issues                    int     `json:"issues"`
	TotalHours                float64 `json:"totalHours"`
	ApprovedHours             float64 `json:"approvedHours"`
	PendingHours              float64 `json:"pendingHours"`
	AvgHoursPerPerson         float64 `json:"avgHoursPerPerson"`
	AvgApprovedHoursPerPerson float64 `json:"avgApprovedHoursPerPerson"`
	AvgPendingHoursPerPerson  float64 `json:"avgPendingHoursPerPerson"`
	AveragePeoplePerDay       float64 `json:"averagePeoplePerDay"`
	OnLeaveMembers            int     `json:"onLeaveMembers"`
	// Fractional, matching the producer: a half-day absence counts 0.5
	// of a present member. Declared int here, a report containing one
	// would fail to decode outright.
	NonLeaveMembers float64 `json:"nonLeaveMembers"`
	QueryIssues     int     `json:"queryIssues"`
	IncidentIssues  int     `json:"incidentIssues"`
}

// TypeBucket is one row of the work-item type breakdown. Label is what the
// report calls the type -- "Query" where the schema says CASE.
type TypeBucket struct {
	Type          string  `json:"type"`
	Label         string  `json:"label"`
	Issues        int     `json:"issues"`
	TotalHours    float64 `json:"totalHours"`
	ApprovedHours float64 `json:"approvedHours"`
	PendingHours  float64 `json:"pendingHours"`
}

// TeamReport is one team's section.
type TeamReport struct {
	TeamKey  string `json:"teamKey"`
	TeamName string `json:"teamName"`
	TeamType string `json:"teamType"`
	Members  int    `json:"members"`
	// MembersForAverage is the divisor behind the two averages below,
	// and is not the member count: the weekly report uses members less
	// one, the daily present members less leads. Fractional for the
	// same reason as NonLeaveMembers.
	MembersForAverage         float64     `json:"membersForAverage"`
	Issues                    int         `json:"issues"`
	ApprovedHours             float64     `json:"approvedHours"`
	PendingHours              float64     `json:"pendingHours"`
	TotalHours                float64     `json:"totalHours"`
	AvgApprovedHoursPerPerson float64     `json:"avgApprovedHoursPerPerson"`
	AvgTotalHoursPerPerson    float64     `json:"avgTotalHoursPerPerson"`
	Developers                []Developer `json:"developers"`
}

// IssueRef is one work item as the report prints it. The id travels with
// the key so the mail can link it.
type IssueRef struct {
	ID  string `json:"id"`
	Key string `json:"key"`
}

// Developer is one person's row inside a team section.
type Developer struct {
	UserID             string     `json:"userId"`
	Email              string     `json:"email"`
	Name               string     `json:"name"`
	WorkedDays         float64    `json:"workedDays"`
	Leave              string     `json:"leave"`
	ApprovedIssues     int        `json:"approvedIssues"`
	ApprovedHours      float64    `json:"approvedHours"`
	PendingIssues      int        `json:"pendingIssues"`
	PendingHours       float64    `json:"pendingHours"`
	Issues             []IssueRef `json:"issues"`
	DisapprovedEntries int        `json:"disapprovedEntries"`
	AvgTimePerDay      float64    `json:"avgTimePerDay"`
	TotalHours         float64    `json:"totalHours"`
}
