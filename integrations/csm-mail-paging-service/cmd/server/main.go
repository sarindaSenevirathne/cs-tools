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

// Command server polls a Gmail mailbox and publishes an event for each email
// that matches a configured rule.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/wso2-open-operations/cs-tools/integrations/csm-mail-paging-service/internal/config"
	"github.com/wso2-open-operations/cs-tools/integrations/csm-mail-paging-service/internal/eventbus"
	"github.com/wso2-open-operations/cs-tools/integrations/csm-mail-paging-service/internal/gmail"
	"github.com/wso2-open-operations/cs-tools/integrations/csm-mail-paging-service/internal/poller"
	"github.com/wso2-open-operations/cs-tools/integrations/csm-mail-paging-service/internal/rules"
)

func main() {
	if err := run(); err != nil {
		slog.Error("csm-mail-paging-service stopped", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	rs, err := rules.Parse(cfg.RulesJSON)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	mb, err := gmail.New(ctx, gmail.Config{
		CredentialsJSON: cfg.GmailCredentialsJSON,
		DelegatedUser:   cfg.GmailDelegatedUser,
		ProcessedLabel:  cfg.ProcessedLabel,
	})
	if err != nil {
		return err
	}
	if err := mb.EnsureLabel(ctx); err != nil {
		return err
	}
	prod := eventbus.NewProducer(eventbus.Config{Broker: cfg.KafkaBroker, ConnectionString: cfg.KafkaConnection, Topic: cfg.KafkaTopic})
	defer prod.Close()

	slog.Info("csm-mail-paging-service started", "rules", len(rs), "interval", cfg.PollInterval.String())
	poller.New(mb, prod, rs, cfg.PollInterval, cfg.Lookback).Run(ctx)
	return nil
}
