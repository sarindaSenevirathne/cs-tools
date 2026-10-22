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

package domain

import "time"

// MigrationCaseReportRequest asks for the weekly migration case scan: the
// migration engagements still open after OlderThanDays.
type MigrationCaseReportRequest struct {
	// OlderThanDays is the age a migration must have reached to be listed.
	// The cut-off is the start of the day that many days before Now, in the
	// report's own timezone, and a migration is listed only when it was
	// created at or before that instant: it has then been open for the
	// whole of those days. One created later on the cut-off day is not yet
	// old enough and waits for the next run. This is the rule the scan has
	// always applied.
	OlderThanDays int
	// Now is the instant the report is built for. The scheduled task passes
	// its firing time, so a retried run lists the same cases the original
	// would have.
	Now time.Time
}

// MigrationCaseReport is the weekly migration case scan: three tables of
// long-running migration engagements, split by the tags the CS team keeps
// on them. A migration carrying both tags appears in both of the first two
// tables; one carrying neither appears in the third.
type MigrationCaseReport struct {
	GeneratedAt   time.Time `json:"generatedAt"`
	OlderThanDays int       `json:"olderThanDays"`
	// CreatedBefore is the cut-off every listed migration was created on
	// or before.
	CreatedBefore time.Time `json:"createdBefore"`
	// OnHold are the migrations the customer asked to pause.
	OnHold []MigrationCaseReportRow `json:"onHold"`
	// WaitingOnAccounts are the migrations blocked on the accounts team.
	WaitingOnAccounts []MigrationCaseReportRow `json:"waitingOnAccounts"`
	// InProgress are the remaining long-running migrations: neither on
	// hold nor waiting on accounts.
	InProgress []MigrationCaseReportRow `json:"inProgress"`
	// Recipients are the mail groups of the teams configured to receive
	// the scan (team.group_email). Empty when no team is configured or none
	// of the configured teams has a mail group.
	Recipients []string `json:"recipients"`
}

// MigrationCaseReportRow is one migration engagement in the scan, with the
// account facts the report shows beside it.
type MigrationCaseReportRow struct {
	ID     string `json:"id"`
	Number string `json:"number"`
	// IssueKey is the engagement's customer-facing identifier.
	IssueKey  string    `json:"issueKey"`
	CreatedOn time.Time `json:"createdOn"`
	// LastCustomerCommentOn is when the customer last commented on the
	// engagement; nil when they never have.
	LastCustomerCommentOn *time.Time `json:"lastCustomerCommentOn"`
	AssignedTo            string     `json:"assignedTo"`
	Product               string     `json:"product"`
	Account               string     `json:"account"`
	// CsTeam is the name of the CS team that owns the account.
	CsTeam         string `json:"csTeam"`
	SalesRegion    string `json:"salesRegion"`
	AccountManager string `json:"accountManager"`
	TechnicalOwner string `json:"technicalOwner"`
	// OnHold and WaitingOnAccounts are the tags that decide which table
	// the row lands in. The service uses them to bucket; the tables
	// themselves are the contract, so they are not serialised.
	OnHold            bool `json:"-"`
	WaitingOnAccounts bool `json:"-"`
}
