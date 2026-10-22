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

package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func marshalDeployedProduct(t *testing.T, v DeployedProductView) (string, map[string]any) {
	t.Helper()
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("output is not a JSON object: %v", err)
	}
	return string(out), m
}

// The deployment's type is nested in "deployment" next to its id and name; it is
// not a field of its own, and "deployment" appears exactly once.
func TestDeployedProductView_NestsDeploymentType(t *testing.T) {
	qa := DeploymentTypeQA
	raw, m := marshalDeployedProduct(t, DeployedProductView{
		ID:             "dp1",
		Deployment:     EntityRef{ID: "d1", Name: "QA env"},
		DeploymentType: &qa,
	})

	dep, ok := m["deployment"].(map[string]any)
	if !ok {
		t.Fatalf("deployment is not an object: %s", raw)
	}
	if dep["id"] != "d1" || dep["name"] != "QA env" || dep["type"] != "qa" {
		t.Errorf("deployment = %v, want id d1, name QA env, type qa", dep)
	}
	if n := strings.Count(raw, `"deployment"`); n != 1 {
		t.Errorf(`"deployment" appears %d times, want 1: %s`, n, raw)
	}
	if _, has := m["DeploymentType"]; has {
		t.Errorf("DeploymentType leaked as its own field: %s", raw)
	}
	// the rest of the view is still written
	for _, k := range []string{"id", "product", "version", "cores", "tps", "category", "projectSfId", "active", "description", "updates", "createdOn", "updatedOn"} {
		if _, has := m[k]; !has {
			t.Errorf("missing %q: %s", k, raw)
		}
	}
}

// A deployment with no type (or a data source that does not say) gives null.
func TestDeployedProductView_DeploymentTypeNull(t *testing.T) {
	_, m := marshalDeployedProduct(t, DeployedProductView{ID: "dp1", Deployment: EntityRef{ID: "d1", Name: "n"}})
	dep := m["deployment"].(map[string]any)
	if v, has := dep["type"]; !has || v != nil {
		t.Errorf("type = %v (present=%v), want null", v, has)
	}
}
