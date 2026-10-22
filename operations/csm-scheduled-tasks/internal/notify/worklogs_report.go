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

package notify

import (
	_ "embed"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/wso2-open-operations/cs-tools/operations/csm-scheduled-tasks/internal/worklogreport"
)

//go:embed templates/worklogs_report.html
var worklogsReportTemplateRaw string

var worklogsReportTemplate = bakeLogo(worklogsReportTemplateRaw)

const (
	cellLeft  = `style="padding:7px 10px; border:1px solid #dfe3e6;"`
	cellRight = `align="right" style="padding:7px 10px; border:1px solid #dfe3e6;"`
	headLeft  = `align="left" style="background-color:#f1f3f4; padding:8px 10px; border:1px solid #dfe3e6;"`
	headRight = `align="right" style="background-color:#f1f3f4; padding:8px 10px; border:1px solid #dfe3e6;"`
)

// WorklogsReportSubject is the mail's subject line, in the same shape the
// delivered reports use: "[2026-10-18] CS Worklogs Daily Report" and
// "[2026-10-05 - 2026-10-11] CS Worklogs Weekly Report".
// caseLink renders one issue key, linked when a base URL is configured.
//
// ServiceNow linked each key into its agent workspace, a URL that stops
// existing at cutover. caseLinkBase is the CSM portal, the equivalent
// internal surface and where the engineers reading these reports
// actually work -- not the customer portal, which is a different
// audience most of them cannot open. The path matches
// csm-notification-service's recipientlinks resolver, which already
// builds <CSM base>/cases/{id} for internal recipients.
//
// Empty renders plain text: an unset deployment shows the key rather
// than a link to the wrong host.
func caseLink(caseLinkBase string, iss worklogreport.IssueRef) string {
	key := escapeHTML(iss.Key)
	if caseLinkBase == "" || iss.ID == "" {
		return key
	}
	href := strings.TrimRight(caseLinkBase, "/") + "/cases/" + url.PathEscape(iss.ID)
	return `<a href="` + escapeHTML(href) +
		`" style="color:#ff7300; text-decoration:none;">` + key + `</a>`
}

func WorklogsReportSubject(r worklogreport.Report) string {
	if r.Cadence == worklogreport.CadenceWeekly {
		return fmt.Sprintf("[%s - %s] CS Worklogs Weekly Report", r.From, r.To)
	}
	return fmt.Sprintf("[%s] CS Worklogs Daily Report", r.From)
}

// RenderWorklogsReport fills in the CS Worklogs HTML email template. One
// template serves both cadences: the daily report shows a Leave column and
// disapproved entries, the weekly one worked days and a per-day average,
// and the shared columns are identical.
func RenderWorklogsReport(r worklogreport.Report, caseLinkBase string) string {
	daily := r.Cadence != worklogreport.CadenceWeekly

	cadenceLabel := "Weekly"
	rangeLabel := "Date range: " + r.From + " to " + r.To
	if daily {
		cadenceLabel = "Daily"
		rangeLabel = "Date: " + r.From
	}

	replacer := strings.NewReplacer(
		"<!-- [CADENCE_LABEL] -->", cadenceLabel,
		"<!-- [RANGE_LABEL] -->", escapeHTML(rangeLabel),
		"<!-- [SUMMARY_ROWS] -->", renderWorklogSummary(r, daily),
		"<!-- [TEAM_SUMMARY_ROWS] -->", renderWorklogTeamSummary(r),
		"<!-- [TYPE_ROWS] -->", renderWorklogTypes(r),
		"<!-- [TEAM_SECTIONS] -->", renderWorklogTeamSections(r, daily, caseLinkBase),
		"<!-- [YEAR] -->", strconv.Itoa(time.Now().Year()),
	)
	return replacer.Replace(worklogsReportTemplate)
}

func hrs(f float64) string { return strconv.FormatFloat(f, 'f', 2, 64) }

// days renders a worked-day count, which is a whole number unless somebody
// took half-day leave.
func days(f float64) string {
	if f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'f', 1, 64)
}

func summaryRow(label, value string) string {
	return `<tr><td ` + cellLeft + `>` + escapeHTML(label) + `</td><td ` + cellRight +
		`><strong>` + escapeHTML(value) + `</strong></td></tr>`
}

func renderWorklogSummary(r worklogreport.Report, daily bool) string {
	t := r.Totals
	var b strings.Builder
	b.WriteString(summaryRow("Total members", strconv.Itoa(t.Members)))
	b.WriteString(summaryRow("Total issues", strconv.Itoa(t.Issues)))
	b.WriteString(summaryRow("Total support time (hrs)", hrs(t.TotalHours)))
	b.WriteString(summaryRow("Average support time (per person)", hrs(t.AvgHoursPerPerson)))
	if daily {
		b.WriteString(summaryRow("Non-leave members", days(t.NonLeaveMembers)))
		b.WriteString(summaryRow("On leave", strconv.Itoa(t.OnLeaveMembers)))
	} else {
		b.WriteString(summaryRow("Average people per day", hrs(t.AveragePeoplePerDay)))
	}
	b.WriteString(summaryRow("Queries", strconv.Itoa(t.QueryIssues)))
	b.WriteString(summaryRow("Incidents", strconv.Itoa(t.IncidentIssues)))
	b.WriteString(summaryRow("Approved (hrs)", hrs(t.ApprovedHours)))
	b.WriteString(summaryRow("Pending (hrs)", hrs(t.PendingHours)))
	return b.String()
}

func renderWorklogTeamSummary(r worklogreport.Report) string {
	if len(r.Teams) == 0 {
		return `<tr><td colspan="7" style="padding:16px 10px; text-align:center; color:#8a8f98;">No teams logged time in this period.</td></tr>`
	}
	var b strings.Builder
	for _, t := range r.Teams {
		b.WriteString(`<tr>`)
		b.WriteString(`<td ` + cellLeft + `>` + escapeHTML(t.TeamName) + `</td>`)
		b.WriteString(`<td ` + cellRight + `>` + strconv.Itoa(t.Members) + `</td>`)
		b.WriteString(`<td ` + cellRight + `>` + strconv.Itoa(t.Issues) + `</td>`)
		b.WriteString(`<td ` + cellRight + `>` + hrs(t.ApprovedHours) + `</td>`)
		b.WriteString(`<td ` + cellRight + `>` + hrs(t.PendingHours) + `</td>`)
		b.WriteString(`<td ` + cellRight + `><strong>` + hrs(t.TotalHours) + `</strong></td>`)
		b.WriteString(`<td ` + cellRight + `>` + hrs(t.AvgTotalHoursPerPerson) + `</td>`)
		b.WriteString(`</tr>`)
	}
	return b.String()
}

func renderWorklogTypes(r worklogreport.Report) string {
	if len(r.ByType) == 0 {
		return `<tr><td colspan="5" style="padding:16px 10px; text-align:center; color:#8a8f98;">No time logged in this period.</td></tr>`
	}
	var b strings.Builder
	for _, t := range r.ByType {
		b.WriteString(`<tr>`)
		b.WriteString(`<td ` + cellLeft + `>` + escapeHTML(t.Label) + `</td>`)
		b.WriteString(`<td ` + cellRight + `>` + strconv.Itoa(t.Issues) + `</td>`)
		b.WriteString(`<td ` + cellRight + `>` + hrs(t.ApprovedHours) + `</td>`)
		b.WriteString(`<td ` + cellRight + `>` + hrs(t.PendingHours) + `</td>`)
		b.WriteString(`<td ` + cellRight + `><strong>` + hrs(t.TotalHours) + `</strong></td>`)
		b.WriteString(`</tr>`)
	}
	return b.String()
}

// renderWorklogTeamSections builds the per-team developer tables -- the
// bulk of the mail. The weekly report carries worked days and an average
// per day; the daily one carries leave and disapproved entries instead.
func renderWorklogTeamSections(r worklogreport.Report, daily bool, caseLinkBase string) string {
	var b strings.Builder
	for _, t := range r.Teams {
		b.WriteString(`<h3 style="margin:28px 0 4px; font-size:16px; color:#465868;">` +
			escapeHTML(t.TeamName) + `</h3>`)
		b.WriteString(`<p style="margin:0 0 10px; font-size:13px; color:#8a8f98;">` +
			strconv.Itoa(t.Members) + ` members &middot; ` + strconv.Itoa(t.Issues) +
			` issues &middot; ` + hrs(t.ApprovedHours) + ` approved hrs &middot; ` +
			hrs(t.PendingHours) + ` pending hrs &middot; ` +
			hrs(t.AvgTotalHoursPerPerson) + ` avg total hrs/person</p>`)

		b.WriteString(`<table border="0" cellpadding="0" cellspacing="0" width="100%" class="reportTable" style="margin:0 0 10px; border-collapse:collapse; font-size:12px;"><thead><tr>`)
		b.WriteString(`<th ` + headLeft + `>Developer</th>`)
		if daily {
			b.WriteString(`<th ` + headLeft + `>Leave</th>`)
		} else {
			b.WriteString(`<th ` + headRight + `>Worked days</th>`)
		}
		b.WriteString(`<th ` + headRight + `>Approved #</th>`)
		b.WriteString(`<th ` + headRight + `>Approved hrs</th>`)
		b.WriteString(`<th ` + headRight + `>Pending #</th>`)
		b.WriteString(`<th ` + headRight + `>Pending hrs</th>`)
		if daily {
			b.WriteString(`<th ` + headRight + `>Disapproved</th>`)
		} else {
			b.WriteString(`<th ` + headRight + `>Avg hrs/day</th>`)
		}
		b.WriteString(`<th ` + headLeft + `>Issues</th>`)
		b.WriteString(`</tr></thead><tbody>`)

		if len(t.Developers) == 0 {
			b.WriteString(`<tr><td colspan="8" style="padding:12px 10px; text-align:center; color:#8a8f98;">No members.</td></tr>`)
		}
		for _, d := range t.Developers {
			b.WriteString(`<tr>`)
			b.WriteString(`<td ` + cellLeft + `>` + escapeHTML(d.Email) + `</td>`)
			if daily {
				b.WriteString(`<td ` + cellLeft + `>` + escapeHTML(dashIfEmpty(d.Leave)) + `</td>`)
			} else {
				b.WriteString(`<td ` + cellRight + `>` + days(d.WorkedDays) + `</td>`)
			}
			b.WriteString(`<td ` + cellRight + `>` + strconv.Itoa(d.ApprovedIssues) + `</td>`)
			b.WriteString(`<td ` + cellRight + `>` + hrs(d.ApprovedHours) + `</td>`)
			b.WriteString(`<td ` + cellRight + `>` + strconv.Itoa(d.PendingIssues) + `</td>`)
			b.WriteString(`<td ` + cellRight + `>` + hrs(d.PendingHours) + `</td>`)
			if daily {
				b.WriteString(`<td ` + cellRight + `>` + strconv.Itoa(d.DisapprovedEntries) + `</td>`)
			} else {
				b.WriteString(`<td ` + cellRight + `>` + hrs(d.AvgTimePerDay) + `</td>`)
			}
			links := make([]string, 0, len(d.Issues))
			for _, iss := range d.Issues {
				links = append(links, caseLink(caseLinkBase, iss))
			}
			b.WriteString(`<td ` + cellLeft + `>` + strings.Join(links, " ") + `</td>`)
			b.WriteString(`</tr>`)
		}
		b.WriteString(`</tbody></table>`)
	}
	return b.String()
}

// dashIfEmpty keeps an empty cell from reading as broken data, the same
// convention renderCaseRows uses.
func dashIfEmpty(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}
