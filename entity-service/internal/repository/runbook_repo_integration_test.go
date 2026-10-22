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

// This is an integration test: it applies migrations 0241, 0243 and 0244 in a private schema and runs
// RunbookRepository against it. Skipped without a DSN:
//
//	RUNBOOK_TEST_DSN=postgres://... go test ./internal/repository/ -run Runbook

package repository_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
)

// runbookTestPool creates a private schema with migration 0241 and 0180's number series applied, runs seed against it, applies
// 0243 and 0244 twice each and returns a pool on that schema.
func runbookTestPool(t *testing.T, seed string) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("RUNBOOK_TEST_DSN")
	if dsn == "" {
		t.Skip("RUNBOOK_TEST_DSN not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(admin.Close)
	schema := fmt.Sprintf("runbook_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE") })

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect to schema: %v", err)
	}
	t.Cleanup(pool.Close)

	apply := func(file string) {
		sql, err := os.ReadFile("../../migrations/" + file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply %s: %v", file, err)
		}
	}
	apply("0241_runbook.sql")
	numbering, err := os.ReadFile("../../migrations/0180_native_number_series.sql")
	if err != nil {
		t.Fatalf("read 0180: %v", err)
	}
	for _, part := range []struct{ from, to string }{
		{"CREATE TABLE number_series (", ");"},
		{"CREATE OR REPLACE FUNCTION next_series_number(", "$$ LANGUAGE plpgsql;"},
	} {
		sql := string(numbering)
		start := strings.Index(sql, part.from)
		end := strings.Index(sql[start:], part.to)
		if start < 0 || end < 0 {
			t.Fatalf("0180 no longer holds %q", part.from)
		}
		if _, err := pool.Exec(ctx, sql[start:start+end+len(part.to)]); err != nil {
			t.Fatalf("apply 0180's %q: %v", part.from, err)
		}
	}
	if seed != "" {
		if _, err := pool.Exec(ctx, seed); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	apply("0243_runbook_alerts.sql")
	apply("0243_runbook_alerts.sql")
	apply("0244_runbook_name.sql")
	apply("0244_runbook_name.sql")
	return pool
}

func isConflict(err error) bool {
	var c *apierror.ConflictError
	return errors.As(err, &c)
}

func TestRunbookIntegration_MigrationsNumberEachEntryOldestFirst(t *testing.T) {
	pool := runbookTestPool(t, `INSERT INTO runbook (alert_name, runbook_url, created_on, created_by, updated_by)
		VALUES ('gw-5xx', 'https://docs.example.com/gw', '2026-02-01', 's', 's'), (' JVM Heap ', NULL, '2026-01-01', 's', 's')`)
	got, err := repository.NewRunbookRepository(pool).List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d runbooks, want 2", len(got))
	}
	if got[0].Number != "RB0000002" || got[0].Name != "gw-5xx" || got[0].RunbookURL == nil || !reflect.DeepEqual(got[0].Alerts, []string{"gw-5xx"}) {
		t.Fatalf("first (newest) = %+v", got[0])
	}
	if got[1].Number != "RB0000001" || got[1].Name != "JVM Heap" || got[1].RunbookURL != nil || !reflect.DeepEqual(got[1].Alerts, []string{"JVM Heap"}) {
		t.Fatalf("second = %+v", got[1])
	}
	var hasName bool
	if err := pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM pg_attribute WHERE attrelid = 'runbook'::regclass AND attname = 'alert_name' AND NOT attisdropped)`).Scan(&hasName); err != nil || hasName {
		t.Fatalf("runbook.alert_name must be gone: exists = %v, err = %v", hasName, err)
	}
}

func TestRunbookIntegration_CreateUpdateDelete(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewRunbookRepository(runbookTestPool(t, ""))

	gw, err := repo.Create(ctx, "a@x", domain.SaveRunbookRequest{Name: "Gateway", RunbookURL: strp("https://d/gw"), Alerts: []string{"gw-latency", "Gateway 5xx Spike"}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if gw.Number != "RB0000001" || gw.Name != "Gateway" || !reflect.DeepEqual(gw.Alerts, []string{"Gateway 5xx Spike", "gw-latency"}) || gw.CreatedBy != "a@x" {
		t.Fatalf("created = %+v", gw)
	}

	_, err = repo.Create(ctx, "a@x", domain.SaveRunbookRequest{Name: "JVM", Alerts: []string{"GW-LATENCY"}})
	if !isConflict(err) || err.Error() != "alert gw-latency already belongs to runbook RB0000001" {
		t.Fatalf("taken alert: err = %v", err)
	}
	_, err = repo.Create(ctx, "a@x", domain.SaveRunbookRequest{Name: " gateway ", Alerts: []string{"other"}})
	if !isConflict(err) || err.Error() != "a runbook named  gateway  already exists" {
		t.Fatalf("taken name, other case: err = %v", err)
	}

	updated, err := repo.Update(ctx, "b@x", gw.ID, domain.SaveRunbookRequest{Name: "Gateway latency", Alerts: []string{"gw-latency", "gw-timeout"}})
	if err != nil {
		t.Fatalf("Update keeping its own alert: %v", err)
	}
	if updated.Number != gw.Number || updated.Name != "Gateway latency" || !reflect.DeepEqual(updated.Alerts, []string{"gw-latency", "gw-timeout"}) || updated.RunbookURL != nil || updated.UpdatedBy != "b@x" {
		t.Fatalf("updated = %+v", updated)
	}
	if missing, err := repo.Update(ctx, "b@x", "00000000-0000-4000-8000-000000000000", domain.SaveRunbookRequest{Name: "N", Alerts: []string{"n"}}); err != nil || missing != nil {
		t.Fatalf("Update unknown id = %v, %v; want nil, nil", missing, err)
	}

	if ok, err := repo.Delete(ctx, gw.ID); err != nil || !ok {
		t.Fatalf("Delete = %v, %v", ok, err)
	}
	next, err := repo.Create(ctx, "a@x", domain.SaveRunbookRequest{Name: "Gateway latency", Alerts: []string{"gw-latency"}})
	if err != nil {
		t.Fatalf("deleting a runbook must free its alerts: %v", err)
	}
	// The two refused creates above each drew a number too, and a number is never reused.
	if next.Number != "RB0000004" {
		t.Fatalf("a number is never reused: got %s, want RB0000004", next.Number)
	}
}

func TestRunbookIntegration_ImportMatchesTheRunbookOfTheFirstListedAlert(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewRunbookRepository(runbookTestPool(t, ""))
	gw, err := repo.Create(ctx, "a@x", domain.SaveRunbookRequest{Name: "Gateway", Alerts: []string{"gw-5xx", "gw-latency"}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	res, err := repo.Import(ctx, "i@x", []domain.SaveRunbookRequest{
		{RunbookURL: strp("https://d/gw"), Alerts: []string{"GW-5XX"}},
		{Alerts: []string{"gw-latency", "db-latency"}},
	})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if res.Created != 1 || res.Updated != 1 || len(res.Removed) != 0 {
		t.Fatalf("import = %+v, want 1 created and 1 updated", res)
	}
	got, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 || got[1].ID != gw.ID || got[1].Name != "Gateway" || got[1].RunbookURL == nil || !reflect.DeepEqual(got[1].Alerts, []string{"GW-5XX"}) {
		t.Fatalf("the first entry must update %s, got %+v", gw.Number, got)
	}
	if got[0].Number != "RB0000002" || got[0].Name != "gw-latency" || !reflect.DeepEqual(got[0].Alerts, []string{"db-latency", "gw-latency"}) {
		t.Fatalf("the second entry must be a new runbook, listed first, got %+v", got[0])
	}
}

func TestRunbookIntegration_ImportRemovesARunbookWhoseAlertsAllMove(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewRunbookRepository(runbookTestPool(t, ""))
	if _, err := repo.Create(ctx, "a@x", domain.SaveRunbookRequest{Name: "Disk low", RunbookURL: strp("https://d/low"), Alerts: []string{"disk-low"}}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	full, err := repo.Create(ctx, "a@x", domain.SaveRunbookRequest{Name: "Disk full", Alerts: []string{"disk-full"}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	res, err := repo.Import(ctx, "i@x", []domain.SaveRunbookRequest{{RunbookURL: strp("https://d/disk"), Alerts: []string{"disk-full", "DISK-LOW"}}})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if res.Created != 0 || res.Updated != 1 || len(res.Removed) != 1 ||
		res.Removed[0].Number != "RB0000001" || res.Removed[0].Name != "Disk low" || res.Removed[0].RunbookURL == nil || *res.Removed[0].RunbookURL != "https://d/low" {
		t.Fatalf("import = %+v, want 1 updated and RB0000001 removed with its link", res)
	}
	got, _ := repo.List(ctx)
	if len(got) != 1 || got[0].ID != full.ID || !reflect.DeepEqual(got[0].Alerts, []string{"disk-full", "DISK-LOW"}) {
		t.Fatalf("got %+v, want only %s holding both alerts", got, full.Number)
	}
}

func TestRunbookIntegration_ImportNames(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewRunbookRepository(runbookTestPool(t, ""))
	if _, err := repo.Create(ctx, "a@x", domain.SaveRunbookRequest{Name: "Gateway", Alerts: []string{"gw-5xx"}}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := repo.Create(ctx, "a@x", domain.SaveRunbookRequest{Name: "Disk", Alerts: []string{"disk-low"}}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := repo.Import(ctx, "i@x", []domain.SaveRunbookRequest{
		{Alerts: []string{"gw-5xx", "gw-latency"}},
		{Name: "Storage", Alerts: []string{"disk-low"}},
		{Alerts: []string{"jvm-heap"}},
	}); err != nil {
		t.Fatalf("Import: %v", err)
	}
	got, _ := repo.List(ctx)
	names := map[string]string{}
	for _, rb := range got {
		names[rb.Alerts[0]] = rb.Name
	}
	want := map[string]string{"gw-5xx": "Gateway", "disk-low": "Storage", "jvm-heap": "jvm-heap"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("names = %v, want %v (an entry with no name keeps the matched name, a new one is named after its first alert)", names, want)
	}

	_, err := repo.Import(ctx, "i@x", []domain.SaveRunbookRequest{{Name: "GATEWAY", Alerts: []string{"jvm-heap"}}})
	if !isConflict(err) || err.Error() != "a runbook named GATEWAY already exists" {
		t.Fatalf("an import taking another runbook's name: err = %v, want the name conflict", err)
	}
	if after, _ := repo.List(ctx); !reflect.DeepEqual(after, got) {
		t.Fatalf("a refused import must change nothing")
	}
}

func TestRunbookIntegration_ImportWaitsForAnEditOfTheSameRunbook(t *testing.T) {
	ctx := context.Background()
	pool := runbookTestPool(t, "")
	repo := repository.NewRunbookRepository(pool)
	x, err := repo.Create(ctx, "a@x", domain.SaveRunbookRequest{Name: "X", Alerts: []string{"a", "b"}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Hold the lock an edit of x takes, then start an import that moves b away from x.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT 1 FROM runbook WHERE id = $1::uuid FOR UPDATE`, x.ID); err != nil {
		t.Fatalf("lock: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := repo.Import(ctx, "i@x", []domain.SaveRunbookRequest{{Alerts: []string{"b"}}})
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("the import must wait for the lock on %s, finished with %v", x.Number, err)
	case <-time.After(300 * time.Millisecond):
	}

	// Once the lock is released the import goes ahead: b leaves x, which keeps a.
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("Import: %v", err)
	}
	got, _ := repo.List(ctx)
	for _, rb := range got {
		if len(rb.Alerts) == 0 {
			t.Fatalf("no runbook may be left without alerts, got %+v", got)
		}
	}
}
