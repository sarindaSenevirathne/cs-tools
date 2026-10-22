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
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
)

// RunbookRepository reads and writes the runbook catalog (migrations 0241-0244).
type RunbookRepository interface {
	// List is every runbook with its alerts, newest number first.
	List(ctx context.Context) ([]domain.Runbook, error)
	// Create adds a runbook with the next number; a ConflictError when its name or one of its alerts is taken.
	Create(ctx context.Context, actor string, req domain.SaveRunbookRequest) (domain.Runbook, error)
	// Update replaces a runbook's name, link and alerts; nil when there is none with that id, a ConflictError when the name or an
	// alert is taken.
	Update(ctx context.Context, actor, id string, req domain.SaveRunbookRequest) (*domain.Runbook, error)
	// Delete removes a runbook and its alerts; false when there was none.
	Delete(ctx context.Context, id string) (bool, error)
	// Import applies every entry in one transaction: an entry replaces the runbook that holds its first listed alert, or is
	// created; its alerts leave any other runbook, and a runbook left without alerts is removed and listed in the response.
	Import(ctx context.Context, actor string, reqs []domain.SaveRunbookRequest) (domain.ImportRunbooksResponse, error)

}

type runbookRepo struct {
	db *pgxpool.Pool
}

// NewRunbookRepository constructs a Postgres-backed RunbookRepository.
func NewRunbookRepository(db *pgxpool.Pool) RunbookRepository {
	return &runbookRepo{db: db}
}

const runbookSelect = `SELECT r.id::text, r.number, r.name, r.runbook_url,
	COALESCE((SELECT array_agg(a.alert_name ORDER BY lower(a.alert_name)) FROM runbook_alert a WHERE a.runbook_id = r.id), '{}'),
	r.created_on, r.created_by, r.updated_on, r.updated_by
	FROM runbook r`

func scanRunbook(row pgx.Row) (domain.Runbook, error) {
	var rb domain.Runbook
	err := row.Scan(&rb.ID, &rb.Number, &rb.Name, &rb.RunbookURL, &rb.Alerts, &rb.CreatedOn, &rb.CreatedBy, &rb.UpdatedOn, &rb.UpdatedBy)
	return rb, err
}

func (r *runbookRepo) List(ctx context.Context) ([]domain.Runbook, error) {
	rows, err := r.db.Query(ctx, runbookSelect+` ORDER BY length(r.number) DESC, r.number DESC`)
	if err != nil {
		return nil, fmt.Errorf("list runbooks: %w", err)
	}
	defer rows.Close()
	out := []domain.Runbook{}
	for rows.Next() {
		rb, err := scanRunbook(rows)
		if err != nil {
			return nil, fmt.Errorf("list runbooks: scan: %w", err)
		}
		out = append(out, rb)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list runbooks: %w", err)
	}
	return out, nil
}

func (r *runbookRepo) Create(ctx context.Context, actor string, req domain.SaveRunbookRequest) (domain.Runbook, error) {
	var out domain.Runbook
	err := r.inTx(ctx, "create runbook", func(tx pgx.Tx) error {
		var id string
		if err := tx.QueryRow(ctx,
			`INSERT INTO runbook (name, runbook_url, created_by, updated_by) VALUES ($1, $2, $3, $3) RETURNING id::text`,
			req.Name, req.RunbookURL, actor).Scan(&id); err != nil {
			return runbookNameErr(err, req.Name)
		}
		if err := setRunbookAlertsTx(ctx, tx, id, req.Alerts); err != nil {
			return err
		}
		rb, err := scanRunbook(tx.QueryRow(ctx, runbookSelect+` WHERE r.id = $1::uuid`, id))
		out = rb
		return err
	})
	return out, err
}

func (r *runbookRepo) Update(ctx context.Context, actor, id string, req domain.SaveRunbookRequest) (*domain.Runbook, error) {
	var out *domain.Runbook
	err := r.inTx(ctx, "update runbook", func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`UPDATE runbook SET name = $2, runbook_url = $3, updated_by = $4, updated_on = NOW() WHERE id = $1::uuid`,
			id, req.Name, req.RunbookURL, actor)
		if err != nil {
			return runbookNameErr(err, req.Name)
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		if err := setRunbookAlertsTx(ctx, tx, id, req.Alerts); err != nil {
			return err
		}
		rb, err := scanRunbook(tx.QueryRow(ctx, runbookSelect+` WHERE r.id = $1::uuid`, id))
		out = &rb
		return err
	})
	return out, err
}

func (r *runbookRepo) Delete(ctx context.Context, id string) (bool, error) {
	tag, err := r.db.Exec(ctx, `DELETE FROM runbook WHERE id = $1::uuid`, id)
	if err != nil {
		return false, fmt.Errorf("delete runbook: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (r *runbookRepo) Import(ctx context.Context, actor string, reqs []domain.SaveRunbookRequest) (domain.ImportRunbooksResponse, error) {
	out := domain.ImportRunbooksResponse{Removed: []domain.RemovedRunbook{}}
	var all []string
	for _, req := range reqs {
		all = append(all, req.Alerts...)
	}
	err := r.inTx(ctx, "import runbooks", func(tx pgx.Tx) error {
		// Lock every runbook that holds an imported alert, in id order, so an edit or another import waits instead of
		// emptying one of them behind this import's back, and two imports cannot deadlock.
		var owners []string
		rows, err := tx.Query(ctx,
			`SELECT id::text FROM runbook WHERE id IN (
			   SELECT runbook_id FROM runbook_alert WHERE lower(btrim(alert_name)) = ANY($1::text[])
			 ) ORDER BY id FOR UPDATE`,
			lowerAll(all))
		if err != nil {
			return err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			owners = append(owners, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		for _, req := range reqs {
			// The runbook holding the entry's first listed alert takes the entry; an entry none of whose alerts is listed is new.
			var id string
			err := tx.QueryRow(ctx,
				`SELECT a.runbook_id::text FROM unnest($1::text[]) WITH ORDINALITY e(alert, pos)
				 JOIN runbook_alert a ON lower(btrim(a.alert_name)) = e.alert
				 ORDER BY e.pos LIMIT 1`,
				lowerAll(req.Alerts)).Scan(&id)
			switch {
			case errors.Is(err, pgx.ErrNoRows):
				// A new runbook with no name in the file is named after its first alert.
				name := req.Name
				if name == "" {
					name = req.Alerts[0]
				}
				if err := tx.QueryRow(ctx,
					`INSERT INTO runbook (name, runbook_url, created_by, updated_by) VALUES ($1, $2, $3, $3) RETURNING id::text`,
					name, req.RunbookURL, actor).Scan(&id); err != nil {
					return runbookNameErr(err, name)
				}
				out.Created++
			case err != nil:
				return err
			default:
				// An entry with no name keeps the matched runbook's name.
				if _, err := tx.Exec(ctx,
					`UPDATE runbook SET name = COALESCE(NULLIF($4, ''), name), runbook_url = $2, updated_by = $3, updated_on = NOW()
					 WHERE id = $1::uuid`,
					id, req.RunbookURL, actor, req.Name); err != nil {
					return runbookNameErr(err, req.Name)
				}
				out.Updated++
			}
			// The entry's alerts are the runbook's alerts now, wherever they were listed before.
			if _, err := tx.Exec(ctx,
				`DELETE FROM runbook_alert WHERE runbook_id = $1::uuid OR lower(btrim(alert_name)) = ANY($2::text[])`,
				id, lowerAll(req.Alerts)); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx,
				`INSERT INTO runbook_alert (runbook_id, alert_name) SELECT $1::uuid, unnest($2::text[])`, id, req.Alerts); err != nil {
				return err
			}
		}

		// A locked runbook whose alerts all moved to other entries is removed; the response names it and its link.
		removed, err := tx.Query(ctx,
			`DELETE FROM runbook r WHERE r.id = ANY($1::uuid[])
			   AND NOT EXISTS (SELECT 1 FROM runbook_alert a WHERE a.runbook_id = r.id)
			 RETURNING r.number, r.name, r.runbook_url`,
			owners)
		if err != nil {
			return err
		}
		defer removed.Close()
		for removed.Next() {
			var rb domain.RemovedRunbook
			if err := removed.Scan(&rb.Number, &rb.Name, &rb.RunbookURL); err != nil {
				return err
			}
			out.Removed = append(out.Removed, rb)
		}
		if err := removed.Err(); err != nil {
			return err
		}
		sort.Slice(out.Removed, func(i, j int) bool { return out.Removed[i].Number < out.Removed[j].Number })
		return nil
	})
	return out, err
}

// setRunbookAlertsTx replaces a runbook's alerts; an alert listed under another runbook is a ConflictError.
func setRunbookAlertsTx(ctx context.Context, tx pgx.Tx, id string, alerts []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM runbook_alert WHERE runbook_id = $1::uuid`, id); err != nil {
		return err
	}
	var alert, owner string
	err := tx.QueryRow(ctx,
		`SELECT a.alert_name, r.number FROM runbook_alert a JOIN runbook r ON r.id = a.runbook_id
		 WHERE lower(btrim(a.alert_name)) = ANY($1::text[]) ORDER BY lower(a.alert_name) LIMIT 1`,
		lowerAll(alerts)).Scan(&alert, &owner)
	if err == nil {
		return &apierror.ConflictError{Msg: "alert " + alert + " already belongs to runbook " + owner}
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO runbook_alert (runbook_id, alert_name) SELECT $1::uuid, unnest($2::text[])`, id, alerts); err != nil {
		if isUniqueViolation(err) {
			return &apierror.ConflictError{Msg: "one of the alerts already belongs to another runbook"}
		}
		return err
	}
	return nil
}

// runbookNameErr turns a refusal of the unique runbook name into a ConflictError naming it; any other error passes through.
func runbookNameErr(err error, name string) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "runbook_name_key" {
		return &apierror.ConflictError{Msg: "a runbook named " + name + " already exists"}
	}
	return err
}

// inTx runs fn in a transaction; a ConflictError or ValidationError passes through unwrapped so the handler can map it.
func (r *runbookRepo) inTx(ctx context.Context, op string, fn func(tx pgx.Tx) error) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("%s: begin tx: %w", op, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		var conflict *apierror.ConflictError
		var invalid *apierror.ValidationError
		if errors.As(err, &conflict) || errors.As(err, &invalid) {
			return err
		}
		return fmt.Errorf("%s: %w", op, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("%s: commit: %w", op, err)
	}
	return nil
}
