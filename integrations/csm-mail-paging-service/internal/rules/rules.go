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

// Package rules decides which inbound emails should raise an event. A rule is
// pure configuration: add one to MAIL_TRIGGER_RULES to react to another
// mailbox address or group without touching code.
package rules

import (
	"encoding/json"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
)

// Message is the part of an email a rule can look at.
type Message struct {
	ID         string // RFC 5322 Message-ID, the idempotency key
	From       string
	Recipients []string // To, Cc and Delivered-To addresses
	Subject    string
	ReceivedAt string // RFC3339
}

// Rule is one entry of MAIL_TRIGGER_RULES.
type Rule struct {
	Name string `json:"name"`
	// Recipient is the group (or mailbox) address the email must have been
	// sent to.
	Recipient string `json:"recipient"`
	// SenderRegex is an optional extra condition.
	SenderRegex string `json:"senderRegex,omitempty"`
	// EventType is the event published when the rule matches.
	EventType string `json:"eventType"`

	sender *regexp.Regexp
}

// Parse reads and validates MAIL_TRIGGER_RULES.
func Parse(raw string) ([]Rule, error) {
	var rules []Rule
	if err := json.Unmarshal([]byte(raw), &rules); err != nil {
		return nil, fmt.Errorf("rules: parse: %w", err)
	}
	if len(rules) == 0 {
		return nil, fmt.Errorf("rules: at least one rule is required")
	}
	seen := map[string]bool{}
	for i := range rules {
		r := &rules[i]
		switch {
		case r.Name == "":
			return nil, fmt.Errorf("rules: rule %d has no name", i)
		case seen[r.Name]:
			return nil, fmt.Errorf("rules: duplicate rule name %q", r.Name)
		case strings.TrimSpace(r.Recipient) == "":
			return nil, fmt.Errorf("rules: rule %q has no recipient", r.Name)
		case r.EventType == "":
			return nil, fmt.Errorf("rules: rule %q has no eventType", r.Name)
		}
		seen[r.Name] = true
		r.Recipient = strings.ToLower(strings.TrimSpace(r.Recipient))
		var err error
		if r.SenderRegex != "" {
			if r.sender, err = regexp.Compile(r.SenderRegex); err != nil {
				return nil, fmt.Errorf("rules: rule %q senderRegex: %w", r.Name, err)
			}
		}
	}
	return rules, nil
}

// Matches reports whether m satisfies every condition of r.
func (r Rule) Matches(m Message) bool {
	found := false
	for _, addr := range m.Recipients {
		if strings.EqualFold(addr, r.Recipient) {
			found = true
			break
		}
	}
	if !found {
		return false
	}
	if r.sender != nil && !r.sender.MatchString(m.From) {
		return false
	}
	return true
}

// Addresses extracts bare lower-case addresses from an address-list header
// value; an unparseable value yields none.
func Addresses(header string) []string {
	list, err := mail.ParseAddressList(header)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, a := range list {
		out = append(out, strings.ToLower(a.Address))
	}
	return out
}
