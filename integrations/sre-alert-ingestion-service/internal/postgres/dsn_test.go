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

package postgres

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestDSN_EscapesEachPart: a space, "+", "@" or "/" in the credentials or database name must reach PostgreSQL unchanged.
func TestDSN_EscapesEachPart(t *testing.T) {
	cfg := Config{Host: "db.example.com", Port: 5432, Database: "alert db", User: "svc@corp",
		Password: "p a+ss/w@rd:%", SSLMode: "require"}
	pc, err := pgxpool.ParseConfig(dsn(cfg))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	c := pc.ConnConfig
	if c.User != cfg.User || c.Password != cfg.Password || c.Database != cfg.Database ||
		c.Host != cfg.Host || c.Port != uint16(cfg.Port) {
		t.Errorf("parsed user=%q password=%q database=%q host=%q port=%d", c.User, c.Password, c.Database, c.Host, c.Port)
	}
	if c.TLSConfig == nil {
		t.Error("sslmode=require should leave TLS on")
	}
}
