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

// Package poller turns unprocessed mailbox messages into events.
package poller

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/wso2-open-operations/cs-tools/integrations/csm-mail-paging-service/internal/rules"
)

// Mailbox is the mail source.
type Mailbox interface {
	ListUnprocessed(ctx context.Context, searchQuery string) ([]string, error)
	Get(ctx context.Context, id string) (rules.Message, error)
	MarkProcessed(ctx context.Context, id string) error
}

// Publisher publishes one event record.
type Publisher interface {
	Publish(ctx context.Context, key, value []byte) error
}

// Envelope is the wire shape every event on the bus shares; kept in sync by
// hand with csm-notification-service's events.Envelope.
type Envelope struct {
	Type     string          `json:"type"`
	EntityID string          `json:"entityId"`
	Payload  json.RawMessage `json:"payload"`
}

// Payload is what every rule-triggered event carries.
type Payload struct {
	RuleName  string `json:"ruleName"`
	MessageID string `json:"messageId"`
	// DedupeKey is stable across retries and distinct per rule, so one email
	// matching two rules yields two events a consumer must both act on.
	DedupeKey  string `json:"dedupeKey"`
	Subject    string `json:"subject"`
	From       string `json:"from"`
	ReceivedAt string `json:"receivedAt,omitempty"`
}

// Poller polls a mailbox and publishes an event per matching message.
type Poller struct {
	mailbox  Mailbox
	pub      Publisher
	rules    []rules.Rule
	query    string
	interval time.Duration
}

// New builds a Poller. lookback bounds how far back unprocessed mail is read
// (a Gmail newer_than value such as "2d").
func New(mb Mailbox, pub Publisher, rs []rules.Rule, interval time.Duration, lookback string) *Poller {
	return &Poller{mailbox: mb, pub: pub, rules: rs, interval: interval, query: searchQuery(rs, lookback)}
}

// searchQuery narrows Gmail's own listing to mail addressed to any rule's
// recipient, so unrelated mail is never read.
func searchQuery(rs []rules.Rule, lookback string) string {
	parts := make([]string, 0, len(rs))
	for _, r := range rs {
		parts = append(parts, fmt.Sprintf("to:%s cc:%s deliveredto:%s", r.Recipient, r.Recipient, r.Recipient))
	}
	return fmt.Sprintf("{%s} newer_than:%s", strings.Join(parts, " "), lookback)
}

// Run polls until ctx is cancelled.
func (p *Poller) Run(ctx context.Context) {
	t := time.NewTicker(p.interval)
	defer t.Stop()
	for {
		if err := p.Poll(ctx); err != nil {
			slog.ErrorContext(ctx, "mail poll failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Poll runs one pass. A message is labelled processed only after every
// matching rule's event was published, so a publish failure leaves it for the
// next pass: delivery is at-least-once, keyed by Message-ID.
func (p *Poller) Poll(ctx context.Context) error {
	ids, err := p.mailbox.ListUnprocessed(ctx, p.query)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := p.handle(ctx, id); err != nil {
			slog.ErrorContext(ctx, "mail message failed; will retry", "gmailId", id, "err", err)
		}
	}
	return nil
}

func (p *Poller) handle(ctx context.Context, id string) error {
	msg, err := p.mailbox.Get(ctx, id)
	if err != nil {
		return err
	}
	for _, r := range p.rules {
		if !r.Matches(msg) {
			continue
		}
		payload, err := json.Marshal(Payload{
			RuleName: r.Name, MessageID: msg.ID, DedupeKey: msg.ID + "/" + r.Name, Subject: msg.Subject,
			From: msg.From, ReceivedAt: msg.ReceivedAt,
		})
		if err != nil {
			return err
		}
		value, err := json.Marshal(Envelope{Type: r.EventType, EntityID: msg.ID, Payload: payload})
		if err != nil {
			return err
		}
		if err := p.pub.Publish(ctx, []byte(msg.ID), value); err != nil {
			return err
		}
		slog.InfoContext(ctx, "mail event published", "rule", r.Name, "eventType", r.EventType)
	}
	// Labelled even when no rule matched (e.g. a sender filter), so it is
	// not re-read on every pass.
	return p.mailbox.MarkProcessed(ctx, id)
}
