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
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
)

// IncidentAudiencePrefix namespaces INCIDENT_CHAT_SPACES entries so an assignment group never collides with a GOOGLE_CHAT_SPACES audience.
const IncidentAudiencePrefix = "incident/"

// IncidentAudience is the Chat audience key of an incident's assignment group.
func IncidentAudience(assignmentGroup string) string {
	return IncidentAudiencePrefix + strings.ToLower(strings.TrimSpace(assignmentGroup))
}

// IncidentFallbackAudience is where an incident's card and paging replies go
// when its assignment group has no space of its own: the SRE ladder's shared
// room, registered by SetIncidentRouteSpaces. "#" cannot start a group the
// mapping produces, so no assignment group can take this key.
const IncidentFallbackAudience = IncidentAudiencePrefix + "#unmapped"

// IncidentRoutes is the group fan-out taken from the Alert Settings mapping:
// a default group (an incident with no SRE team of its own) -> the SRE teams
// whose rooms take such incidents (sre.teamlessChats). It is shared by the
// dispatcher's card and the paging engine's replies so both pick the same
// rooms, and set once the paging configuration is loaded. The zero value and
// a nil pointer hold nothing. Safe for concurrent use.
type IncidentRoutes struct {
	mu     sync.RWMutex
	fanout map[string][]string
}

// Set replaces the routed fan-out (assignment group -> groups or team keys).
func (r *IncidentRoutes) Set(fanout map[string][]string) {
	m := make(map[string][]string, len(fanout))
	for group, targets := range fanout {
		if key := strings.ToLower(strings.TrimSpace(group)); key != "" && len(targets) > 0 {
			m[key] = targets
		}
	}
	r.mu.Lock()
	r.fanout = m
	r.mu.Unlock()
}

// Groups is the groups whose spaces get a group's incident messages:
// INCIDENT_CHAT_FANOUT's list when it names the group (env wins), else the
// mapping's, else the group itself.
func (r *IncidentRoutes) Groups(env map[string][]string, group string) []string {
	key := strings.ToLower(strings.TrimSpace(group))
	if key == "" {
		return nil
	}
	if groups := IncidentGroups(env, group); len(groups) != 1 || strings.ToLower(strings.TrimSpace(groups[0])) != key {
		return groups
	}
	if r != nil {
		r.mu.RLock()
		targets := r.fanout[key]
		r.mu.RUnlock()
		if len(targets) > 0 {
			return targets
		}
	}
	return []string{group}
}

// IncidentAudiences is the incident audiences, each with a space, that a
// group's card and paging replies go to: the audiences of routes.Groups that
// has knows, else IncidentFallbackAudience when has knows it (fallback is
// then true), else none. The dispatcher and the paging engine both use it,
// so the card and the replies under it always pick the same rooms.
func IncidentAudiences(routes *IncidentRoutes, env map[string][]string, group string, has func(string) bool) (audiences []string, fallback bool) {
	if strings.TrimSpace(group) == "" {
		return nil, false
	}
	for _, target := range routes.Groups(env, group) {
		audience := IncidentAudience(target)
		if has(audience) && !slices.Contains(audiences, audience) {
			audiences = append(audiences, audience)
		}
	}
	if len(audiences) == 0 && has(IncidentFallbackAudience) {
		return []string{IncidentFallbackAudience}, true
	}
	return audiences, false
}

// IncidentGroups is the assignment groups whose INCIDENT_CHAT_SPACES spaces get a group's incident messages: its INCIDENT_CHAT_FANOUT list, else the group itself.
func IncidentGroups(fanout map[string][]string, group string) []string {
	key := strings.ToLower(strings.TrimSpace(group))
	if key == "" {
		return nil
	}
	for g, targets := range fanout {
		if strings.ToLower(strings.TrimSpace(g)) == key && len(targets) > 0 {
			return targets
		}
	}
	return []string{group}
}

// IncidentCreatedAlert is SendIncidentCreatedAlert's card content.
type IncidentCreatedAlert struct {
	// IncidentID keys the card's thread (IncidentThreadKey).
	IncidentID       string
	Number           string
	Team             string
	Priority         string
	ShortDescription string
	Service          string
	Environment      string
	Category         string
	State            string
	IncidentLink     string
}

// IncidentAssignedAlert is SendIncidentAssignedAlert's card content.
type IncidentAssignedAlert struct {
	// IncidentID keys the card's thread (IncidentThreadKey).
	IncidentID   string
	Number       string
	AssigneeName string
	UpdatedBy    string
	IncidentLink string
}

// incidentPriorityLabels renders an incident priority the way ServiceNow's incident card does.
var incidentPriorityLabels = map[string]struct{ word, label string }{
	"CRITICAL": {"Critical", "P1 - Critical"},
	"HIGH":     {"High", "P2 - High"},
	"MODERATE": {"Moderate", "P3 - Moderate"},
	"LOW":      {"Low", "P4 - Low"},
	"PLANNING": {"Planning", "P5 - Planning"},
	"P1":       {"Critical", "P1 - Critical"},
	"P2":       {"High", "P2 - High"},
	"P3":       {"Moderate", "P3 - Moderate"},
	"P4":       {"Low", "P4 - Low"},
	"P5":       {"Planning", "P5 - Planning"},
}

// SendIncidentCreatedAlert posts ServiceNow's "<team> | <priority> Priority Incident Reported" card.
func (c *GoogleChatClient) SendIncidentCreatedAlert(ctx context.Context, audience string, a IncidentCreatedAlert) error {
	if a.IncidentID == "" {
		return fmt.Errorf("notifications: incidentId is required")
	}
	priority := incidentPriorityLabels[strings.ToUpper(strings.TrimSpace(a.Priority))]
	title := "Priority Incident Reported"
	if priority.word != "" {
		title = priority.word + " " + title
	}
	if strings.TrimSpace(a.Team) != "" {
		title = strings.ToUpper(strings.TrimSpace(a.Team)) + " | " + title
	}
	subtitle := "#" + incidentRef(a.Number, a.IncidentID)
	for _, part := range []string{a.Service, a.Environment} {
		if strings.TrimSpace(part) != "" {
			subtitle += " | " + strings.TrimSpace(part)
		}
	}

	summary := []chatCardWidget{{TextParagraph: &chatTextParagraph{
		Text: caseAlertLine(`<b>Short Description:</b><br>%s`, orEmDash(a.ShortDescription)),
	}}}
	if a.IncidentLink != "" {
		summary = append(summary, chatCardWidget{ButtonList: &chatButtonList{
			Buttons: []chatButton{{Text: "View Incident", OnClick: chatOnClick{OpenLink: chatOpenLink{URL: a.IncidentLink}}}},
		}})
	}
	details := []string{
		caseAlertLine(`<b>Category:</b> %s`, orEmDash(strings.ToLower(a.Category))),
		caseAlertLine(`<b>Priority:</b> %s`, orEmDash(priority.label)),
		caseAlertLine(`<b>State:</b> %s`, orEmDash(humanizeEnumLabel(a.State))),
	}
	sections := []chatCardSection{
		{Widgets: summary},
		{
			Header:      "Incident Details",
			Collapsible: true,
			Widgets:     []chatCardWidget{{TextParagraph: &chatTextParagraph{Text: strings.Join(details, "<br>")}}},
		},
	}

	msg := chatCardMessage{
		CardsV2: []chatCardWrapper{{CardID: "incident-created-alert", Card: chatCard{
			Header:   &chatCardHeader{Title: title, Subtitle: subtitle},
			Sections: sections,
		}}},
		Thread: &chatThread{ThreadKey: IncidentThreadKey(a.IncidentID)},
	}
	return c.sendCardToAudience(ctx, audience, a.Number, a.IncidentID, msg)
}

// SendIncidentAssignedAlert posts ServiceNow's "Incident Acknowledged." reply under the incident's card.
func (c *GoogleChatClient) SendIncidentAssignedAlert(ctx context.Context, audience string, a IncidentAssignedAlert) error {
	if a.IncidentID == "" {
		return fmt.Errorf("notifications: incidentId is required")
	}
	lines := []string{caseAlertLine(`Incident acknowledged by %s.`, orEmDash(a.AssigneeName))}
	if a.UpdatedBy != "" {
		lines = append(lines, caseAlertLine(`Incident updated by %s.`, a.UpdatedBy))
	}
	// The link keeps the reply usable on its own in a space that never got the card (the group changed since creation).
	if a.IncidentLink != "" {
		lines = append(lines, caseAlertLine(`<a href="%s">View incident</a>`, a.IncidentLink))
	}
	msg := chatCardMessage{
		CardsV2: []chatCardWrapper{{CardID: "incident-assigned-alert", Card: chatCard{
			Header:   &chatCardHeader{Title: "Incident Acknowledged.", Subtitle: incidentRef(a.Number, a.IncidentID)},
			Sections: []chatCardSection{{Widgets: []chatCardWidget{{TextParagraph: &chatTextParagraph{Text: strings.Join(lines, "<br>")}}}}},
		}}},
		Thread: &chatThread{ThreadKey: IncidentThreadKey(a.IncidentID)},
	}
	return c.sendCardToAudience(ctx, audience, a.Number, a.IncidentID, msg)
}

// incidentRef is the incident's number, or its id when an older publisher sent no number.
func incidentRef(number, incidentID string) string {
	if strings.TrimSpace(number) != "" {
		return number
	}
	return incidentID
}

// IncidentThreadKey keeps an incident's created card and its replies, the paging rungs included, in one Chat thread.
func IncidentThreadKey(incidentID string) string {
	return "incident-" + incidentID
}

// humanizeEnumLabel turns an UPPER_SNAKE value such as IN_PROGRESS into "In Progress".
func humanizeEnumLabel(v string) string {
	words := strings.Fields(strings.ReplaceAll(strings.ToLower(strings.TrimSpace(v)), "_", " "))
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}
