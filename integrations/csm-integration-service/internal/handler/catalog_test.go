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

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

type mockCatalogClient struct {
	products         func(body []byte) ([]byte, error)
	versions         func(productID string, body []byte) ([]byte, error)
	deployments      func(body []byte) ([]byte, error)
	deployedProducts func(body []byte) ([]byte, error)
}

func (m *mockCatalogClient) SearchProducts(_ context.Context, b []byte) ([]byte, error) {
	return m.products(b)
}
func (m *mockCatalogClient) SearchProductVersions(_ context.Context, id string, b []byte) ([]byte, error) {
	return m.versions(id, b)
}
func (m *mockCatalogClient) SearchDeployments(_ context.Context, b []byte) ([]byte, error) {
	return m.deployments(b)
}
func (m *mockCatalogClient) SearchDeployedProducts(_ context.Context, b []byte) ([]byte, error) {
	return m.deployedProducts(b)
}

const catalogUUID = "11111111-1111-1111-1111-111111111111"

func serveCatalog(m *mockCatalogClient, target, body string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	h := NewCatalogHandler(m)
	mux.HandleFunc("POST /products/search", h.SearchProducts)
	mux.HandleFunc("POST /products/{id}/versions/search", h.SearchProductVersions)
	mux.HandleFunc("POST /deployments/search", h.SearchDeployments)
	mux.HandleFunc("POST /deployed-products/search", h.SearchDeployedProducts)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, target, strings.NewReader(body)))
	return w
}

// catalogCases lists each search endpoint with the mock wiring that records its upstream body.
func catalogCases(got *[]byte, resp string, err error) []struct {
	name, target, failMsg string
	mock                  *mockCatalogClient
} {
	rec := func(b []byte) ([]byte, error) { *got = b; return []byte(resp), err }
	return []struct {
		name, target, failMsg string
		mock                  *mockCatalogClient
	}{
		{"products", "/products/search", "Failed to search products.", &mockCatalogClient{products: rec}},
		{"versions", "/products/" + catalogUUID + "/versions/search", "Failed to search product versions.",
			&mockCatalogClient{versions: func(_ string, b []byte) ([]byte, error) { return rec(b) }}},
		{"deployments", "/deployments/search", "Failed to search deployments.", &mockCatalogClient{deployments: rec}},
		{"deployed-products", "/deployed-products/search", "Failed to search deployed products.", &mockCatalogClient{deployedProducts: rec}},
	}
}

func TestCatalog_ForwardsBodyAndKeepsResponseContent(t *testing.T) {
	const payload = `{"pagination":{"limit":5,"offset":0},"searchQuery":"x"}`
	const upstream = `{"items":[{"id":"a"}],"total":1,"hasMore":false,"extra":{"kept":true}}`
	var got []byte
	for _, tc := range catalogCases(&got, upstream, nil) {
		t.Run(tc.name, func(t *testing.T) {
			got = nil
			w := serveCatalog(tc.mock, tc.target, payload)
			assertStatus(t, w, http.StatusOK)
			assertContentType(t, w, "application/json")
			if string(got) != payload {
				t.Errorf("upstream body = %q, want %q", got, payload)
			}
			var want, have any
			_ = json.Unmarshal([]byte(upstream), &want)
			if err := json.Unmarshal(w.Body.Bytes(), &have); err != nil {
				t.Fatalf("response is not JSON: %v", err)
			}
			if !reflect.DeepEqual(have, want) {
				t.Errorf("response = %s, want content of %s", w.Body.String(), upstream)
			}
		})
	}
}

func TestCatalog_UppercasesEnumFields(t *testing.T) {
	for _, tc := range []struct {
		name, target, upstream, wantField, want string
		mock                                    func(resp string) *mockCatalogClient
	}{
		{"products", "/products/search", `{"products":[{"id":"a","name":"APIM","class":"software"},{"id":"b","name":"Svc","class":"service"}],"total":2}`, "class", "SOFTWARE",
			func(r string) *mockCatalogClient {
				return &mockCatalogClient{products: func([]byte) ([]byte, error) { return []byte(r), nil }}
			}},
		{"versions", "/products/" + catalogUUID + "/versions/search", `{"productVersions":[{"id":"v","version":"4.0.0","currentSupportStatus":"available"}],"total":1}`, "currentSupportStatus", "AVAILABLE",
			func(r string) *mockCatalogClient {
				return &mockCatalogClient{versions: func(string, []byte) ([]byte, error) { return []byte(r), nil }}
			}},
		{"deployments", "/deployments/search", `{"deployments":[{"id":"d","type":"primary_production"}],"total":1}`, "type", "PRIMARY_PRODUCTION",
			func(r string) *mockCatalogClient {
				return &mockCatalogClient{deployments: func([]byte) ([]byte, error) { return []byte(r), nil }}
			}},
		{"deployed-products", "/deployed-products/search", `{"deployedProducts":[{"id":"p","category":"pdp"}],"total":1}`, "category", "PDP",
			func(r string) *mockCatalogClient {
				return &mockCatalogClient{deployedProducts: func([]byte) ([]byte, error) { return []byte(r), nil }}
			}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := serveCatalog(tc.mock(tc.upstream), tc.target, `{}`)
			assertStatus(t, w, http.StatusOK)
			resp := decodeJSON[map[string]any](t, w)
			var list []any
			for _, v := range resp {
				if l, ok := v.([]any); ok {
					list = l
				}
			}
			if len(list) == 0 || list[0].(map[string]any)[tc.wantField] != tc.want {
				t.Errorf("%s = %v, want %q; body: %s", tc.wantField, list, tc.want, w.Body.String())
			}
		})
	}
}

func TestCatalog_UpperEnumFieldsLeavesTheRestAlone(t *testing.T) {
	in := `{"deployedProducts":[{"id":"x","category":null,"tps":12.50,"cores":4,"note":"a<b&c","updates":[{"updateLevel":12}]},{"id":"y","category":"ms"}],"total":2,"unrelated":{"category":"keep"}}`
	out, err := upperEnumFields([]byte(in), "deployedProducts", "category")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := string(out)
	for _, want := range []string{`"tps":12.50`, `"cores":4`, `"note":"a<b&c"`, `"category":null`, `"category":"MS"`, `"unrelated":{"category":"keep"}`, `"updateLevel":12`} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %s: %s", want, got)
		}
	}
	// a response without the list, and a non-object body, are handled safely
	if out, err := upperEnumFields([]byte(`{"total":0}`), "deployedProducts", "category"); err != nil || string(out) != `{"total":0}` {
		t.Errorf("no list: out=%s err=%v", out, err)
	}
	for name, body := range map[string]string{
		"array":         `[1]`,
		"null":          `null`,
		"trailing data": `{"total":0} {"total":1}`,
		"trailing junk": `{"total":0} x`,
		"empty":         ``,
	} {
		if _, err := upperEnumFields([]byte(body), "deployedProducts", "category"); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if _, err := upperEnumFields([]byte("{\"total\":0}\n  "), "deployedProducts", "category"); err != nil {
		t.Errorf("trailing whitespace must be accepted: %v", err)
	}
}

func TestCatalog_UpperEnumFieldsKeepsMemberOrder(t *testing.T) {
	in := `{"total":2,"hasMore":true,"deployments":[{"type":"qa","id":"x","projectSfId":null},{"id":"y","type":"primary_production"}],"limit":1}`
	want := `{"total":2,"hasMore":true,"deployments":[{"type":"QA","id":"x","projectSfId":null},{"id":"y","type":"PRIMARY_PRODUCTION"}],"limit":1}`
	out, err := upperEnumFields([]byte(in), "deployments", "type")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != want {
		t.Errorf("order or content changed:\n got %s\nwant %s", out, want)
	}
}

func TestCatalog_UpperEnumFieldsNestedDeploymentType(t *testing.T) {
	in := `{"total":4,"deployedProducts":[` +
		`{"id":"a","category":"pdp","deployment":{"id":"d1","name":"QA env","type":"qa"},"tps":1.50},` +
		`{"id":"b","category":null,"deployment":{"id":"d2","name":"n","type":null}},` +
		`{"id":"c","deployment":null},` +
		`{"id":"d","deployment":{"id":"d4","type":"primary_production","name":"p"}}` +
		`],"hasMore":false}`
	want := `{"total":4,"deployedProducts":[` +
		`{"id":"a","category":"PDP","deployment":{"id":"d1","name":"QA env","type":"QA"},"tps":1.50},` +
		`{"id":"b","category":null,"deployment":{"id":"d2","name":"n","type":null}},` +
		`{"id":"c","deployment":null},` +
		`{"id":"d","deployment":{"id":"d4","type":"PRIMARY_PRODUCTION","name":"p"}}` +
		`],"hasMore":false}`
	out, err := upperEnumFields([]byte(in), "deployedProducts", "category", "deployment.type")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != want {
		t.Errorf("nested rewrite wrong:\n got %s\nwant %s", out, want)
	}
	// a non-object "deployment" and a missing one are left alone
	for _, body := range []string{`{"deployedProducts":[{"deployment":"x"}]}`, `{"deployedProducts":[{"id":"z"}]}`} {
		out, err := upperEnumFields([]byte(body), "deployedProducts", "deployment.type")
		if err != nil || string(out) != body {
			t.Errorf("body %s: out=%s err=%v, want unchanged", body, out, err)
		}
	}
}

func TestCatalog_DeployedProductsUppercaseDeploymentType(t *testing.T) {
	m := &mockCatalogClient{deployedProducts: func([]byte) ([]byte, error) {
		return []byte(`{"deployedProducts":[{"id":"p","category":"pdp","deployment":{"id":"d","name":"Prod","type":"primary_production"}}],"total":1}`), nil
	}}
	w := serveCatalog(m, "/deployed-products/search", `{}`)
	assertStatus(t, w, http.StatusOK)
	list := decodeJSON[map[string]any](t, w)["deployedProducts"].([]any)
	dep := list[0].(map[string]any)["deployment"].(map[string]any)
	if dep["type"] != "PRIMARY_PRODUCTION" || dep["id"] != "d" || dep["name"] != "Prod" || list[0].(map[string]any)["category"] != "PDP" {
		t.Errorf("unexpected deployed product: %v", list[0])
	}
}

func TestCatalog_NonJSONUpstreamBodyIs500(t *testing.T) {
	m := &mockCatalogClient{products: func([]byte) ([]byte, error) { return []byte(`not json`), nil }}
	assertStatus(t, serveCatalog(m, "/products/search", `{}`), http.StatusInternalServerError)
}

func TestCatalog_EmptyBodyBecomesEmptyObject(t *testing.T) {
	for _, body := range []string{"", "  \n"} {
		var got []byte
		for _, tc := range catalogCases(&got, `{}`, nil) {
			t.Run(tc.name, func(t *testing.T) {
				got = nil
				assertStatus(t, serveCatalog(tc.mock, tc.target, body), http.StatusOK)
				if string(got) != `{}` {
					t.Errorf("upstream body = %q for input %q, want {}", got, body)
				}
			})
		}
	}
}

func TestCatalog_RejectsBadBodies(t *testing.T) {
	var got []byte
	for _, tc := range catalogCases(&got, `{}`, nil) {
		t.Run(tc.name+"/invalid json", func(t *testing.T) {
			w := serveCatalog(tc.mock, tc.target, "not-json")
			assertStatus(t, w, http.StatusBadRequest)
			assertErrorMessage(t, w, ErrMsgBadRequest)
		})
		t.Run(tc.name+"/too large", func(t *testing.T) {
			w := serveCatalog(tc.mock, tc.target, strings.Repeat("x", maxRequestBodyBytes+1))
			assertStatus(t, w, http.StatusRequestEntityTooLarge)
			assertErrorMessage(t, w, ErrMsgTooLarge)
		})
	}
}

func TestCatalog_VersionsRejectsNonUUIDProductID(t *testing.T) {
	w := serveCatalog(&mockCatalogClient{}, "/products/not-a-uuid/versions/search", `{}`)
	assertStatus(t, w, http.StatusBadRequest)
	assertErrorMessage(t, w, ErrMsgInvalidUUID)
}

func TestCatalog_VersionsForwardsProductID(t *testing.T) {
	var gotID string
	m := &mockCatalogClient{versions: func(id string, _ []byte) ([]byte, error) {
		gotID = id
		return []byte(`{}`), nil
	}}
	assertStatus(t, serveCatalog(m, "/products/"+catalogUUID+"/versions/search", `{}`), http.StatusOK)
	if gotID != catalogUUID {
		t.Errorf("product id = %q, want %q", gotID, catalogUUID)
	}
}

func TestCatalog_UpstreamErrors(t *testing.T) {
	var sink []byte
	for i, base := range catalogCases(&sink, ``, nil) {
		for _, tc := range upstreamErrors(base.failMsg) {
			t.Run(base.name+"/"+tc.name, func(t *testing.T) {
				c := catalogCases(&sink, ``, tc.err)[i]
				w := serveCatalog(c.mock, c.target, `{}`)
				assertStatus(t, w, tc.wantCode)
				assertErrorMessage(t, w, tc.wantMsg)
			})
		}
	}
}
