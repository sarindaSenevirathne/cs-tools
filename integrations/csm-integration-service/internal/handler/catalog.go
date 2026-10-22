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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

// entityCatalogClient abstracts the entity service operations used by CatalogHandler.
type entityCatalogClient interface {
	SearchProducts(ctx context.Context, body []byte) ([]byte, error)
	SearchProductVersions(ctx context.Context, productID string, body []byte) ([]byte, error)
	SearchDeployments(ctx context.Context, body []byte) ([]byte, error)
	SearchDeployedProducts(ctx context.Context, body []byte) ([]byte, error)
}

// CatalogHandler handles the product, product-version, deployment and
// deployed-product searches. Each request body is forwarded to the entity
// service as-is (an empty one as {}) and its response is returned as received,
// except that the enum-valued fields listed in each handler are upper-cased.
type CatalogHandler struct {
	entity entityCatalogClient
}

// NewCatalogHandler creates a CatalogHandler backed by the given entity client.
func NewCatalogHandler(entity entityCatalogClient) *CatalogHandler {
	return &CatalogHandler{entity: entity}
}

// readSearchBody caps and reads the request body and checks it is JSON. An empty
// (or whitespace-only) body becomes "{}", since entity-service rejects a body
// with no JSON object. On failure it writes the error response and returns false.
func readSearchBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if _, ok := err.(*http.MaxBytesError); ok {
			writeError(w, http.StatusRequestEntityTooLarge, ErrMsgTooLarge)
			return nil, false
		}
		writeError(w, http.StatusBadRequest, errMsgReadBody)
		return nil, false
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return []byte("{}"), true
	}
	if !json.Valid(body) {
		writeError(w, http.StatusBadRequest, ErrMsgBadRequest)
		return nil, false
	}
	return body, true
}

// orderedField and orderedObject hold a JSON object's members in their original
// order, with each value kept as its exact original text, so rewriting one field
// leaves every other byte and the key order as received.
type orderedField struct {
	key string
	val json.RawMessage
}

type orderedObject []orderedField

// decodeOrdered reads one JSON object from dec, in member order. It errors when
// the next value is not an object (including null).
func decodeOrdered(dec *json.Decoder) (orderedObject, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, errors.New("not a JSON object")
	}
	var obj orderedObject
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := kt.(string)
		if !ok {
			return nil, errors.New("invalid JSON object key")
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		obj = append(obj, orderedField{key: key, val: raw})
	}
	if _, err := dec.Token(); err != nil { // the closing brace
		return nil, err
	}
	return obj, nil
}

// marshalPlain encodes v as JSON without HTML escaping or a trailing newline.
func marshalPlain(v any) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v) // v is a string here; encoding cannot fail
	return bytes.TrimRight(buf.Bytes(), "\n")
}

func (o orderedObject) bytes() []byte {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, f := range o {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.Write(marshalPlain(f.key))
		buf.WriteByte(':')
		buf.Write(f.val)
	}
	buf.WriteByte('}')
	return buf.Bytes()
}

// upperEnumFields upper-cases the string value of each named field on every
// object in the response's list under listKey (e.g. "products"), leaving
// everything else exactly as received: member order, number text, nulls and
// non-string values are untouched, and a response without that list passes
// through unchanged. It errors unless body is exactly one JSON object (not null,
// not another type, and with nothing after it).
func upperEnumFields(body []byte, listKey string, fields ...string) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	doc, err := decodeOrdered(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("response has data after the JSON object")
	}
	for i, f := range doc {
		if f.key == listKey {
			doc[i].val = upperListFields(f.val, fields)
		}
	}
	return doc.bytes(), nil
}

// upperPath upper-cases the string at path inside obj, where each step after the
// first names a nested object (e.g. {"deployment", "type"}). Only a JSON string
// is rewritten: a null, a missing key or a nested value that is not an object is
// left as it was (unmarshalling null into a string would succeed and turn the
// null into "").
func upperPath(obj orderedObject, path []string) {
	for i, f := range obj {
		if f.key != path[0] {
			continue
		}
		if len(path) == 1 {
			var str string
			if len(f.val) > 0 && f.val[0] == '"' && json.Unmarshal(f.val, &str) == nil {
				obj[i].val = marshalPlain(strings.ToUpper(str))
			}
			continue
		}
		nested, err := decodeOrdered(json.NewDecoder(bytes.NewReader(f.val)))
		if err != nil {
			continue
		}
		upperPath(nested, path[1:])
		obj[i].val = nested.bytes()
	}
}

// upperListFields rewrites the named string fields (a dotted name reaches into a
// nested object) on each object of the JSON array raw; anything that is not an array, or an item that is not an object,
// is returned as it was.
func upperListFields(raw json.RawMessage, fields []string) json.RawMessage {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if tok, err := dec.Token(); err != nil {
		return raw
	} else if d, ok := tok.(json.Delim); !ok || d != '[' {
		return raw
	}
	var items [][]byte
	for dec.More() {
		var item json.RawMessage
		if err := dec.Decode(&item); err != nil {
			return raw
		}
		obj, err := decodeOrdered(json.NewDecoder(bytes.NewReader(item)))
		if err != nil {
			items = append(items, item)
			continue
		}
		for _, name := range fields {
			upperPath(obj, strings.Split(name, "."))
		}
		items = append(items, obj.bytes())
	}
	return append(append([]byte{'['}, bytes.Join(items, []byte{','})...), ']')
}

// writeUpperedJSON upper-cases the given enum fields and writes the result,
// or a 500 if the upstream body cannot be processed.
func writeUpperedJSON(w http.ResponseWriter, r *http.Request, result []byte, listKey string, fields ...string) {
	out, err := upperEnumFields(result, listKey, fields...)
	if err != nil {
		slog.ErrorContext(r.Context(), "catalog response is not a JSON object", "list", listKey, "err", err)
		writeError(w, http.StatusInternalServerError, ErrMsgInternal)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// SearchProducts handles POST /products/search; the product class is upper-cased.
func (h *CatalogHandler) SearchProducts(w http.ResponseWriter, r *http.Request) {
	body, ok := readSearchBody(w, r)
	if !ok {
		return
	}
	result, err := h.entity.SearchProducts(r.Context(), body)
	if err != nil {
		slog.ErrorContext(r.Context(), "entity SearchProducts failed", "err", summarizeErr(err))
		mapUpstreamError(w, err, "Failed to search products.")
		return
	}
	writeUpperedJSON(w, r, result, "products", "class")
}

// SearchProductVersions handles POST /products/{id}/versions/search; the
// support status is upper-cased.
func (h *CatalogHandler) SearchProductVersions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || !uuidRe.MatchString(id) {
		writeError(w, http.StatusBadRequest, ErrMsgInvalidUUID)
		return
	}
	body, ok := readSearchBody(w, r)
	if !ok {
		return
	}
	result, err := h.entity.SearchProductVersions(r.Context(), id, body)
	if err != nil {
		slog.ErrorContext(r.Context(), "entity SearchProductVersions failed", "productID", id, "err", summarizeErr(err))
		mapUpstreamError(w, err, "Failed to search product versions.")
		return
	}
	writeUpperedJSON(w, r, result, "productVersions", "currentSupportStatus")
}

// SearchDeployments handles POST /deployments/search; the deployment type is upper-cased.
func (h *CatalogHandler) SearchDeployments(w http.ResponseWriter, r *http.Request) {
	body, ok := readSearchBody(w, r)
	if !ok {
		return
	}
	result, err := h.entity.SearchDeployments(r.Context(), body)
	if err != nil {
		slog.ErrorContext(r.Context(), "entity SearchDeployments failed", "err", summarizeErr(err))
		mapUpstreamError(w, err, "Failed to search deployments.")
		return
	}
	writeUpperedJSON(w, r, result, "deployments", "type")
}

// SearchDeployedProducts handles POST /deployed-products/search; the product
// category and the deployment's type are upper-cased.
func (h *CatalogHandler) SearchDeployedProducts(w http.ResponseWriter, r *http.Request) {
	body, ok := readSearchBody(w, r)
	if !ok {
		return
	}
	result, err := h.entity.SearchDeployedProducts(r.Context(), body)
	if err != nil {
		slog.ErrorContext(r.Context(), "entity SearchDeployedProducts failed", "err", summarizeErr(err))
		mapUpstreamError(w, err, "Failed to search deployed products.")
		return
	}
	writeUpperedJSON(w, r, result, "deployedProducts", "category", "deployment.type")
}
