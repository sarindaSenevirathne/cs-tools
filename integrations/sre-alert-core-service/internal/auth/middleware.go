// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
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
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

const bearerPrefix = "Bearer "

// verifiedTTL is how long a verified credential skips the lookup and PBKDF2 check; a disable or rotation takes up to this long to apply.
const verifiedTTL = 60 * time.Second

// verifiedCache holds a SHA-256 of each recently verified secret (never the secret), so a wake burst costs one PBKDF2 check per minute.
type verifiedCache struct {
	mu      sync.Mutex
	entries map[string]verifiedEntry
}

type verifiedEntry struct {
	digest  [sha256.Size]byte
	expires time.Time
}

func (c *verifiedCache) hit(username, secret string, now time.Time) bool {
	c.mu.Lock()
	e, ok := c.entries[username]
	c.mu.Unlock()
	digest := sha256.Sum256([]byte(secret))
	return ok && now.Before(e.expires) && subtle.ConstantTimeCompare(e.digest[:], digest[:]) == 1
}

func (c *verifiedCache) remember(u User, secret string, now time.Time) {
	expires := now.Add(verifiedTTL)
	if u.ExpiresAt.After(time.Unix(0, 0)) && u.ExpiresAt.Before(expires) {
		expires = u.ExpiresAt
	}
	c.mu.Lock()
	c.entries[u.Username] = verifiedEntry{digest: sha256.Sum256([]byte(secret)), expires: expires}
	c.mu.Unlock()
}

// dummySalt is used only to burn CPU time on an unknown-user auth attempt, never for real secret storage.
var dummySalt = []byte("integration-users-timing-salt!!")

// RequireAuth requires a valid Authorization header (Bearer base64("<username>:<secret>"), or Basic i.e. -u) naming an enabled integration_users row; every failure is a generic 401, and only the username is logged, never the secret.
func RequireAuth(repo *UserRepo, logger *slog.Logger) func(http.Handler) http.Handler {
	cache := &verifiedCache{entries: map[string]verifiedEntry{}}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			username, secret, ok := parseCredentials(r)
			if !ok {
				logger.Warn("auth: missing or malformed Authorization header", "path", r.URL.Path)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			if cache.hit(username, secret, time.Now()) {
				next.ServeHTTP(w, r)
				return
			}

			u, err := repo.Get(r.Context(), username)
			if err != nil {
				if !errors.Is(err, ErrUserNotFound) {
					logger.Error("auth: lookup failed", "username", username, "error", err)
				} else {
					logger.Warn("auth: unknown user", "username", username)
					// Burn comparable time to a real VerifySecret call so response timing can't be used to enumerate usernames.
					HashSecret(secret, dummySalt, Iterations)
				}
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			if !u.Enabled {
				logger.Warn("auth: disabled user", "username", username)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			if !VerifySecret(secret, u.Salt, u.SecretHash, u.Iterations) {
				logger.Warn("auth: secret mismatch", "username", username)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			if u.IsExpired(time.Now()) {
				logger.Warn("auth: secret expired", "username", username)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			cache.remember(u, secret, time.Now())
			next.ServeHTTP(w, r)
		})
	}
}

// parseCredentials extracts username/secret from either Bearer base64("<username>:<secret>") or Basic (what -u sends, decoded via net/http's BasicAuth).
func parseCredentials(r *http.Request) (username, secret string, ok bool) {
	// Case-insensitive per RFC 7235, as BasicAuth already is for Basic.
	if header := r.Header.Get("Authorization"); len(header) >= len(bearerPrefix) &&
		strings.EqualFold(header[:len(bearerPrefix)], bearerPrefix) {
		token := header[len(bearerPrefix):]
		decoded, err := base64.StdEncoding.DecodeString(token)
		if err != nil {
			return "", "", false
		}
		username, secret, found := strings.Cut(string(decoded), ":")
		if !found || username == "" || secret == "" {
			return "", "", false
		}
		return username, secret, true
	}

	username, secret, ok = r.BasicAuth()
	if !ok || username == "" || secret == "" {
		return "", "", false
	}
	return username, secret, true
}
