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

// The rota import through the real service, repository and catalogue; skipped unless ENTITY_TEST_DATABASE_URL is set.

package service

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
)

const (
	rotaImportTeamID  = "7a1e0000-0000-4000-8000-000000000001"
	rotaImportTeamKey = "rotaimportfixture"
	rotaImportAdminID = "7a1e0000-0000-4000-8000-000000000002"
	rotaImportLeadID  = "7a1e0000-0000-4000-8000-000000000003"
	rotaImportUser1ID = "7a1e0000-0000-4000-8000-000000000004"
	rotaImportUser2ID = "7a1e0000-0000-4000-8000-000000000005"

	rotaImportAdminEmail = "rota-import-admin@example.com"
	rotaImportLeadEmail  = "rota-import-lead@example.com"
	rotaImportUser1Email = "rota-import-user1@example.com"
	rotaImportUser2Email = "rota-import-user2@example.com"

	// A week far enough ahead that a replace of the SaaS rota touches nothing real: Monday to Sunday.
	rotaImportFrom = "2031-03-03"
	rotaImportTo   = "2031-03-09"
)

func rotaImportExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func cleanRotaImportFixtures(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	users := []string{rotaImportAdminID, rotaImportLeadID, rotaImportUser1ID, rotaImportUser2ID}
	for _, stmt := range []string{
		`DELETE FROM team_schedule_assignment_activity WHERE user_id = ANY($1::uuid[])`,
		`DELETE FROM team_schedule_absence_activity WHERE user_id = ANY($1::uuid[])`,
		`DELETE FROM team_schedule_assignment WHERE user_id = ANY($1::uuid[])`,
		`DELETE FROM team_schedule_absence WHERE user_id = ANY($1::uuid[])`,
		`DELETE FROM team_member WHERE user_id = ANY($1::uuid[])`,
		`DELETE FROM user_role WHERE user_id = ANY($1::uuid[])`,
		`DELETE FROM "user" WHERE id = ANY($1::uuid[])`,
	} {
		rotaImportExec(t, pool, stmt, users)
	}
	rotaImportExec(t, pool, `DELETE FROM team WHERE id = $1`, rotaImportTeamID)
}

// newRotaImportIntegration seeds an SRE team on the SaaS rota with a lead and two engineers, and an SRE rota admin.
func newRotaImportIntegration(t *testing.T) (ScheduleService, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("ENTITY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ENTITY_TEST_DATABASE_URL is not set; skipping the live-database tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	cleanRotaImportFixtures(t, pool)
	t.Cleanup(func() { cleanRotaImportFixtures(t, pool) })

	rotaImportExec(t, pool, `
		INSERT INTO team (id, created_on, updated_on, created_by, updated_by, name, key, type)
		VALUES ($1, NOW(), NOW(), 'fixture', 'fixture', $2, $2, 'sre-abt')`, rotaImportTeamID, rotaImportTeamKey)
	for _, u := range []struct{ id, email string }{
		{rotaImportAdminID, rotaImportAdminEmail},
		{rotaImportLeadID, rotaImportLeadEmail},
		{rotaImportUser1ID, rotaImportUser1Email},
		{rotaImportUser2ID, rotaImportUser2Email},
	} {
		rotaImportExec(t, pool, `
			INSERT INTO "user" (id, created_on, updated_on, created_by, updated_by, user_name, first_name, last_name, email)
			VALUES ($1, NOW(), NOW(), 'fixture', 'fixture', $2, 'Rota', 'Import', $2)`, u.id, u.email)
	}
	for _, id := range []string{rotaImportLeadID, rotaImportUser1ID, rotaImportUser2ID} {
		rotaImportExec(t, pool, `
			INSERT INTO team_member (id, created_on, updated_on, created_by, updated_by, team_id, user_id, role)
			VALUES (gen_random_uuid(), NOW(), NOW(), 'fixture', 'fixture', $1, $2, 'lead')`, rotaImportTeamID, id)
	}
	rotaImportExec(t, pool, `
		INSERT INTO role (id, created_on, updated_on, created_by, updated_by, name, description)
		VALUES (gen_random_uuid(), NOW(), NOW(), 'fixture', 'fixture', 'internal', 'Internal staff')
		ON CONFLICT (name) DO NOTHING`)
	rotaImportExec(t, pool, `
		INSERT INTO user_role (id, created_on, updated_on, created_by, updated_by, user_id, role_id)
		SELECT gen_random_uuid(), NOW(), NOW(), 'fixture', 'fixture', $1, r.id
		  FROM role r WHERE r.name IN ('sre_rota_admin', 'internal')`, rotaImportAdminID)

	return NewScheduleService(repository.NewScheduleRepository(pool), alwaysUnrestrictedAccess{}), pool
}

// rotaImportStandardFile is the standard structure on the fixture team.
func rotaImportStandardFile(mode string) domain.ImportRotaRequest {
	l1, l2 := "L1", "L2"
	return domain.ImportRotaRequest{
		SchemaVersion: 1,
		Family:        "SRE",
		RotaCode:      "SRE_SAAS",
		Period:        domain.ImportRotaPeriod{From: rotaImportFrom, To: rotaImportTo},
		Mode:          mode,
		Assignments: []domain.ImportRotaAssignment{
			{Email: rotaImportUser1Email, TeamKey: rotaImportTeamKey, ShiftCode: "SRE_TZ1_L1", From: rotaImportFrom, To: rotaImportTo},
			{Email: rotaImportUser2Email, TeamKey: rotaImportTeamKey, ShiftCode: "SRE_TZ1", Tier: &l2, From: rotaImportFrom, To: "2031-03-07"},
			{Email: rotaImportUser2Email, TeamKey: rotaImportTeamKey, ShiftCode: "SRE_WE_TZ1", Tier: &l1, From: "2031-03-08", To: rotaImportTo},
		},
		Absences: []domain.ImportRotaAbsence{
			{Email: rotaImportLeadEmail, TeamKey: rotaImportTeamKey, KindCode: "ANNUAL_LEAVE", From: "2031-03-05", To: "2031-03-06"},
		},
	}
}

func TestScheduleImportIntegration_TheStandardFileLandsOnTheSaaSRota(t *testing.T) {
	svc, pool := newRotaImportIntegration(t)
	ctx := leadCtx(rotaImportAdminEmail)

	preview, err := svc.ImportRota(ctx, "SRE_SAAS", rotaImportStandardFile("merge"), true)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	// L1 on the 5 weekdays (the weekend skipped), L2 on the 5 weekdays, TZ1+2 on the 2 weekend days.
	if preview.Applied != 12 || preview.Skipped != 2 || preview.AbsencesCreated != 1 || len(preview.SkippedDays) != 1 || preview.SkippedDays[0].Index != 0 {
		t.Fatalf("dry run %+v", preview)
	}
	var before int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM team_schedule_assignment WHERE team_key = $1`, rotaImportTeamKey).Scan(&before); err != nil || before != 0 {
		t.Fatalf("the dry run wrote %d rows (%v)", before, err)
	}

	res, err := svc.ImportRota(ctx, "SRE_SAAS", rotaImportStandardFile("merge"), false)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.Applied != preview.Applied || res.Skipped != preview.Skipped || res.AbsencesCreated != preview.AbsencesCreated {
		t.Errorf("import %+v differs from its dry run %+v", res, preview)
	}

	var rows, l1, l2, weekend, leave int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*),
		       count(*) FILTER (WHERE s.code = 'SRE_TZ1_L1' AND a.tier::text = 'L1' AND a.user_id = $4::uuid),
		       count(*) FILTER (WHERE s.code = 'SRE_TZ1' AND a.tier::text = 'L2' AND a.user_id = $5::uuid),
		       count(*) FILTER (WHERE s.code = 'SRE_WE_TZ1' AND a.user_id = $5::uuid)
		  FROM team_schedule_assignment a JOIN team_schedule_shift s ON s.id = a.shift_id
		 WHERE a.team_key = $1 AND a.rota_date BETWEEN $2::date AND $3::date`,
		rotaImportTeamKey, rotaImportFrom, rotaImportTo, rotaImportUser1ID, rotaImportUser2ID).Scan(&rows, &l1, &l2, &weekend); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if rows != 12 || l1 != 5 || l2 != 5 || weekend != 2 {
		t.Errorf("stored %d rows (L1 %d, L2 %d, weekend %d), want 12 (5, 5, 2)", rows, l1, l2, weekend)
	}
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM team_schedule_absence
		 WHERE user_id = $1::uuid AND starts_on = '2031-03-05' AND ends_on = '2031-03-06'`, rotaImportLeadID).Scan(&leave); err != nil || leave != 1 {
		t.Errorf("%d leave spans stored (%v), want 1", leave, err)
	}
}

func TestScheduleImportIntegration_OnlyARotaAdminReplacesTheWholeRota(t *testing.T) {
	svc, _ := newRotaImportIntegration(t)

	_, err := svc.ImportRota(leadCtx(rotaImportLeadEmail), "SRE_SAAS", rotaImportStandardFile("replace"), true)
	var forbidden *apierror.ForbiddenError
	if !errors.As(err, &forbidden) {
		t.Fatalf("a lead of one SaaS team replaced the whole SaaS rota: %v", err)
	}
	if _, err := svc.ImportRota(leadCtx(rotaImportLeadEmail), "SRE_SAAS", rotaImportStandardFile("merge"), true); err != nil {
		t.Fatalf("the lead merging their own team's rows: %v", err)
	}
	if _, err := svc.ImportRota(leadCtx(rotaImportAdminEmail), "SRE_SAAS", rotaImportStandardFile("replace"), true); err != nil {
		t.Fatalf("the SRE rota admin replacing the SaaS rota: %v", err)
	}
}
