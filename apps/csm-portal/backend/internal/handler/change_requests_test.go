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
	"strings"
	"testing"

	"github.com/wso2-open-operations/cs-tools/apps/csm-portal/backend/internal/apierror"
)

const testCRID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

func TestCreateChangeRequest(t *testing.T) {
	t.Run("requires authenticated user", func(t *testing.T) {
		h := NewChangeRequestHandler(&mockEntityChangeRequestClient{})
		r := httptest.NewRequest(http.MethodPost, "/change-requests", strings.NewReader(`{"subject":"Deploy v2","type":"normal"}`))
		w := httptest.NewRecorder()
		h.CreateChangeRequest(w, r)
		assertStatus(t, w, http.StatusUnauthorized)
		assertErrorMessage(t, w, ErrMsgUnauthorized)
		assertContentType(t, w, "application/json")
	})

	t.Run("rejects body exceeding 1 MiB", func(t *testing.T) {
		h := NewChangeRequestHandler(&mockEntityChangeRequestClient{})
		r := withUser(httptest.NewRequest(http.MethodPost, "/change-requests", strings.NewReader(strings.Repeat("x", maxRequestBodyBytes+1))))
		w := httptest.NewRecorder()
		h.CreateChangeRequest(w, r)
		assertStatus(t, w, http.StatusRequestEntityTooLarge)
		assertErrorMessage(t, w, ErrMsgTooLarge)
		assertContentType(t, w, "application/json")
	})

	t.Run("requires a type of standard, normal or emergency", func(t *testing.T) {
		for name, payload := range map[string]string{
			"missing":      `{"subject":"Deploy v2"}`,
			"empty":        `{"subject":"Deploy v2","type":""}`,
			"not a string": `{"subject":"Deploy v2","type":3}`,
			"azure":        `{"subject":"Deploy v2","type":"azure"}`,
			"bogus":        `{"subject":"Deploy v2","type":"bogus"}`,
		} {
			t.Run(name, func(t *testing.T) {
				called := false
				client := &mockEntityChangeRequestClient{
					createChangeRequestFn: func(_ context.Context, _ []byte) ([]byte, error) {
						called = true
						return nil, nil
					},
				}
				h := NewChangeRequestHandler(client)
				r := withUser(httptest.NewRequest(http.MethodPost, "/change-requests", strings.NewReader(payload)))
				w := httptest.NewRecorder()
				h.CreateChangeRequest(w, r)
				assertStatus(t, w, http.StatusBadRequest)
				if called {
					t.Error("upstream was called for a change request without a valid type")
				}
				if msg := w.Body.String(); !strings.Contains(msg, "standard, normal or emergency") {
					t.Errorf("message %q should name the allowed types", msg)
				}
			})
		}
	})

	t.Run("accepts each creatable type", func(t *testing.T) {
		for _, typ := range []string{"standard", "normal", "emergency"} {
			client := &mockEntityChangeRequestClient{
				createChangeRequestFn: func(_ context.Context, _ []byte) ([]byte, error) {
					return []byte(`{"message":"ok"}`), nil
				},
			}
			h := NewChangeRequestHandler(client)
			r := withUser(httptest.NewRequest(http.MethodPost, "/change-requests", strings.NewReader(`{"subject":"x","type":"`+typ+`"}`)))
			w := httptest.NewRecorder()
			h.CreateChangeRequest(w, r)
			assertStatus(t, w, http.StatusCreated)
		}
	})

	t.Run("rejects invalid JSON body", func(t *testing.T) {
		h := NewChangeRequestHandler(&mockEntityChangeRequestClient{})
		r := withUser(httptest.NewRequest(http.MethodPost, "/change-requests", strings.NewReader(`not-json`)))
		w := httptest.NewRecorder()
		h.CreateChangeRequest(w, r)
		assertStatus(t, w, http.StatusBadRequest)
		assertErrorMessage(t, w, ErrMsgBadRequest)
		assertContentType(t, w, "application/json")
	})

	t.Run("forwards body to upstream and returns 201 with response", func(t *testing.T) {
		const reqPayload = `{"subject":"Deploy v2","type":"normal"}`
		var capturedBody []byte
		client := &mockEntityChangeRequestClient{
			createChangeRequestFn: func(_ context.Context, body []byte) ([]byte, error) {
				capturedBody = body
				return []byte(`{"message":"Change request created.","changeRequest":{"id":"` + testCRID + `","number":"CHG0001","createdOn":"2026-01-01T00:00:00Z","createdBy":"user@example.com"}}`), nil
			},
		}
		h := NewChangeRequestHandler(client)
		r := withUser(httptest.NewRequest(http.MethodPost, "/change-requests", strings.NewReader(reqPayload)))
		w := httptest.NewRecorder()
		h.CreateChangeRequest(w, r)

		assertStatus(t, w, http.StatusCreated)
		assertContentType(t, w, "application/json")
		if string(capturedBody) != reqPayload {
			t.Errorf("upstream received body %q, want %q", capturedBody, reqPayload)
		}
	})

	t.Run("upstream errors are mapped correctly", func(t *testing.T) {
		for _, tc := range upstreamErrorsGeneric("Failed to create change request.") {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				client := &mockEntityChangeRequestClient{
					createChangeRequestFn: func(_ context.Context, _ []byte) ([]byte, error) {
						return nil, tc.err
					},
				}
				h := NewChangeRequestHandler(client)
				r := withUser(httptest.NewRequest(http.MethodPost, "/change-requests", strings.NewReader(`{"subject":"Deploy v2","type":"normal"}`)))
				w := httptest.NewRecorder()
				h.CreateChangeRequest(w, r)
				assertStatus(t, w, tc.wantCode)
				assertErrorMessage(t, w, tc.wantMsg)
				assertContentType(t, w, "application/json")
			})
		}
	})
}

func TestGetChangeRequest(t *testing.T) {
	t.Run("requires authenticated user", func(t *testing.T) {
		h := NewChangeRequestHandler(&mockEntityChangeRequestClient{})
		r := httptest.NewRequest(http.MethodGet, "/change-requests/"+testCRID, nil)
		r.SetPathValue("id", testCRID)
		w := httptest.NewRecorder()
		h.GetChangeRequest(w, r)
		assertStatus(t, w, http.StatusUnauthorized)
		assertErrorMessage(t, w, ErrMsgUnauthorized)
		assertContentType(t, w, "application/json")
	})

	t.Run("rejects malformed UUID", func(t *testing.T) {
		h := NewChangeRequestHandler(&mockEntityChangeRequestClient{})
		r := withUser(httptest.NewRequest(http.MethodGet, "/change-requests/not-a-uuid", nil))
		r.SetPathValue("id", "not-a-uuid")
		w := httptest.NewRecorder()
		h.GetChangeRequest(w, r)
		assertStatus(t, w, http.StatusBadRequest)
		assertErrorMessage(t, w, ErrMsgInvalidUUID)
		assertContentType(t, w, "application/json")
	})

	t.Run("rejects empty id", func(t *testing.T) {
		h := NewChangeRequestHandler(&mockEntityChangeRequestClient{})
		r := withUser(httptest.NewRequest(http.MethodGet, "/change-requests/", nil))
		w := httptest.NewRecorder()
		h.GetChangeRequest(w, r)
		assertStatus(t, w, http.StatusBadRequest)
		assertErrorMessage(t, w, ErrMsgInvalidUUID)
		assertContentType(t, w, "application/json")
	})

	t.Run("forwards id to upstream and returns 200", func(t *testing.T) {
		var capturedID string
		client := &mockEntityChangeRequestClient{
			getChangeRequestFn: func(_ context.Context, id string) ([]byte, error) {
				capturedID = id
				return []byte(`{"id":"` + testCRID + `","number":"CHG001","state":"scheduled","legalNextStates":["implement","canceled"]}`), nil
			},
		}
		h := NewChangeRequestHandler(client)
		r := withUser(httptest.NewRequest(http.MethodGet, "/change-requests/"+testCRID, nil))
		r.SetPathValue("id", testCRID)
		w := httptest.NewRecorder()
		h.GetChangeRequest(w, r)

		assertStatus(t, w, http.StatusOK)
		assertContentType(t, w, "application/json")

		if capturedID != testCRID {
			t.Errorf("upstream received id %q, want %q", capturedID, testCRID)
		}
		resp := decodeJSON[map[string]any](t, w)
		if resp["number"] != "CHG001" {
			t.Errorf("response number = %v, want CHG001", resp["number"])
		}
		legalNextStates, ok := resp["legalNextStates"].([]any)
		if !ok || len(legalNextStates) != 2 {
			t.Fatalf("legalNextStates = %v, want 2 entries", resp["legalNextStates"])
		}
	})

	t.Run("upstream errors are mapped correctly", func(t *testing.T) {
		for _, tc := range upstreamErrorsGeneric("Failed to retrieve change request.") {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				client := &mockEntityChangeRequestClient{
					getChangeRequestFn: func(_ context.Context, _ string) ([]byte, error) {
						return nil, tc.err
					},
				}
				h := NewChangeRequestHandler(client)
				r := withUser(httptest.NewRequest(http.MethodGet, "/change-requests/"+testCRID, nil))
				r.SetPathValue("id", testCRID)
				w := httptest.NewRecorder()
				h.GetChangeRequest(w, r)
				assertStatus(t, w, tc.wantCode)
				assertErrorMessage(t, w, tc.wantMsg)
				assertContentType(t, w, "application/json")
			})
		}
	})
}

func TestPatchChangeRequest(t *testing.T) {
	t.Run("requires authenticated user", func(t *testing.T) {
		h := NewChangeRequestHandler(&mockEntityChangeRequestClient{})
		r := httptest.NewRequest(http.MethodPatch, "/change-requests/"+testCRID, strings.NewReader(`{"requestApproval":true}`))
		r.SetPathValue("id", testCRID)
		w := httptest.NewRecorder()
		h.PatchChangeRequest(w, r)
		assertStatus(t, w, http.StatusUnauthorized)
		assertErrorMessage(t, w, ErrMsgUnauthorized)
		assertContentType(t, w, "application/json")
	})

	t.Run("rejects malformed UUID", func(t *testing.T) {
		h := NewChangeRequestHandler(&mockEntityChangeRequestClient{})
		r := withUser(httptest.NewRequest(http.MethodPatch, "/change-requests/not-a-uuid", strings.NewReader(`{"requestApproval":true}`)))
		r.SetPathValue("id", "not-a-uuid")
		w := httptest.NewRecorder()
		h.PatchChangeRequest(w, r)
		assertStatus(t, w, http.StatusBadRequest)
		assertErrorMessage(t, w, ErrMsgInvalidUUID)
		assertContentType(t, w, "application/json")
	})

	t.Run("rejects empty id", func(t *testing.T) {
		h := NewChangeRequestHandler(&mockEntityChangeRequestClient{})
		r := withUser(httptest.NewRequest(http.MethodPatch, "/change-requests/", strings.NewReader(`{"requestApproval":true}`)))
		w := httptest.NewRecorder()
		h.PatchChangeRequest(w, r)
		assertStatus(t, w, http.StatusBadRequest)
		assertErrorMessage(t, w, ErrMsgInvalidUUID)
		assertContentType(t, w, "application/json")
	})

	t.Run("rejects body exceeding 1 MiB", func(t *testing.T) {
		h := NewChangeRequestHandler(&mockEntityChangeRequestClient{})
		r := withUser(httptest.NewRequest(http.MethodPatch, "/change-requests/"+testCRID, strings.NewReader(strings.Repeat("x", maxRequestBodyBytes+1))))
		r.SetPathValue("id", testCRID)
		w := httptest.NewRecorder()
		h.PatchChangeRequest(w, r)
		assertStatus(t, w, http.StatusRequestEntityTooLarge)
		assertErrorMessage(t, w, ErrMsgTooLarge)
		assertContentType(t, w, "application/json")
	})

	t.Run("rejects invalid JSON body", func(t *testing.T) {
		h := NewChangeRequestHandler(&mockEntityChangeRequestClient{})
		r := withUser(httptest.NewRequest(http.MethodPatch, "/change-requests/"+testCRID, strings.NewReader(`not-json`)))
		r.SetPathValue("id", testCRID)
		w := httptest.NewRecorder()
		h.PatchChangeRequest(w, r)
		assertStatus(t, w, http.StatusBadRequest)
		assertErrorMessage(t, w, ErrMsgBadRequest)
		assertContentType(t, w, "application/json")
	})

	t.Run("forwards requestApproval to upstream and returns 200 with legalNextStates", func(t *testing.T) {
		const reqPayload = `{"requestApproval":true}`
		var capturedID string
		var capturedBody []byte
		client := &mockEntityChangeRequestClient{
			patchChangeRequestFn: func(_ context.Context, id string, body []byte) ([]byte, error) {
				capturedID = id
				capturedBody = body
				return []byte(`{"id":"` + testCRID + `","number":"CHG0001","state":"assess","legalNextStates":["authorize","canceled"]}`), nil
			},
		}
		h := NewChangeRequestHandler(client)
		r := withUser(httptest.NewRequest(http.MethodPatch, "/change-requests/"+testCRID, strings.NewReader(reqPayload)))
		r.SetPathValue("id", testCRID)
		w := httptest.NewRecorder()
		h.PatchChangeRequest(w, r)

		assertStatus(t, w, http.StatusOK)
		assertContentType(t, w, "application/json")

		if capturedID != testCRID {
			t.Errorf("upstream received id %q, want %q", capturedID, testCRID)
		}
		if string(capturedBody) != reqPayload {
			t.Errorf("upstream received body %q, want %q", capturedBody, reqPayload)
		}

		resp := decodeJSON[map[string]any](t, w)
		legalNextStates, ok := resp["legalNextStates"].([]any)
		if !ok || len(legalNextStates) != 2 {
			t.Fatalf("legalNextStates = %v, want 2 entries", resp["legalNextStates"])
		}
	})

	t.Run("upstream errors are mapped correctly", func(t *testing.T) {
		for _, tc := range upstreamErrors("Failed to update change request.") {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				client := &mockEntityChangeRequestClient{
					patchChangeRequestFn: func(_ context.Context, _ string, _ []byte) ([]byte, error) {
						return nil, tc.err
					},
				}
				h := NewChangeRequestHandler(client)
				r := withUser(httptest.NewRequest(http.MethodPatch, "/change-requests/"+testCRID, strings.NewReader(`{"requestApproval":true}`)))
				r.SetPathValue("id", testCRID)
				w := httptest.NewRecorder()
				h.PatchChangeRequest(w, r)
				assertStatus(t, w, tc.wantCode)
				assertErrorMessage(t, w, tc.wantMsg)
				assertContentType(t, w, "application/json")
			})
		}
	})
}

func TestGetChangeRequestApprovals(t *testing.T) {
	t.Run("requires authenticated user", func(t *testing.T) {
		h := NewChangeRequestHandler(&mockEntityChangeRequestClient{})
		r := httptest.NewRequest(http.MethodGet, "/change-requests/"+testCRID+"/approvals", nil)
		r.SetPathValue("id", testCRID)
		w := httptest.NewRecorder()
		h.GetChangeRequestApprovals(w, r)
		assertStatus(t, w, http.StatusUnauthorized)
		assertErrorMessage(t, w, ErrMsgUnauthorized)
		assertContentType(t, w, "application/json")
	})

	t.Run("rejects malformed UUID", func(t *testing.T) {
		h := NewChangeRequestHandler(&mockEntityChangeRequestClient{})
		r := withUser(httptest.NewRequest(http.MethodGet, "/change-requests/not-a-uuid/approvals", nil))
		r.SetPathValue("id", "not-a-uuid")
		w := httptest.NewRecorder()
		h.GetChangeRequestApprovals(w, r)
		assertStatus(t, w, http.StatusBadRequest)
		assertErrorMessage(t, w, ErrMsgInvalidUUID)
		assertContentType(t, w, "application/json")
	})

	t.Run("rejects empty id", func(t *testing.T) {
		h := NewChangeRequestHandler(&mockEntityChangeRequestClient{})
		r := withUser(httptest.NewRequest(http.MethodGet, "/change-requests//approvals", nil))
		w := httptest.NewRecorder()
		h.GetChangeRequestApprovals(w, r)
		assertStatus(t, w, http.StatusBadRequest)
		assertErrorMessage(t, w, ErrMsgInvalidUUID)
		assertContentType(t, w, "application/json")
	})

	t.Run("forwards id to upstream and returns 200", func(t *testing.T) {
		var capturedID string
		client := &mockEntityChangeRequestClient{
			getChangeRequestApprovalsFn: func(_ context.Context, id string) ([]byte, error) {
				capturedID = id
				return []byte(`{"approvals":[{"stage":"Assess","approverType":"STATIC_GROUP","approverName":"Devops Approval","status":"APPROVED","approvers":[{"id":"11111111-1111-1111-1111-111111111111","name":"Thenuka Keerthibandara","status":"APPROVED","respondedOn":"2026-07-08 06:02:56"}]}]}`), nil
			},
		}
		h := NewChangeRequestHandler(client)
		r := withUser(httptest.NewRequest(http.MethodGet, "/change-requests/"+testCRID+"/approvals", nil))
		r.SetPathValue("id", testCRID)
		w := httptest.NewRecorder()
		h.GetChangeRequestApprovals(w, r)

		assertStatus(t, w, http.StatusOK)
		assertContentType(t, w, "application/json")

		if capturedID != testCRID {
			t.Errorf("upstream received id %q, want %q", capturedID, testCRID)
		}
		resp := decodeJSON[map[string]any](t, w)
		approvals, ok := resp["approvals"].([]any)
		if !ok || len(approvals) != 1 {
			t.Fatalf("approvals = %v, want 1 entry", resp["approvals"])
		}
	})

	t.Run("upstream errors are mapped correctly", func(t *testing.T) {
		for _, tc := range upstreamErrorsGeneric("Failed to retrieve change request approvals.") {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				client := &mockEntityChangeRequestClient{
					getChangeRequestApprovalsFn: func(_ context.Context, _ string) ([]byte, error) {
						return nil, tc.err
					},
				}
				h := NewChangeRequestHandler(client)
				r := withUser(httptest.NewRequest(http.MethodGet, "/change-requests/"+testCRID+"/approvals", nil))
				r.SetPathValue("id", testCRID)
				w := httptest.NewRecorder()
				h.GetChangeRequestApprovals(w, r)
				assertStatus(t, w, tc.wantCode)
				assertErrorMessage(t, w, tc.wantMsg)
				assertContentType(t, w, "application/json")
			})
		}
	})
}

func TestDecideChangeRequestApproval(t *testing.T) {
	t.Run("requires authenticated user", func(t *testing.T) {
		h := NewChangeRequestHandler(&mockEntityChangeRequestClient{})
		r := httptest.NewRequest(http.MethodPost, "/change-requests/"+testCRID+"/approvals/decision", strings.NewReader(`{"decision":"approved"}`))
		r.SetPathValue("id", testCRID)
		w := httptest.NewRecorder()
		h.DecideChangeRequestApproval(w, r)
		assertStatus(t, w, http.StatusUnauthorized)
		assertErrorMessage(t, w, ErrMsgUnauthorized)
		assertContentType(t, w, "application/json")
	})

	t.Run("rejects malformed UUID", func(t *testing.T) {
		h := NewChangeRequestHandler(&mockEntityChangeRequestClient{})
		r := withUser(httptest.NewRequest(http.MethodPost, "/change-requests/not-a-uuid/approvals/decision", strings.NewReader(`{"decision":"approved"}`)))
		r.SetPathValue("id", "not-a-uuid")
		w := httptest.NewRecorder()
		h.DecideChangeRequestApproval(w, r)
		assertStatus(t, w, http.StatusBadRequest)
		assertErrorMessage(t, w, ErrMsgInvalidUUID)
		assertContentType(t, w, "application/json")
	})

	t.Run("rejects empty id", func(t *testing.T) {
		h := NewChangeRequestHandler(&mockEntityChangeRequestClient{})
		r := withUser(httptest.NewRequest(http.MethodPost, "/change-requests//approvals/decision", strings.NewReader(`{"decision":"approved"}`)))
		w := httptest.NewRecorder()
		h.DecideChangeRequestApproval(w, r)
		assertStatus(t, w, http.StatusBadRequest)
		assertErrorMessage(t, w, ErrMsgInvalidUUID)
		assertContentType(t, w, "application/json")
	})

	t.Run("rejects body exceeding 1 MiB", func(t *testing.T) {
		h := NewChangeRequestHandler(&mockEntityChangeRequestClient{})
		r := withUser(httptest.NewRequest(http.MethodPost, "/change-requests/"+testCRID+"/approvals/decision", strings.NewReader(strings.Repeat("x", maxRequestBodyBytes+1))))
		r.SetPathValue("id", testCRID)
		w := httptest.NewRecorder()
		h.DecideChangeRequestApproval(w, r)
		assertStatus(t, w, http.StatusRequestEntityTooLarge)
		assertErrorMessage(t, w, ErrMsgTooLarge)
		assertContentType(t, w, "application/json")
	})

	t.Run("rejects invalid JSON body", func(t *testing.T) {
		h := NewChangeRequestHandler(&mockEntityChangeRequestClient{})
		r := withUser(httptest.NewRequest(http.MethodPost, "/change-requests/"+testCRID+"/approvals/decision", strings.NewReader(`not-json`)))
		r.SetPathValue("id", testCRID)
		w := httptest.NewRecorder()
		h.DecideChangeRequestApproval(w, r)
		assertStatus(t, w, http.StatusBadRequest)
		assertErrorMessage(t, w, ErrMsgBadRequest)
		assertContentType(t, w, "application/json")
	})

	t.Run("rejects unknown fields", func(t *testing.T) {
		h := NewChangeRequestHandler(&mockEntityChangeRequestClient{})
		r := withUser(httptest.NewRequest(http.MethodPost, "/change-requests/"+testCRID+"/approvals/decision", strings.NewReader(`{"decision":"approved","comment":"lgtm"}`)))
		r.SetPathValue("id", testCRID)
		w := httptest.NewRecorder()
		h.DecideChangeRequestApproval(w, r)
		assertStatus(t, w, http.StatusBadRequest)
		assertErrorMessage(t, w, ErrMsgBadRequest)
		assertContentType(t, w, "application/json")
	})

	t.Run("rejects trailing data after the JSON value", func(t *testing.T) {
		h := NewChangeRequestHandler(&mockEntityChangeRequestClient{})
		r := withUser(httptest.NewRequest(http.MethodPost, "/change-requests/"+testCRID+"/approvals/decision", strings.NewReader(`{"decision":"approved"}{"decision":"rejected"}`)))
		r.SetPathValue("id", testCRID)
		w := httptest.NewRecorder()
		h.DecideChangeRequestApproval(w, r)
		assertStatus(t, w, http.StatusBadRequest)
		assertErrorMessage(t, w, ErrMsgBadRequest)
		assertContentType(t, w, "application/json")
	})

	t.Run("rejects a decision value outside approved/rejected", func(t *testing.T) {
		h := NewChangeRequestHandler(&mockEntityChangeRequestClient{})
		r := withUser(httptest.NewRequest(http.MethodPost, "/change-requests/"+testCRID+"/approvals/decision", strings.NewReader(`{"decision":"maybe"}`)))
		r.SetPathValue("id", testCRID)
		w := httptest.NewRecorder()
		h.DecideChangeRequestApproval(w, r)
		assertStatus(t, w, http.StatusBadRequest)
		assertErrorMessage(t, w, ErrMsgBadRequest)
		assertContentType(t, w, "application/json")
	})

	t.Run("rejects an empty decision value", func(t *testing.T) {
		h := NewChangeRequestHandler(&mockEntityChangeRequestClient{})
		r := withUser(httptest.NewRequest(http.MethodPost, "/change-requests/"+testCRID+"/approvals/decision", strings.NewReader(`{}`)))
		r.SetPathValue("id", testCRID)
		w := httptest.NewRecorder()
		h.DecideChangeRequestApproval(w, r)
		assertStatus(t, w, http.StatusBadRequest)
		assertErrorMessage(t, w, ErrMsgBadRequest)
		assertContentType(t, w, "application/json")
	})

	t.Run("forwards body to upstream and returns 200 with response", func(t *testing.T) {
		const reqPayload = `{"decision":"approved"}`
		var capturedID string
		var capturedBody []byte
		client := &mockEntityChangeRequestClient{
			decideChangeRequestApprovalFn: func(_ context.Context, id string, body []byte) ([]byte, error) {
				capturedID = id
				capturedBody = body
				return []byte(`{"id":"11111111-1111-1111-1111-111111111111","state":"approved"}`), nil
			},
		}
		h := NewChangeRequestHandler(client)
		r := withUser(httptest.NewRequest(http.MethodPost, "/change-requests/"+testCRID+"/approvals/decision", strings.NewReader(reqPayload)))
		r.SetPathValue("id", testCRID)
		w := httptest.NewRecorder()
		h.DecideChangeRequestApproval(w, r)

		assertStatus(t, w, http.StatusOK)
		assertContentType(t, w, "application/json")

		if capturedID != testCRID {
			t.Errorf("upstream received id %q, want %q", capturedID, testCRID)
		}
		if string(capturedBody) != reqPayload {
			t.Errorf("upstream received body %q, want %q", capturedBody, reqPayload)
		}
		resp := decodeJSON[map[string]any](t, w)
		if resp["state"] != "approved" {
			t.Errorf("state = %v, want approved", resp["state"])
		}
	})

	t.Run("upstream errors are mapped correctly", func(t *testing.T) {
		for _, tc := range upstreamErrorsGeneric("Failed to submit change request approval decision.") {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				client := &mockEntityChangeRequestClient{
					decideChangeRequestApprovalFn: func(_ context.Context, _ string, _ []byte) ([]byte, error) {
						return nil, tc.err
					},
				}
				h := NewChangeRequestHandler(client)
				r := withUser(httptest.NewRequest(http.MethodPost, "/change-requests/"+testCRID+"/approvals/decision", strings.NewReader(`{"decision":"approved"}`)))
				r.SetPathValue("id", testCRID)
				w := httptest.NewRecorder()
				h.DecideChangeRequestApproval(w, r)
				assertStatus(t, w, tc.wantCode)
				assertErrorMessage(t, w, tc.wantMsg)
				assertContentType(t, w, "application/json")
			})
		}
	})

	// A refusal to decide must tell the approver why (creator may not approve,
	// SRE members may not give peer approval); the reason comes from the entity
	// service's own error envelope. Anything else stays generic.
	t.Run("a 403 carrying the entity service's reason shows it", func(t *testing.T) {
		client := &mockEntityChangeRequestClient{
			decideChangeRequestApprovalFn: func(_ context.Context, _ string, _ []byte) ([]byte, error) {
				return nil, &apierror.Error{StatusCode: http.StatusForbidden, Body: `{"code":403,"message":"the creator of a change request cannot approve it"}`}
			},
		}
		h := NewChangeRequestHandler(client)
		r := withUser(httptest.NewRequest(http.MethodPost, "/change-requests/"+testCRID+"/approvals/decision", strings.NewReader(`{"decision":"approved"}`)))
		r.SetPathValue("id", testCRID)
		w := httptest.NewRecorder()
		h.DecideChangeRequestApproval(w, r)
		assertStatus(t, w, http.StatusForbidden)
		assertErrorMessage(t, w, "the creator of a change request cannot approve it")
	})
}

func TestSearchChangeRequests(t *testing.T) {
	t.Run("requires authenticated user", func(t *testing.T) {
		h := NewChangeRequestHandler(&mockEntityChangeRequestClient{})
		r := httptest.NewRequest(http.MethodPost, "/change-requests/search", strings.NewReader(`{}`))
		w := httptest.NewRecorder()
		h.SearchChangeRequests(w, r)
		assertStatus(t, w, http.StatusUnauthorized)
		assertErrorMessage(t, w, ErrMsgUnauthorized)
		assertContentType(t, w, "application/json")
	})

	t.Run("rejects body exceeding 1 MiB", func(t *testing.T) {
		h := NewChangeRequestHandler(&mockEntityChangeRequestClient{})
		r := withUser(httptest.NewRequest(http.MethodPost, "/change-requests/search", strings.NewReader(strings.Repeat("x", maxRequestBodyBytes+1))))
		w := httptest.NewRecorder()
		h.SearchChangeRequests(w, r)
		assertStatus(t, w, http.StatusRequestEntityTooLarge)
		assertErrorMessage(t, w, ErrMsgTooLarge)
		assertContentType(t, w, "application/json")
	})

	t.Run("rejects invalid JSON body", func(t *testing.T) {
		h := NewChangeRequestHandler(&mockEntityChangeRequestClient{})
		r := withUser(httptest.NewRequest(http.MethodPost, "/change-requests/search", strings.NewReader(`not-json`)))
		w := httptest.NewRecorder()
		h.SearchChangeRequests(w, r)
		assertStatus(t, w, http.StatusBadRequest)
		assertErrorMessage(t, w, ErrMsgBadRequest)
		assertContentType(t, w, "application/json")
	})

	t.Run("forwards body to upstream and returns 200", func(t *testing.T) {
		var capturedBody []byte
		client := &mockEntityChangeRequestClient{
			searchChangeRequestsFn: func(_ context.Context, body []byte) ([]byte, error) {
				capturedBody = body
				return []byte(`{"changeRequests":[{"id":"cr-1"}],"total":1,"limit":20,"offset":0}`), nil
			},
		}
		h := NewChangeRequestHandler(client)
		const payload = `{"filters":{"states":["scheduled"]},"pagination":{"limit":20,"offset":0}}`
		r := withUser(httptest.NewRequest(http.MethodPost, "/change-requests/search", strings.NewReader(payload)))
		w := httptest.NewRecorder()
		h.SearchChangeRequests(w, r)

		assertStatus(t, w, http.StatusOK)
		assertContentType(t, w, "application/json")

		if string(capturedBody) != payload {
			t.Errorf("upstream received body %q, want %q", capturedBody, payload)
		}
		resp := decodeJSON[map[string]any](t, w)
		if resp["total"] != float64(1) {
			t.Errorf("total = %v, want 1", resp["total"])
		}
	})

	t.Run("upstream errors are mapped correctly", func(t *testing.T) {
		for _, tc := range upstreamErrorsGeneric("Failed to search change requests.") {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				client := &mockEntityChangeRequestClient{
					searchChangeRequestsFn: func(_ context.Context, _ []byte) ([]byte, error) {
						return nil, tc.err
					},
				}
				h := NewChangeRequestHandler(client)
				r := withUser(httptest.NewRequest(http.MethodPost, "/change-requests/search", strings.NewReader(`{}`)))
				w := httptest.NewRecorder()
				h.SearchChangeRequests(w, r)
				assertStatus(t, w, tc.wantCode)
				assertErrorMessage(t, w, tc.wantMsg)
				assertContentType(t, w, "application/json")
			})
		}
	})
}

func TestCreateChangeRequestComment(t *testing.T) {
	t.Run("requires authenticated user", func(t *testing.T) {
		h := NewChangeRequestHandler(&mockEntityChangeRequestClient{})
		r := httptest.NewRequest(http.MethodPost, "/change-requests/"+testCRID+"/comments", strings.NewReader(`{"type":"comment","content":"hi"}`))
		r.SetPathValue("id", testCRID)
		w := httptest.NewRecorder()
		h.CreateChangeRequestComment(w, r)
		assertStatus(t, w, http.StatusUnauthorized)
		assertErrorMessage(t, w, ErrMsgUnauthorized)
	})

	t.Run("rejects malformed UUID", func(t *testing.T) {
		h := NewChangeRequestHandler(&mockEntityChangeRequestClient{})
		r := withUser(httptest.NewRequest(http.MethodPost, "/change-requests/not-a-uuid/comments", strings.NewReader(`{"type":"comment","content":"hi"}`)))
		r.SetPathValue("id", "not-a-uuid")
		w := httptest.NewRecorder()
		h.CreateChangeRequestComment(w, r)
		assertStatus(t, w, http.StatusBadRequest)
		assertErrorMessage(t, w, ErrMsgInvalidUUID)
	})

	t.Run("rejects invalid JSON body", func(t *testing.T) {
		h := NewChangeRequestHandler(&mockEntityChangeRequestClient{})
		r := withUser(httptest.NewRequest(http.MethodPost, "/change-requests/"+testCRID+"/comments", strings.NewReader(`not-json`)))
		r.SetPathValue("id", testCRID)
		w := httptest.NewRecorder()
		h.CreateChangeRequestComment(w, r)
		assertStatus(t, w, http.StatusBadRequest)
		assertErrorMessage(t, w, ErrMsgBadRequest)
	})

	t.Run("injects referenceId and referenceType and forwards to the generic comment endpoint", func(t *testing.T) {
		var capturedBody []byte
		client := &mockEntityChangeRequestClient{
			getChangeRequestFn: func(_ context.Context, _ string) ([]byte, error) {
				return []byte(`{"id":"` + testCRID + `"}`), nil
			},
			createCommentFn: func(_ context.Context, body []byte) ([]byte, error) {
				capturedBody = body
				return []byte(`{"message":"Comment created.","comment":{"id":"11111111-1111-1111-1111-111111111111","createdOn":"2026-01-01T00:00:00Z","createdBy":"user@example.com"}}`), nil
			},
		}
		h := NewChangeRequestHandler(client)
		r := withUser(httptest.NewRequest(http.MethodPost, "/change-requests/"+testCRID+"/comments", strings.NewReader(`{"type":"comment","content":"hi"}`)))
		r.SetPathValue("id", testCRID)
		w := httptest.NewRecorder()
		h.CreateChangeRequestComment(w, r)

		assertStatus(t, w, http.StatusCreated)
		if !strings.Contains(string(capturedBody), `"referenceId":"`+testCRID+`"`) {
			t.Errorf("expected referenceId to be injected, got %q", capturedBody)
		}
		if !strings.Contains(string(capturedBody), `"referenceType":"change_request"`) {
			t.Errorf("expected referenceType change_request to be injected, got %q", capturedBody)
		}
	})

	t.Run("upstream GetChangeRequest error is mapped correctly", func(t *testing.T) {
		for _, tc := range upstreamErrorsGeneric("Failed to create change request comment.") {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				client := &mockEntityChangeRequestClient{
					getChangeRequestFn: func(_ context.Context, _ string) ([]byte, error) {
						return nil, tc.err
					},
				}
				h := NewChangeRequestHandler(client)
				r := withUser(httptest.NewRequest(http.MethodPost, "/change-requests/"+testCRID+"/comments", strings.NewReader(`{"type":"comment","content":"hi"}`)))
				r.SetPathValue("id", testCRID)
				w := httptest.NewRecorder()
				h.CreateChangeRequestComment(w, r)
				assertStatus(t, w, tc.wantCode)
				assertErrorMessage(t, w, tc.wantMsg)
			})
		}
	})

	t.Run("upstream CreateComment error is mapped correctly", func(t *testing.T) {
		for _, tc := range upstreamErrorsGeneric("Failed to create change request comment.") {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				client := &mockEntityChangeRequestClient{
					getChangeRequestFn: func(_ context.Context, _ string) ([]byte, error) {
						return []byte(`{"id":"` + testCRID + `"}`), nil
					},
					createCommentFn: func(_ context.Context, _ []byte) ([]byte, error) {
						return nil, tc.err
					},
				}
				h := NewChangeRequestHandler(client)
				r := withUser(httptest.NewRequest(http.MethodPost, "/change-requests/"+testCRID+"/comments", strings.NewReader(`{"type":"comment","content":"hi"}`)))
				r.SetPathValue("id", testCRID)
				w := httptest.NewRecorder()
				h.CreateChangeRequestComment(w, r)
				assertStatus(t, w, tc.wantCode)
				assertErrorMessage(t, w, tc.wantMsg)
			})
		}
	})
}

func TestSearchChangeRequestComments(t *testing.T) {
	t.Run("requires authenticated user", func(t *testing.T) {
		h := NewChangeRequestHandler(&mockEntityChangeRequestClient{})
		r := httptest.NewRequest(http.MethodPost, "/change-requests/"+testCRID+"/comments/search", strings.NewReader(`{}`))
		r.SetPathValue("id", testCRID)
		w := httptest.NewRecorder()
		h.SearchChangeRequestComments(w, r)
		assertStatus(t, w, http.StatusUnauthorized)
		assertErrorMessage(t, w, ErrMsgUnauthorized)
	})

	t.Run("injects referenceId and referenceType and forwards to the generic search endpoint", func(t *testing.T) {
		var capturedBody []byte
		client := &mockEntityChangeRequestClient{
			searchCommentsFn: func(_ context.Context, body []byte) ([]byte, error) {
				capturedBody = body
				return []byte(`{"comments":[],"total":0,"limit":20,"offset":0}`), nil
			},
		}
		h := NewChangeRequestHandler(client)
		r := withUser(httptest.NewRequest(http.MethodPost, "/change-requests/"+testCRID+"/comments/search", strings.NewReader(`{"pagination":{"offset":0,"limit":20}}`)))
		r.SetPathValue("id", testCRID)
		w := httptest.NewRecorder()
		h.SearchChangeRequestComments(w, r)

		assertStatus(t, w, http.StatusOK)
		if !strings.Contains(string(capturedBody), `"referenceId":"`+testCRID+`"`) {
			t.Errorf("expected referenceId to be injected, got %q", capturedBody)
		}
		if !strings.Contains(string(capturedBody), `"referenceType":"change_request"`) {
			t.Errorf("expected referenceType change_request to be injected, got %q", capturedBody)
		}
	})
}

func TestAggregateChangeRequests(t *testing.T) {
	t.Run("requires authenticated user", func(t *testing.T) {
		h := NewChangeRequestHandler(&mockEntityChangeRequestClient{})
		r := httptest.NewRequest(http.MethodPost, "/change-requests/aggregate", strings.NewReader(`{}`))
		w := httptest.NewRecorder()
		h.AggregateChangeRequests(w, r)
		assertStatus(t, w, http.StatusUnauthorized)
		assertErrorMessage(t, w, ErrMsgUnauthorized)
		assertContentType(t, w, "application/json")
	})

	t.Run("rejects body exceeding 1 MiB", func(t *testing.T) {
		h := NewChangeRequestHandler(&mockEntityChangeRequestClient{})
		r := withUser(httptest.NewRequest(http.MethodPost, "/change-requests/aggregate", strings.NewReader(strings.Repeat("x", maxRequestBodyBytes+1))))
		w := httptest.NewRecorder()
		h.AggregateChangeRequests(w, r)
		assertStatus(t, w, http.StatusRequestEntityTooLarge)
		assertErrorMessage(t, w, ErrMsgTooLarge)
		assertContentType(t, w, "application/json")
	})

	t.Run("rejects invalid JSON body", func(t *testing.T) {
		h := NewChangeRequestHandler(&mockEntityChangeRequestClient{})
		r := withUser(httptest.NewRequest(http.MethodPost, "/change-requests/aggregate", strings.NewReader(`not-json`)))
		w := httptest.NewRecorder()
		h.AggregateChangeRequests(w, r)
		assertStatus(t, w, http.StatusBadRequest)
		assertErrorMessage(t, w, ErrMsgBadRequest)
		assertContentType(t, w, "application/json")
	})

	t.Run("forwards body to upstream and returns 200 with response", func(t *testing.T) {
		const reqPayload = `{"filters":{},"groupBy":"state","maxGroups":12}`
		var capturedBody []byte
		client := &mockEntityChangeRequestClient{
			aggregateChangeRequestsFn: func(_ context.Context, body []byte) ([]byte, error) {
				capturedBody = body
				return []byte(`{"groups":[{"key":"open","label":"Open","count":6}],"othersCount":1,"totalRecords":7}`), nil
			},
		}
		h := NewChangeRequestHandler(client)
		r := withUser(httptest.NewRequest(http.MethodPost, "/change-requests/aggregate", strings.NewReader(reqPayload)))
		w := httptest.NewRecorder()
		h.AggregateChangeRequests(w, r)

		assertStatus(t, w, http.StatusOK)
		assertContentType(t, w, "application/json")
		if string(capturedBody) != reqPayload {
			t.Errorf("upstream received body %q, want %q", capturedBody, reqPayload)
		}
		resp := decodeJSON[map[string]any](t, w)
		if resp["totalRecords"] != float64(7) {
			t.Errorf("totalRecords = %v, want 7", resp["totalRecords"])
		}
	})

	t.Run("upstream errors are mapped correctly", func(t *testing.T) {
		for _, tc := range upstreamErrorsGeneric("Failed to aggregate change requests.") {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				client := &mockEntityChangeRequestClient{
					aggregateChangeRequestsFn: func(_ context.Context, _ []byte) ([]byte, error) {
						return nil, tc.err
					},
				}
				h := NewChangeRequestHandler(client)
				r := withUser(httptest.NewRequest(http.MethodPost, "/change-requests/aggregate", strings.NewReader(`{}`)))
				w := httptest.NewRecorder()
				h.AggregateChangeRequests(w, r)
				assertStatus(t, w, tc.wantCode)
				assertErrorMessage(t, w, tc.wantMsg)
				assertContentType(t, w, "application/json")
			})
		}
	})
}

// The creation form's "Customer Approval" / "Customer Review" checkboxes
// (customerApprovalRequired / customerReviewRequired) pass through the BFF
// untouched; only their type is checked here.
func TestCreateChangeRequest_CustomerGateFlags(t *testing.T) {
	t.Run("forwards both checkboxes to the entity service unchanged", func(t *testing.T) {
		const reqPayload = `{"subject":"Deploy v2","type":"normal","customerApprovalRequired":true,"customerReviewRequired":false}`
		var capturedBody []byte
		client := &mockEntityChangeRequestClient{
			createChangeRequestFn: func(_ context.Context, body []byte) ([]byte, error) {
				capturedBody = body
				return []byte(`{"message":"ok"}`), nil
			},
		}
		h := NewChangeRequestHandler(client)
		r := withUser(httptest.NewRequest(http.MethodPost, "/change-requests", strings.NewReader(reqPayload)))
		w := httptest.NewRecorder()
		h.CreateChangeRequest(w, r)
		assertStatus(t, w, http.StatusCreated)
		if string(capturedBody) != reqPayload {
			t.Errorf("upstream received body %q, want %q", capturedBody, reqPayload)
		}
	})

	t.Run("rejects a checkbox that is not a boolean", func(t *testing.T) {
		for name, payload := range map[string]string{
			"approval as string":  `{"subject":"x","type":"normal","customerApprovalRequired":"yes"}`,
			"review as number":    `{"subject":"x","type":"normal","customerReviewRequired":1}`,
			"approval as null":    `{"subject":"x","type":"normal","customerApprovalRequired":null}`,
			"review as object":    `{"subject":"x","type":"normal","customerReviewRequired":{}}`,
			"one good, one wrong": `{"subject":"x","type":"normal","customerApprovalRequired":true,"customerReviewRequired":"true"}`,
		} {
			t.Run(name, func(t *testing.T) {
				called := false
				client := &mockEntityChangeRequestClient{
					createChangeRequestFn: func(_ context.Context, _ []byte) ([]byte, error) {
						called = true
						return nil, nil
					},
				}
				h := NewChangeRequestHandler(client)
				r := withUser(httptest.NewRequest(http.MethodPost, "/change-requests", strings.NewReader(payload)))
				w := httptest.NewRecorder()
				h.CreateChangeRequest(w, r)
				assertStatus(t, w, http.StatusBadRequest)
				if called {
					t.Error("upstream was called with a non-boolean checkbox")
				}
				if msg := w.Body.String(); !strings.Contains(msg, "must be a boolean") {
					t.Errorf("message %q should say the checkbox must be a boolean", msg)
				}
			})
		}
	})
}

func TestPatchChangeRequest_CustomerGateFlags(t *testing.T) {
	patch := func(h *ChangeRequestHandler, payload string) *httptest.ResponseRecorder {
		r := withUser(httptest.NewRequest(http.MethodPatch, "/change-requests/"+testCRID, strings.NewReader(payload)))
		r.SetPathValue("id", testCRID)
		w := httptest.NewRecorder()
		h.PatchChangeRequest(w, r)
		return w
	}

	t.Run("forwards the checkboxes and returns them with legalNextStates", func(t *testing.T) {
		const reqPayload = `{"customerApprovalRequired":true,"customerReviewRequired":true}`
		var capturedBody []byte
		client := &mockEntityChangeRequestClient{
			patchChangeRequestFn: func(_ context.Context, _ string, body []byte) ([]byte, error) {
				capturedBody = body
				return []byte(`{"message":"ok","changeRequest":{"state":"review","customerApprovalRequired":true,"customerReviewRequired":true,"legalNextStates":["customer_review","canceled"]}}`), nil
			},
		}
		w := patch(NewChangeRequestHandler(client), reqPayload)
		assertStatus(t, w, http.StatusOK)
		if string(capturedBody) != reqPayload {
			t.Errorf("upstream received body %q, want %q", capturedBody, reqPayload)
		}
		resp := decodeJSON[map[string]any](t, w)
		cr, _ := resp["changeRequest"].(map[string]any)
		if cr["customerApprovalRequired"] != true || cr["customerReviewRequired"] != true {
			t.Errorf("response flags = %v/%v, want true/true", cr["customerApprovalRequired"], cr["customerReviewRequired"])
		}
	})

	t.Run("rejects a checkbox that is not a boolean", func(t *testing.T) {
		for name, payload := range map[string]string{
			"approval as string": `{"customerApprovalRequired":"true"}`,
			"review as null":     `{"customerReviewRequired":null}`,
			"review as number":   `{"title":"x","customerReviewRequired":0}`,
		} {
			t.Run(name, func(t *testing.T) {
				called := false
				client := &mockEntityChangeRequestClient{
					patchChangeRequestFn: func(_ context.Context, _ string, _ []byte) ([]byte, error) {
						called = true
						return nil, nil
					},
				}
				w := patch(NewChangeRequestHandler(client), payload)
				assertStatus(t, w, http.StatusBadRequest)
				if called {
					t.Error("upstream was called with a non-boolean checkbox")
				}
				if msg := w.Body.String(); !strings.Contains(msg, "must be a boolean") {
					t.Errorf("message %q should say the checkbox must be a boolean", msg)
				}
			})
		}
	})

	// The entity service's refusals are caller-actionable 400s and reach the
	// form verbatim -- an edit after the gate, and a manual transition that the
	// checkboxes rule out.
	t.Run("surfaces the entity service's refusal messages", func(t *testing.T) {
		for name, tc := range map[string]struct{ payload, msg string }{
			"edit after the approval gate": {
				`{"customerApprovalRequired":false}`,
				"customerApprovalRequired can no longer be changed: the change request has already passed the approval stage (current state: scheduled)",
			},
			"edit after review": {
				`{"customerReviewRequired":true}`,
				"customerReviewRequired can no longer be changed: the change request has already left the review stage (current state: closed)",
			},
			"customer_review when not required": {
				`{"state":"customer_review"}`,
				`state "customer_review" cannot be set: customer review is not required for this change request (customerReviewRequired is false); close it from review instead`,
			},
			"closed from review when required": {
				`{"state":"closed"}`,
				`state "closed" cannot be set from review: customer review is required for this change request (customerReviewRequired is true); move it to customer_review first`,
			},
			"scheduled outside customer_approval": {
				`{"state":"scheduled"}`,
				`state "scheduled" cannot be set manually: it is reached automatically through the approval flow (Request Approval, then peer/CAB approval); it can only be set by hand to record the customer's approval, from customer_approval`,
			},
		} {
			t.Run(name, func(t *testing.T) {
				body, _ := json.Marshal(map[string]any{"code": 400, "message": tc.msg})
				client := &mockEntityChangeRequestClient{
					patchChangeRequestFn: func(_ context.Context, _ string, _ []byte) ([]byte, error) {
						return nil, &apierror.Error{StatusCode: http.StatusBadRequest, Body: string(body)}
					},
				}
				w := patch(NewChangeRequestHandler(client), tc.payload)
				assertStatus(t, w, http.StatusBadRequest)
				assertErrorMessage(t, w, tc.msg)
			})
		}
	})
}

func TestGetChangeRequest_ReturnsCustomerGateFlags(t *testing.T) {
	client := &mockEntityChangeRequestClient{
		getChangeRequestFn: func(_ context.Context, _ string) ([]byte, error) {
			return []byte(`{"id":"` + testCRID + `","state":"customer_approval","customerApprovalRequired":true,"customerReviewRequired":false,"legalNextStates":["scheduled","canceled"]}`), nil
		},
	}
	h := NewChangeRequestHandler(client)
	r := withUser(httptest.NewRequest(http.MethodGet, "/change-requests/"+testCRID, nil))
	r.SetPathValue("id", testCRID)
	w := httptest.NewRecorder()
	h.GetChangeRequest(w, r)
	assertStatus(t, w, http.StatusOK)
	resp := decodeJSON[map[string]any](t, w)
	if resp["customerApprovalRequired"] != true || resp["customerReviewRequired"] != false {
		t.Errorf("flags = %v/%v, want true/false", resp["customerApprovalRequired"], resp["customerReviewRequired"])
	}
	states, _ := resp["legalNextStates"].([]any)
	if len(states) != 2 || states[0] != "scheduled" {
		t.Errorf("legalNextStates = %v, want [scheduled canceled] passed through untouched", states)
	}
}
