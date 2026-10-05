// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
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

// Package postgres connects to Azure Flexible Server PostgreSQL, claims ids from alert_seq and writes alerts rows.
package postgres

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// IDPrefix and IDWidth keep ids fixed-width so they sort in claim order.
	IDPrefix = "ALT"
	IDWidth  = 9
)

// FormatID renders a sequence number as an alert id, e.g. 123 -> "ALT000000123".
func FormatID(seq int64) string {
	return fmt.Sprintf("%s%0*d", IDPrefix, IDWidth, seq)
}

// Config holds connection settings from PG* env vars, identical to alerts-core's.
type Config struct {
	Host     string `env:"PGHOST,notEmpty"`
	Port     int    `env:"PGPORT" envDefault:"5432"`
	Database string `env:"PGDATABASE,notEmpty"`
	User     string `env:"PGUSER,notEmpty"`
	Password string `env:"PGPASSWORD,notEmpty"`
	SSLMode  string `env:"PGSSLMODE" envDefault:"require"`
	// PoolMaxConns caps this replica's pgxpool connections; 0 leaves pgx's default, too small under concurrent load.
	PoolMaxConns int32 `env:"PGPOOLMAXCONNS" envDefault:"0"`
}

// ConfigFromEnv reads Config from the environment.
func ConfigFromEnv() (Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return Config{}, fmt.Errorf("postgres config: %w", err)
	}
	return cfg, nil
}

// dsn builds the connection URL, escaping each part for where it sits; url.QueryEscape would turn a space in the password into "+".
func dsn(cfg Config) string {
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(cfg.User, cfg.Password),
		Host:     net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
		Path:     "/" + cfg.Database,
		RawQuery: url.Values{"sslmode": {cfg.SSLMode}}.Encode(),
	}
	return u.String()
}

// Connect opens a pooled connection, bounding connect time and the default per-query timeout.
func Connect(cfg Config, connectTimeout, queryTimeout time.Duration) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(dsn(cfg))
	if err != nil {
		return nil, fmt.Errorf("parse postgres config: %w", err)
	}
	poolCfg.ConnConfig.ConnectTimeout = connectTimeout
	if cfg.PoolMaxConns > 0 {
		poolCfg.MaxConns = cfg.PoolMaxConns
	}

	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	pingCtx, pingCancel := context.WithTimeout(context.Background(), queryTimeout)
	defer pingCancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return pool, nil
}

// Store implements the allocator's storage operations against one pool.
type Store struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
	claimTimeout time.Duration
}

// NewStore returns a Store over pool.
func NewStore(pool *pgxpool.Pool, queryTimeout, claimTimeout time.Duration) *Store {
	return &Store{pool: pool, queryTimeout: queryTimeout, claimTimeout: claimTimeout}
}

// ClaimIDs reserves n alert_seq values in one round trip; values are unique but not necessarily consecutive, since other replicas draw from the same sequence.
func (s *Store) ClaimIDs(ctx context.Context, n int) ([]int64, error) {
	ctx, cancel := context.WithTimeout(ctx, s.claimTimeout)
	defer cancel()
	rows, err := s.pool.Query(ctx, `SELECT nextval('alert_seq') FROM generate_series(1, $1)`, n)
	if err != nil {
		return nil, fmt.Errorf("claim %d ids: %w", n, err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return nil, fmt.Errorf("claim %d ids: %w", n, err)
	}
	if len(ids) != n {
		return nil, fmt.Errorf("claim %d ids: got %d", n, len(ids))
	}
	return ids, nil
}

// InsertRow is one alerts row; Fingerprint lets alerts-core claim an incident's alerts together.
type InsertRow struct {
	ID          string
	Source      string
	Alert       []byte
	Fingerprint string
}

// insertQuery writes a whole batch in one statement; ON CONFLICT makes a retry after a lost commit acknowledgement a no-op, and created_at comes from the database clock so every replica shares one time source.
const insertQuery = `INSERT INTO alerts (id, source, alert, fingerprint)
SELECT * FROM unnest($1::text[], $2::text[], $3::jsonb[], $4::text[])
ON CONFLICT (id) DO NOTHING`

// InsertBatch writes every row in one round trip; it either stores the whole batch or none of it, so retrying the same rows is safe.
func (s *Store) InsertBatch(ctx context.Context, rows []InsertRow) error {
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	ids := make([]string, len(rows))
	sources := make([]string, len(rows))
	alerts := make([]string, len(rows))
	fps := make([]string, len(rows))
	for i, r := range rows {
		ids[i], sources[i], alerts[i], fps[i] = r.ID, r.Source, string(r.Alert), r.Fingerprint
	}
	if _, err := s.pool.Exec(ctx, insertQuery, ids, sources, alerts, fps); err != nil {
		return fmt.Errorf("insert batch of %d: %w", len(rows), err)
	}
	return nil
}
