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

package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
)

// MigrationCaseReportRepository reads the rows behind the weekly migration
// case scan. One query returns every candidate with its two tag flags; the
// service decides which table each row lands in, so the bucketing rule
// lives in one testable place rather than in three near-identical queries.
type MigrationCaseReportRepository interface {
	// ListLongRunningMigrations returns every migration engagement that is
	// neither closed nor awaiting the customer's sign-off and was created
	// on or before createdBefore, oldest first. holdTag and accountsTag
	// are the tag names (compared ignoring case) that set the row's OnHold
	// and WaitingOnAccounts flags.
	ListLongRunningMigrations(ctx context.Context, createdBefore time.Time, holdTag, accountsTag string) ([]domain.MigrationCaseReportRow, error)
	// TeamGroupEmails returns the mail groups of the named teams, in the
	// order given, skipping teams without one or that do not exist.
	TeamGroupEmails(ctx context.Context, teamKeys []string) ([]string, error)
}

type migrationCaseReportRepository struct {
	db *Scoped
}

// NewMigrationCaseReportRepository constructs the Postgres-backed
// MigrationCaseReportRepository. It takes *Scoped, not the bare pool:
// comment carries row-level security, and the report's reads run under
// the system identity the service sets.
func NewMigrationCaseReportRepository(db *Scoped) MigrationCaseReportRepository {
	return &migrationCaseReportRepository{db: db}
}

// userDisplayName is the name expression every list in this service shows
// for a person: the stored display name, else first and last name joined,
// else blank.
func userDisplayName(alias string) string {
	return "COALESCE(" + alias + ".name, NULLIF(TRIM(CONCAT_WS(' ', " + alias + ".first_name, " + alias + ".last_name)), ''), '')"
}

// listLongRunningMigrationsSQL is the scan. Notes on the joins:
//
//   - A migration is an engagement work item whose engagement type is
//     MIGRATION. Its state lives on the engagement row, not on the work
//     item. A NULL state is kept, as the source report kept it: the filter
//     only removes the two states that mean the migration has concluded.
//   - The CS team is the account's CRE team, a "group" row.
//   - The customer's last comment is the newest surviving customer-visible
//     comment whose author resolves to an external user. Work notes are
//     internal by definition and never count; neither does a comment by
//     staff or by the system. A comment whose author cannot be resolved to
//     a user at all is not counted either, so the column is blank rather
//     than wrong when authorship is unknown.
//   - Tag matching ignores case: tags are typed by people.
var listLongRunningMigrationsSQL = `
SELECT wi.id::text,
       wi.number,
       COALESCE(wi.wso2_id, ''),
       wi.created_on,
       lcc.last_customer_comment_on,
       ` + userDisplayName("ae") + `,
       COALESCE(prod.name, ''),
       COALESCE(a.name, ''),
       COALESCE(cre.name, ''),
       COALESCE(a.sales_region, ''),
       ` + userDisplayName("am") + `,
       ` + userDisplayName("tow") + `,
       EXISTS (SELECT 1 FROM work_item_tag wt JOIN tag t ON t.id = wt.tag_id
                WHERE wt.work_item_id = wi.id AND LOWER(t.name) = LOWER($2)),
       EXISTS (SELECT 1 FROM work_item_tag wt JOIN tag t ON t.id = wt.tag_id
                WHERE wt.work_item_id = wi.id AND LOWER(t.name) = LOWER($3))
  FROM work_item wi
  JOIN engagement e ON e.id = wi.id
  LEFT JOIN account a ON a.id = wi.account_id
  LEFT JOIN "group" cre ON cre.id = a.cre_team_id
  LEFT JOIN "user" ae ON ae.id = wi.assigned_to_id
  LEFT JOIN "user" am ON am.id = a.account_manager_id
  LEFT JOIN "user" tow ON tow.id = a.technical_owner_id
  LEFT JOIN product prod ON prod.id = wi.product_id
  LEFT JOIN LATERAL (
       SELECT MAX(c.created_on) AS last_customer_comment_on
         FROM comment c
         JOIN "user" cu ON LOWER(cu.email) = LOWER(c.created_by)
        WHERE c.work_item_id = wi.id
          AND c.type = 'COMMENT'
          AND c.deleted_at IS NULL
          AND cu.user_type = 'EXTERNAL'
  ) lcc ON TRUE
 WHERE wi.type = 'ENGAGEMENT'
   AND e.type = 'MIGRATION'
   AND (e.state IS NULL OR e.state NOT IN ('SOLUTION_PROPOSED', 'CLOSED'))
   AND wi.created_on <= $1
 ORDER BY wi.created_on, wi.number`

func (r *migrationCaseReportRepository) ListLongRunningMigrations(ctx context.Context, createdBefore time.Time, holdTag, accountsTag string) ([]domain.MigrationCaseReportRow, error) {
	rows, err := r.db.Query(ctx, listLongRunningMigrationsSQL, createdBefore, holdTag, accountsTag)
	if err != nil {
		return nil, fmt.Errorf("list long-running migrations: %w", err)
	}
	defer rows.Close()

	out := []domain.MigrationCaseReportRow{}
	for rows.Next() {
		var row domain.MigrationCaseReportRow
		if err := rows.Scan(
			&row.ID, &row.Number, &row.IssueKey, &row.CreatedOn, &row.LastCustomerCommentOn,
			&row.AssignedTo, &row.Product, &row.Account, &row.CsTeam, &row.SalesRegion,
			&row.AccountManager, &row.TechnicalOwner, &row.OnHold, &row.WaitingOnAccounts,
		); err != nil {
			return nil, fmt.Errorf("scan long-running migration: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate long-running migrations: %w", err)
	}
	return out, nil
}

func (r *migrationCaseReportRepository) TeamGroupEmails(ctx context.Context, teamKeys []string) ([]string, error) {
	if len(teamKeys) == 0 {
		return []string{}, nil
	}
	// array_position keeps the configured order, so the first configured
	// team is the first recipient; a stable order makes the audience
	// readable in logs and mails.
	rows, err := r.db.Query(ctx, `
		SELECT TRIM(t.group_email)
		  FROM team t
		 WHERE t.key = ANY($1::text[])
		   AND NULLIF(TRIM(t.group_email), '') IS NOT NULL
		 ORDER BY array_position($1::text[], t.key)`, teamKeys)
	if err != nil {
		return nil, fmt.Errorf("list team mail groups: %w", err)
	}
	defer rows.Close()

	out := []string{}
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, fmt.Errorf("scan team mail group: %w", err)
		}
		out = append(out, email)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate team mail groups: %w", err)
	}
	return out, nil
}
