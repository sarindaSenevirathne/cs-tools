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

package auth

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
)

const userTokenHeader = "x-user-id-token" // #nosec G101 -- header name, not a credential

// Identity is the caller identity established for a request.
type Identity struct {
	// Validated is true only when token validation is enabled and every token
	// presented on the request passed it. When false, nothing in the request
	// may be trusted, and anything that scopes results by caller must refuse.
	Validated bool
	// UserEmail/UserSubject come from a validated x-user-id-token; empty when
	// the request carried none (a machine-to-machine call).
	UserEmail   string
	UserSubject string
	// ClientID comes from a validated Authorization: Bearer token; empty when
	// the request carried none.
	ClientID string
}

type ctxKey struct{}

// WithIdentity returns a copy of ctx carrying id. Tests use it to inject an
// identity without minting tokens.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// IdentityFromContext returns the request's Identity, or the zero (unvalidated)
// Identity if the middleware did not run.
func IdentityFromContext(ctx context.Context) Identity {
	id, _ := ctx.Value(ctxKey{}).(Identity)
	return id
}

// Middleware validates the tokens on every request and attaches the resulting
// Identity to the context. routes.go always supplies a real Validator -- there
// is no config flag to disable this. A nil Validator is a purely defensive
// fallback (attaches an unvalidated Identity and never rejects); it should
// only ever happen if a caller wires this middleware without one, a bug.
//
// A token that is PRESENT but invalid is always rejected with 401 -- never
// downgraded to "no token", which would turn a forged user token into an
// anonymous (and, for a system client, less restricted) request. A request
// with no tokens at all passes through: whether that is acceptable is decided
// per endpoint by the service that scopes by caller.
func Middleware(v *Validator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if v == nil {
				next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), Identity{})))
				return
			}

			id := Identity{Validated: true}

			if raw := bearerToken(r); raw != "" {
				cc, err := v.ValidateClientToken(raw)
				if err != nil {
					reject(w, r, "authorization bearer token", err)
					return
				}
				id.ClientID = cc.ClientID
			}
			if raw := strings.TrimSpace(r.Header.Get(userTokenHeader)); raw != "" {
				uc, err := v.ValidateUserToken(raw)
				if err != nil {
					reject(w, r, userTokenHeader, err)
					return
				}
				id.UserEmail, id.UserSubject = uc.Email, uc.Subject
			}

			next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), id)))
		})
	}
}

func bearerToken(r *http.Request) string {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(h) < 8 || !strings.EqualFold(h[:7], "bearer ") {
		return ""
	}
	return strings.TrimSpace(h[7:])
}

// reject logs why (never the token itself) and answers 401 with a generic body.
func reject(w http.ResponseWriter, r *http.Request, which string, err error) {
	slog.WarnContext(r.Context(), "auth: token rejected", "token", which, "err", err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(apierror.ErrorResponse{Code: http.StatusUnauthorized, Message: "invalid or expired token"})
}
