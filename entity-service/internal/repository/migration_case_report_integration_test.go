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

package repository_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
)

// Skipped unless ENTITY_TEST_DATABASE_URL is set. Runs against a migrated
// Postgres and seeds its own rows under the 6d1... id prefix, deleting
// them before and after so a half-finished run cannot poison the next.
//
//	ENTITY_TEST_DATABASE_URL="postgres:///entity_test" go test -v -run TestMigrationCaseReport ./internal/repository/

const (
	mcrAssigneeID = "6d100000-0000-4000-8000-000000000001"
	mcrAMID       = "6d100000-0000-4000-8000-000000000002"
	mcrTOID       = "6d100000-0000-4000-8000-000000000003"
	mcrCustomerID = "6d100000-0000-4000-8000-000000000004"
	mcrStaffID    = "6d100000-0000-4000-8000-000000000005"
	mcrGroupID    = "6d100000-0000-4000-8000-000000000010"
	mcrAccountID  = "6d100000-0000-4000-8000-000000000020"
	mcrProductID  = "6d100000-0000-4000-8000-000000000030"
	mcrTeamAID    = "6d100000-0000-4000-8000-000000000040"
	mcrTeamBID    = "6d100000-0000-4000-8000-000000000041"
	mcrHoldTagID  = "6d100000-0000-4000-8000-000000000050"
	mcrAccTagID   = "6d100000-0000-4000-8000-000000000051"
	mcrCustRoleID = "6d100000-0000-4000-8000-000000000060"
	mcrStaffRole  = "6d100000-0000-4000-8000-000000000061"

	mcrCustomerMail = "mcr-customer@example.test"
	mcrStaffMail    = "mcr-staff@example.test"
)

// mcrWorkItemID is the id of the n-th seeded engagement.
func mcrWorkItemID(n int) string {
	return "6d100000-0000-4000-8000-0000000001" + string(rune('0'+n/10)) + string(rune('0'+n%10))
}

func mcrPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("ENTITY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ENTITY_TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func mcrCleanup(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	for _, s := range []string{
		`DELETE FROM comment WHERE id::text LIKE '6d1%' OR work_item_id::text LIKE '6d1%'`,
		`DELETE FROM work_item_tag WHERE id::text LIKE '6d1%' OR work_item_id::text LIKE '6d1%'`,
		`DELETE FROM engagement WHERE id::text LIKE '6d1%'`,
		`DELETE FROM work_item WHERE id::text LIKE '6d1%'`,
		`DELETE FROM tag WHERE id::text LIKE '6d1%'`,
		`DELETE FROM team WHERE id::text LIKE '6d1%'`,
		`DELETE FROM product WHERE id::text LIKE '6d1%'`,
		`DELETE FROM account WHERE id::text LIKE '6d1%'`,
		`DELETE FROM "group" WHERE id::text LIKE '6d1%'`,
		`DELETE FROM user_role WHERE id::text LIKE '6d1%' OR user_id::text LIKE '6d1%'`,
		`DELETE FROM "user" WHERE id::text LIKE '6d1%'`,
		`DELETE FROM role WHERE id::text LIKE '6d1%'`,
	} {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Fatalf("cleanup %q: %v", s, err)
		}
	}
}

func TestMigrationCaseReportIntegration(t *testing.T) {
	pool := mcrPool(t)
	mcrCleanup(t, pool)
	t.Cleanup(func() { mcrCleanup(t, pool) })
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed: %v\n%s", err, sql)
		}
	}

	now := time.Now().UTC()
	old := now.AddDate(0, 0, -200)
	cutoff := now.AddDate(0, 0, -180)

	exec(`INSERT INTO "user" (id, created_on, updated_on, user_name, name, first_name, last_name, email, user_type, is_active) VALUES
		($1, now(), now(), 'mcr.assignee', 'Dana Example', NULL, NULL, 'mcr-assignee@example.test', 'INTERNAL', true),
		($2, now(), now(), 'mcr.am', NULL, 'Morgan', 'Example', 'mcr-am@example.test', 'INTERNAL', true),
		($3, now(), now(), 'mcr.to', 'Sam Example', NULL, NULL, 'mcr-to@example.test', 'INTERNAL', true),
		($4, now(), now(), 'mcr.customer', 'Casey Customer', NULL, NULL, $6, 'EXTERNAL', true),
		($5, now(), now(), 'mcr.staff', 'Robin Staff', NULL, NULL, $7, 'INTERNAL', true)`,
		mcrAssigneeID, mcrAMID, mcrTOID, mcrCustomerID, mcrStaffID, mcrCustomerMail, mcrStaffMail)
	// user_type is derived from roles by trigger, never stored directly: a
	// customer is a user holding the customer role, staff hold internal.
	// The roles may already exist on a shared database; use whichever row
	// carries the name.
	exec(`INSERT INTO role (id, created_on, updated_on, name) VALUES
		($1, now(), now(), 'customer'), ($2, now(), now(), 'internal') ON CONFLICT (name) DO NOTHING`, mcrCustRoleID, mcrStaffRole)
	exec(`INSERT INTO user_role (id, created_on, updated_on, user_id, role_id)
		SELECT '6d100000-0000-4000-8000-000000000070', now(), now(), $1, r.id FROM role r WHERE r.name = 'customer'`, mcrCustomerID)
	exec(`INSERT INTO user_role (id, created_on, updated_on, user_id, role_id)
		SELECT '6d100000-0000-4000-8000-000000000071', now(), now(), $1, r.id FROM role r WHERE r.name = 'internal'`, mcrStaffID)
	exec(`INSERT INTO "group" (id, created_on, updated_on, created_by, updated_by, name, group_email, is_active)
		VALUES ($1, now(), now(), 't', 't', 'Team Alpha', 'team-alpha@example.test', true)`, mcrGroupID)
	exec(`INSERT INTO account (id, created_on, updated_on, created_by, updated_by, name, number, sf_id,
			cre_team_id, sales_region, account_manager_id, technical_owner_id)
		VALUES ($1, now(), now(), 't', 't', 'Acme', 'ACC-MCR-1', 'SF-MCR-1', $2, 'EU', $3, $4)`,
		mcrAccountID, mcrGroupID, mcrAMID, mcrTOID)
	exec(`INSERT INTO product (id, created_on, updated_on, created_by, updated_by, manufacturer, category, name)
		VALUES ($1, now(), now(), 't', 't', 'WSO2', 'SOFTWARE', 'MCR Example Product')`, mcrProductID)
	exec(`INSERT INTO team (id, created_on, updated_on, name, type, key, group_email) VALUES
		($1, now(), now(), 'Scan Team A', 'cre-abt', 'mcr-team-a', '  team-a@example.test '),
		($2, now(), now(), 'Scan Team B', 'cre-abt', 'mcr-team-b', NULL)`, mcrTeamAID, mcrTeamBID)
	// The hold tag is seeded in capitals: matching must ignore case.
	exec(`INSERT INTO tag (id, created_on, updated_on, name) VALUES
		($1, now(), now(), 'MCR_HOLD'), ($2, now(), now(), 'mcr_acc')`, mcrHoldTagID, mcrAccTagID)

	engagement := func(n int, createdOn time.Time, state, engType string) string {
		id := mcrWorkItemID(n)
		exec(`INSERT INTO work_item (id, created_on, updated_on, created_by, updated_by, number, wso2_id, subject, type,
				account_id, product_id, assigned_to_id)
			VALUES ($1, $2, $2, 't', 't', $3, $4, 'migration', 'ENGAGEMENT', $5, $6, $7)`,
			id, createdOn, "ENGMCR"+id[len(id)-2:], "ACME-MCR-"+id[len(id)-2:], mcrAccountID, mcrProductID, mcrAssigneeID)
		exec(`INSERT INTO engagement (id, state, type) VALUES ($1, $2::engagement_state_enum, $3::engagement_type_enum)`,
			id, state, engType)
		return id
	}
	tagged := func(workItemID, tagID string) {
		exec(`INSERT INTO work_item_tag (id, created_on, updated_on, work_item_id, tag_id)
			VALUES (gen_random_uuid(), now(), now(), $1, $2)`, workItemID, tagID)
	}
	comment := func(workItemID, author, kind string, at time.Time, deleted bool) {
		var deletedAt *time.Time
		if deleted {
			deletedAt = &now
		}
		exec(`INSERT INTO comment (id, created_on, created_by, type, work_item_id, content, deleted_at)
			VALUES (gen_random_uuid(), $1, $2, $3::comment_type_enum, $4, 'text', $5)`, at, author, kind, workItemID, deletedAt)
	}

	// Listed: oldest first by creation.
	hold := engagement(1, old.AddDate(0, 0, -3), "WORK_IN_PROGRESS", "MIGRATION")
	tagged(hold, mcrHoldTagID)
	waiting := engagement(2, old.AddDate(0, 0, -2), "AWAITING_INFO", "MIGRATION")
	tagged(waiting, mcrAccTagID)
	both := engagement(3, old.AddDate(0, 0, -1), "OPEN", "MIGRATION")
	tagged(both, mcrHoldTagID)
	tagged(both, mcrAccTagID)
	plain := engagement(4, old, "WORK_IN_PROGRESS", "MIGRATION")
	// A NULL state is kept: only the two concluded states are removed.
	exec(`UPDATE engagement SET state = NULL WHERE id = $1`, plain)
	customerAt := old.AddDate(0, 0, 30).Truncate(time.Second)
	comment(plain, mcrCustomerMail, "COMMENT", customerAt, false)
	comment(plain, mcrStaffMail, "COMMENT", customerAt.AddDate(0, 0, 1), false)                // staff, not the customer
	comment(plain, mcrCustomerMail, "WORK_NOTE", customerAt.AddDate(0, 0, 2), false)           // internal by type
	comment(plain, mcrCustomerMail, "COMMENT", customerAt.AddDate(0, 0, 3), true)              // deleted
	comment(plain, "nobody-known@example.test", "COMMENT", customerAt.AddDate(0, 0, 4), false) // unresolvable author

	// Not listed.
	engagement(5, now.AddDate(0, 0, -10), "WORK_IN_PROGRESS", "MIGRATION") // too young
	engagement(6, old, "CLOSED", "MIGRATION")
	engagement(7, old, "SOLUTION_PROPOSED", "MIGRATION")
	engagement(8, old, "WORK_IN_PROGRESS", "CONSULTANCY") // not a migration

	repo := repository.NewMigrationCaseReportRepository(repository.NewScoped(pool))
	sysCtx := repository.WithSystemIdentity(ctx)

	rows, err := repo.ListLongRunningMigrations(sysCtx, cutoff, "mcr_hold", "MCR_ACC")
	if err != nil {
		t.Fatalf("ListLongRunningMigrations: %v", err)
	}
	byID := map[string]domain.MigrationCaseReportRow{}
	var order []string
	for _, r := range rows {
		if _, dup := byID[r.ID]; dup {
			t.Errorf("row %s returned twice", r.Number)
		}
		byID[r.ID] = r
		order = append(order, r.ID)
	}
	if want := []string{hold, waiting, both, plain}; len(order) != len(want) || order[0] != want[0] || order[3] != want[3] {
		t.Fatalf("rows = %v, want exactly the four listed migrations oldest first %v", order, want)
	}

	h := byID[hold]
	if !h.OnHold || h.WaitingOnAccounts {
		t.Errorf("hold flags = %v/%v, want on hold only (tag name matched ignoring case)", h.OnHold, h.WaitingOnAccounts)
	}
	if h.Number != "ENGMCR01" || h.IssueKey != "ACME-MCR-01" {
		t.Errorf("hold identifiers = %q/%q", h.Number, h.IssueKey)
	}
	if h.AssignedTo != "Dana Example" || h.Product != "MCR Example Product" || h.Account != "Acme" || h.CsTeam != "Team Alpha" {
		t.Errorf("hold row = %+v: assignee, product, account or CS team is wrong", h)
	}
	if h.SalesRegion != "EU" || h.AccountManager != "Morgan Example" || h.TechnicalOwner != "Sam Example" {
		t.Errorf("hold account facts = %q/%q/%q: sales region, AM (first+last) or TO is wrong", h.SalesRegion, h.AccountManager, h.TechnicalOwner)
	}
	if h.LastCustomerCommentOn != nil {
		t.Errorf("hold has no comments, LastCustomerCommentOn = %v", h.LastCustomerCommentOn)
	}
	if w := byID[waiting]; w.OnHold || !w.WaitingOnAccounts {
		t.Errorf("waiting flags = %v/%v, want waiting only", w.OnHold, w.WaitingOnAccounts)
	}
	if b := byID[both]; !b.OnHold || !b.WaitingOnAccounts {
		t.Errorf("both flags = %v/%v, want both", b.OnHold, b.WaitingOnAccounts)
	}
	p := byID[plain]
	if p.OnHold || p.WaitingOnAccounts {
		t.Errorf("plain flags = %v/%v, want neither", p.OnHold, p.WaitingOnAccounts)
	}
	if p.LastCustomerCommentOn == nil || !p.LastCustomerCommentOn.Equal(customerAt) {
		t.Errorf("plain LastCustomerCommentOn = %v, want %v: only a surviving customer-visible comment by an external user counts", p.LastCustomerCommentOn, customerAt)
	}

	// The cut-off instant itself is included: a migration created exactly
	// then is listed. One created ten hours later on the same day is not;
	// it has not been open for the whole of the period yet.
	exec(`UPDATE work_item SET created_on = $1 WHERE id = $2`, cutoff, hold)
	rows, err = repo.ListLongRunningMigrations(sysCtx, cutoff, "mcr_hold", "mcr_acc")
	if err != nil {
		t.Fatalf("ListLongRunningMigrations (inclusive): %v", err)
	}
	if len(rows) != 4 {
		t.Errorf("rows after moving one to the cut-off instant = %d, want 4 (inclusive)", len(rows))
	}
	exec(`UPDATE work_item SET created_on = $1 WHERE id = $2`, cutoff.Add(10*time.Hour), hold)
	rows, err = repo.ListLongRunningMigrations(sysCtx, cutoff, "mcr_hold", "mcr_acc")
	if err != nil {
		t.Fatalf("ListLongRunningMigrations (later that day): %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("rows after moving one to later on the cut-off day = %d, want 3 (not yet old enough)", len(rows))
	}

	emails, err := repo.TeamGroupEmails(sysCtx, []string{"mcr-team-b", "mcr-team-a", "mcr-missing"})
	if err != nil {
		t.Fatalf("TeamGroupEmails: %v", err)
	}
	if len(emails) != 1 || emails[0] != "team-a@example.test" {
		t.Errorf("TeamGroupEmails = %v, want the one trimmed mail group; a team without one or unknown is skipped", emails)
	}
	if emails, err = repo.TeamGroupEmails(sysCtx, nil); err != nil || len(emails) != 0 {
		t.Errorf("TeamGroupEmails(nil) = %v, %v; want an empty list", emails, err)
	}
}
