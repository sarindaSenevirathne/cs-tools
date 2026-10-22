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
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
)

// TestOnboardingOwnerRef covers the NULL-handling of the joined owner columns:
// a project with no owner must map to a nil PersonRef (JSON null), and an owner
// with a blank name or NULL email must still be returned.
func TestOnboardingOwnerRef(t *testing.T) {
	const id = "00000000-0000-0000-0000-000000000001"

	if got := onboardingOwnerRef(nil, nil, nil); got != nil {
		t.Errorf("all-NULL owner = %+v, want nil", got)
	}
	if got := onboardingOwnerRef(strp(""), strp("Jane Doe"), nil); got != nil {
		t.Errorf("empty owner id = %+v, want nil", got)
	}

	full := onboardingOwnerRef(strp(id), strp("Jane Doe"), strp("jane.doe@example.com"))
	if full == nil || full.ID != id || full.Name != "Jane Doe" || full.Email == nil || *full.Email != "jane.doe@example.com" {
		t.Errorf("full owner = %+v", full)
	}

	noEmail := onboardingOwnerRef(strp(id), strp("Jane Doe"), nil)
	if noEmail == nil || noEmail.Email != nil || noEmail.Name != "Jane Doe" {
		t.Errorf("null-email owner = %+v, want owner with nil email", noEmail)
	}

	blankName := onboardingOwnerRef(strp(id), nil, strp("jane.doe@example.com"))
	if blankName == nil || blankName.ID != id || blankName.Name != "" {
		t.Errorf("blank-name owner = %+v, want owner with empty name", blankName)
	}
}

// TestUpdateProject_RejectsInvalidOnboardingStatus pins that a bad status is a
// ValidationError raised before any database access (the repo has no pool).
func TestUpdateProject_RejectsInvalidOnboardingStatus(t *testing.T) {
	bad := "Paused"
	r := &projectRepo{}
	_, err := r.UpdateProject(context.Background(), "00000000-0000-0000-0000-000000000001", domain.ProjectUpdateRequest{OnboardingStatus: &bad}, "jane.doe@example.com")
	var ve *apierror.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v, want ValidationError", err)
	}
	for _, ok := range []string{"In-Progress", "IN_PROGRESS", "in progress", "on-hold", "OnHold"} {
		if _, err := onboardingStatusEnumLabels("onboardingStatus", []string{ok}); err != nil {
			t.Errorf("%q rejected: %v", ok, err)
		}
	}
}
