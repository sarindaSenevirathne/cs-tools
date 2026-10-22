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

// Package eventbus publishes records to the Kafka-compatible event bus. It
// mirrors integrations/csm-notification-service's eventbus producer (separate
// Go modules, so kept in sync by hand) so both services speak to the same
// broker the same way.
package eventbus

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"time"

	kafka "github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/plain"
)

// Config holds the connection settings.
type Config struct {
	Broker           string
	ConnectionString string // SASL/PLAIN password; the username is the literal "$ConnectionString"
	Topic            string
}

// Producer publishes to one topic.
type Producer struct{ writer *kafka.Writer }

// NewProducer constructs a Producer; connecting is lazy.
func NewProducer(cfg Config) *Producer {
	return &Producer{writer: &kafka.Writer{
		Addr:         kafka.TCP(cfg.Broker),
		Topic:        cfg.Topic,
		Balancer:     &kafka.Hash{},
		RequiredAcks: kafka.RequireAll,
		// Publish writes one record per call and waits; without this the
		// writer holds each single-record batch for its 1s default.
		BatchTimeout: 10 * time.Millisecond,
		Transport: &kafka.Transport{
			TLS:  &tls.Config{MinVersion: tls.VersionTLS12},
			SASL: plain.Mechanism{Username: "$ConnectionString", Password: cfg.ConnectionString},
		},
		ErrorLogger: kafka.LoggerFunc(func(msg string, args ...any) { slog.Error(fmt.Sprintf(msg, args...)) }),
	}}
}

// Publish sends one record and waits for the broker's acknowledgment.
func (p *Producer) Publish(ctx context.Context, key, value []byte) error {
	if err := p.writer.WriteMessages(ctx, kafka.Message{Key: key, Value: value}); err != nil {
		return fmt.Errorf("eventbus: publish: %w", err)
	}
	return nil
}

// Close releases the connection.
func (p *Producer) Close() { _ = p.writer.Close() }
