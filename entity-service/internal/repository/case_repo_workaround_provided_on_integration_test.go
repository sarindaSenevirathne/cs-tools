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

// Round trip for CaseRepository.GetCaseWorkaroundProvidedOn: marking a
// workaround provided stamps work_item.workaround_provided_on, recalling it
// clears it, and an unknown id reads as nil. Runs against a real Postgres.
// Skipped without CASE_STATS_TEST_DSN.
//
//	CASE_STATS_TEST_DSN=postgres://... go test ./internal/repository/ -run WorkaroundProvidedOnIntegration

package repository_test

import (
	"context"
	"testing"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
)

func TestWorkaroundProvidedOnIntegration_SetRecallAndUnknownID(t *testing.T) {
	pool := caseStatsPool(t)
	seedClosedByFixture(t, pool)
	repo := repository.NewCaseRepository(repository.NewScoped(pool))
	ctx := repository.WithSystemIdentity(context.Background())

	got, err := repo.GetCaseWorkaroundProvidedOn(ctx, cbCaseID)
	if err != nil || got != nil {
		t.Fatalf("before any workaround: got %v, err %v, want nil, nil", got, err)
	}

	provided := true
	if _, err := repo.UpdateCaseFields(ctx, domain.UpdateCaseRequest{ID: cbCaseID, WorkaroundProvided: &provided}, cbCloserUserID, "jane.doe@example.com"); err != nil {
		t.Fatalf("UpdateCaseFields(set): %v", err)
	}
	got, err = repo.GetCaseWorkaroundProvidedOn(ctx, cbCaseID)
	if err != nil || got == nil {
		t.Fatalf("after set: got %v, err %v, want a timestamp", got, err)
	}

	provided = false
	if _, err := repo.UpdateCaseFields(ctx, domain.UpdateCaseRequest{ID: cbCaseID, WorkaroundProvided: &provided}, "", "jane.doe@example.com"); err != nil {
		t.Fatalf("UpdateCaseFields(recall): %v", err)
	}
	got, err = repo.GetCaseWorkaroundProvidedOn(ctx, cbCaseID)
	if err != nil || got != nil {
		t.Fatalf("after recall: got %v, err %v, want nil, nil", got, err)
	}

	got, err = repo.GetCaseWorkaroundProvidedOn(ctx, "00000000-0000-0000-0000-000000000000")
	if err != nil || got != nil {
		t.Fatalf("unknown id: got %v, err %v, want nil, nil", got, err)
	}
}
