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
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"golang.org/x/sync/errgroup"
)

// PendingVerificationRepository defines the persistence operations for the
// pending_verification table — see domain.PendingVerification's doc comment
// for what it's for.
type PendingVerificationRepository interface {
	// Create inserts a new entry. WorkItemID must reference an existing
	// work_item row — a foreign-key violation surfaces as a
	// *apierror.ValidationError.
	Create(ctx context.Context, req domain.CreatePendingVerificationRequest) (domain.PendingVerification, error)
	// Search returns the page of entries matching req.Filters, plus the
	// counts SearchPendingVerificationsResponse needs for the summary tiles
	// and the type-filter dropdown.
	Search(ctx context.Context, req domain.SearchPendingVerificationsRequest) (domain.SearchPendingVerificationsResponse, error)
	// Verify sets verified_on/verified_by for the entry identified by id.
	// Returns a *apierror.NotFoundError if no such entry exists, or if it is
	// already verified (verifying twice is not idempotent here — the
	// service layer surfaces this distinctly from a genuinely missing id).
	Verify(ctx context.Context, req domain.VerifyPendingVerificationRequest) (domain.PendingVerification, error)
}

type pendingVerificationRepo struct {
	db *pgxpool.Pool
}

// NewPendingVerificationRepository constructs a PendingVerificationRepository
// backed by the given connection pool.
func NewPendingVerificationRepository(db *pgxpool.Pool) PendingVerificationRepository {
	return &pendingVerificationRepo{db: db}
}

// pendingVerificationColumns is the column list shared by every query that
// returns a full row, kept in one place so Create/Verify/Search can't drift
// out of sync with scanPendingVerification's field order. severity is
// LEFT-joined from "case" — the only extension table with a severity column
// — and is NULL for every non-Case record type, matching
// domain.PendingVerification.Severity being optional.
const pendingVerificationColumns = `pv.id, pv.work_item_id, wi.number, wi.subject, c.severity::TEXT, wi.type::TEXT,
	pv.added_reason::TEXT, pv.previous_status::TEXT, pv.added_on, pv.added_by,
	pv.verified_on, pv.verified_by, pv.note`

const pendingVerificationFromJoin = `
	FROM pending_verification pv
	JOIN work_item wi ON pv.work_item_id = wi.id
	LEFT JOIN "case" c ON wi.id = c.id`

func scanPendingVerification(row pgx.Row) (domain.PendingVerification, error) {
	var pv domain.PendingVerification
	var severity *string
	var previousStatus *string
	if err := row.Scan(
		&pv.ID, &pv.WorkItemID, &pv.RecordNumber, &pv.RecordTitle, &severity, &pv.RecordType,
		&pv.AddedReason, &previousStatus, &pv.AddedOn, &pv.AddedBy,
		&pv.VerifiedOn, &pv.VerifiedBy, &pv.Note,
	); err != nil {
		return domain.PendingVerification{}, err
	}
	pv.Severity = severity
	if previousStatus != nil {
		s := domain.PendingVerificationPreviousStatus(*previousStatus)
		pv.PreviousStatus = &s
	}
	return pv, nil
}

// Create implements PendingVerificationRepository.
func (r *pendingVerificationRepo) Create(ctx context.Context, req domain.CreatePendingVerificationRequest) (domain.PendingVerification, error) {
	query := `
		INSERT INTO pending_verification (work_item_id, added_reason, previous_status, added_by, note)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`

	var id string
	err := r.db.QueryRow(ctx, query, req.WorkItemID, req.AddedReason, req.PreviousStatus, req.AddedBy, req.Note).Scan(&id)
	if err != nil {
		if pgErr := (*pgconn.PgError)(nil); errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return domain.PendingVerification{}, &apierror.ValidationError{Msg: "workItemId does not reference an existing record"}
		}
		return domain.PendingVerification{}, fmt.Errorf("create pending_verification: %w", err)
	}

	return r.getByID(ctx, id)
}

func (r *pendingVerificationRepo) getByID(ctx context.Context, id string) (domain.PendingVerification, error) {
	query := `SELECT ` + pendingVerificationColumns + pendingVerificationFromJoin + ` WHERE pv.id = $1`
	pv, err := scanPendingVerification(r.db.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.PendingVerification{}, &apierror.NotFoundError{Msg: "pending_verification not found"}
		}
		return domain.PendingVerification{}, fmt.Errorf("get pending_verification: %w", err)
	}
	return pv, nil
}

// Verify implements PendingVerificationRepository.
func (r *pendingVerificationRepo) Verify(ctx context.Context, req domain.VerifyPendingVerificationRequest) (domain.PendingVerification, error) {
	query := `
		UPDATE pending_verification
		SET verified_on = NOW(), verified_by = $2
		WHERE id = $1 AND verified_on IS NULL
		RETURNING id`

	var id string
	err := r.db.QueryRow(ctx, query, req.ID, req.VerifiedBy).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.PendingVerification{}, &apierror.NotFoundError{Msg: "pending_verification not found, or already verified"}
		}
		return domain.PendingVerification{}, fmt.Errorf("verify pending_verification: %w", err)
	}

	return r.getByID(ctx, id)
}

// buildSearchWhere builds the shared WHERE clause (and its args) for every
// branch of Search below. includeTypeFilter is false when computing the
// type-filter dropdown's own per-type counts, since those must reflect every
// other active filter without being narrowed by the type filter itself.
func buildSearchWhere(f domain.PendingVerificationSearchFilters, includeTypeFilter bool) (string, []any) {
	where := "WHERE wi.project_id = $1"
	args := []any{f.ProjectID}
	argIdx := 2

	if f.VerifiedOnly {
		where += " AND pv.verified_on IS NOT NULL"
	} else if !f.IncludeVerified {
		where += " AND pv.verified_on IS NULL"
	}
	if f.WorkItemID != nil {
		where += fmt.Sprintf(" AND pv.work_item_id = $%d", argIdx)
		args = append(args, *f.WorkItemID)
		argIdx++
	}
	if includeTypeFilter && len(f.WorkItemTypes) > 0 {
		where += fmt.Sprintf(" AND wi.type = ANY($%d::work_item_type_enum[])", argIdx)
		args = append(args, f.WorkItemTypes)
		argIdx++
	}
	if f.SearchQuery != "" {
		escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(f.SearchQuery)
		pattern := "%" + escaped + "%"
		where += fmt.Sprintf(" AND (wi.number ILIKE $%d ESCAPE '\\' OR wi.subject ILIKE $%d ESCAPE '\\' OR wi.description ILIKE $%d ESCAPE '\\')", argIdx, argIdx, argIdx)
		args = append(args, pattern)
		argIdx++
	}
	return where, args
}

// Search implements PendingVerificationRepository.
func (r *pendingVerificationRepo) Search(ctx context.Context, req domain.SearchPendingVerificationsRequest) (domain.SearchPendingVerificationsResponse, error) {
	where, args := buildSearchWhere(req.Filters, true)
	typeCountsWhere, typeCountsArgs := buildSearchWhere(req.Filters, false)
	dedupe := req.Filters.VerifiedOnly

	var entries []domain.PendingVerification
	var total, autoClosedCount, manualCount int
	var typeCounts []domain.PendingVerificationTypeCount

	eg, egCtx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		var dataQuery string
		if dedupe {
			// One row per work_item_id — its most-recently-verified entry —
			// so a case verified more than once (VerifiedOnly's whole reason
			// to exist: the Verified tab, post re-add-after-verified) shows
			// as a single card, not one per historical round. DISTINCT ON's
			// own required ordering picks *which* row wins per group; it
			// does not control final display order, hence the outer re-sort.
			dataQuery = fmt.Sprintf(`
				SELECT * FROM (
					SELECT DISTINCT ON (pv.work_item_id) %s
					%s %s
					ORDER BY pv.work_item_id, pv.verified_on DESC
				) latest
				ORDER BY verified_on DESC
				LIMIT $%d OFFSET $%d`,
				pendingVerificationColumns, pendingVerificationFromJoin, where, len(args)+1, len(args)+2)
		} else {
			dataQuery = fmt.Sprintf(`
				SELECT %s %s %s
				ORDER BY pv.added_on DESC
				LIMIT $%d OFFSET $%d`,
				pendingVerificationColumns, pendingVerificationFromJoin, where, len(args)+1, len(args)+2)
		}
		dataArgs := append(append([]any{}, args...), req.Pagination.Limit, req.Pagination.Offset)

		rows, err := r.db.Query(egCtx, dataQuery, dataArgs...)
		if err != nil {
			return fmt.Errorf("query pending_verifications: %w", err)
		}
		defer rows.Close()

		result := make([]domain.PendingVerification, 0, req.Pagination.Limit)
		for rows.Next() {
			pv, err := scanPendingVerification(rows)
			if err != nil {
				return fmt.Errorf("scan pending_verification: %w", err)
			}
			result = append(result, pv)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate pending_verifications: %w", err)
		}
		entries = result
		return nil
	})

	eg.Go(func() error {
		var countQuery string
		if dedupe {
			// Same one-row-per-work-item set as the data query above, so
			// total/autoClosedCount/manualCount agree with what the rows
			// themselves show — each work item's count attributed to its
			// own latest cycle's own addedReason.
			countQuery = fmt.Sprintf(`
				WITH latest AS (
					SELECT DISTINCT ON (pv.work_item_id) pv.added_reason
					%s %s
					ORDER BY pv.work_item_id, pv.verified_on DESC
				)
				SELECT COUNT(*),
				       COUNT(*) FILTER (WHERE added_reason = 'AUTO_CLOSED'),
				       COUNT(*) FILTER (WHERE added_reason = 'MANUAL')
				FROM latest`, pendingVerificationFromJoin, where)
		} else {
			countQuery = fmt.Sprintf(`
				SELECT COUNT(*),
				       COUNT(*) FILTER (WHERE pv.added_reason = 'AUTO_CLOSED'),
				       COUNT(*) FILTER (WHERE pv.added_reason = 'MANUAL')
				%s %s`, pendingVerificationFromJoin, where)
		}
		if err := r.db.QueryRow(egCtx, countQuery, args...).Scan(&total, &autoClosedCount, &manualCount); err != nil {
			return fmt.Errorf("count pending_verifications: %w", err)
		}
		return nil
	})

	eg.Go(func() error {
		var typeCountsQuery string
		if dedupe {
			typeCountsQuery = fmt.Sprintf(`
				WITH latest AS (
					SELECT DISTINCT ON (pv.work_item_id) wi.type
					%s %s
					ORDER BY pv.work_item_id, pv.verified_on DESC
				)
				SELECT type::TEXT, COUNT(*) FROM latest GROUP BY type`,
				pendingVerificationFromJoin, typeCountsWhere)
		} else {
			typeCountsQuery = fmt.Sprintf(`
				SELECT wi.type::TEXT, COUNT(*)
				%s %s
				GROUP BY wi.type`, pendingVerificationFromJoin, typeCountsWhere)
		}
		rows, err := r.db.Query(egCtx, typeCountsQuery, typeCountsArgs...)
		if err != nil {
			return fmt.Errorf("count pending_verifications by type: %w", err)
		}
		defer rows.Close()

		result := make([]domain.PendingVerificationTypeCount, 0)
		for rows.Next() {
			var tc domain.PendingVerificationTypeCount
			if err := rows.Scan(&tc.RecordType, &tc.Count); err != nil {
				return fmt.Errorf("scan pending_verification type count: %w", err)
			}
			result = append(result, tc)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate pending_verification type counts: %w", err)
		}
		typeCounts = result
		return nil
	})

	if err := eg.Wait(); err != nil {
		return domain.SearchPendingVerificationsResponse{}, err
	}

	return domain.SearchPendingVerificationsResponse{
		PendingVerifications: entries,
		Total:                total,
		AutoClosedCount:      autoClosedCount,
		ManualCount:          manualCount,
		TypeCounts:           typeCounts,
		Limit:                req.Pagination.Limit,
		Offset:               req.Pagination.Offset,
		HasMore:              req.Pagination.Offset+len(entries) < total,
	}, nil
}
