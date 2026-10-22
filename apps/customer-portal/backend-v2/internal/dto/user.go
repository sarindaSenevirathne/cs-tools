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

// Package dto defines the portal-facing response shapes returned to the
// customer-portal frontend, together with Map* functions that translate
// entity-service's raw response structs into them: the frontend never sees
// an entity-service struct directly, only these.
package dto

import "github.com/wso2-open-operations/cs-tools/apps/customer-portal/backend-v2/internal/entity"

// UserMeResponse is the portal's response for GET /users/me. PhoneNumber and
// LastPasswordUpdateTime are not part of entity-service's response — the
// handler sets them separately from the SCIM client's result, since both are
// sourced from SCIM, not entity-service. IsReadOnly is likewise set by the
// handler, not this package's Map function — it depends on the caller's JWT
// "roles" claim (middleware.UserInfo.Roles), which entity-service's response
// knows nothing about.
type UserMeResponse struct {
	ID                     string   `json:"id"`
	Email                  string   `json:"email"`
	FirstName              *string  `json:"firstName,omitempty"`
	LastName               string   `json:"lastName"`
	TimeZone               *string  `json:"timeZone,omitempty"`
	Roles                  []string `json:"roles"`
	PhoneNumber            *string  `json:"phoneNumber,omitempty"`
	LastPasswordUpdateTime *string  `json:"lastPasswordUpdateTime,omitempty"`
	// IsReadOnly is true only for an INTERNAL-type user (entity-service's own
	// user_type) who holds none of the deployment-configured write roles on
	// their own token. Always false for a genuine customer/partner — their
	// write access is governed entirely by the webapp's own permission
	// matrix, unaffected by this flag. The webapp hides write-path UI when
	// this is true.
	IsReadOnly bool `json:"isReadOnly"`
}

// MapUserMe builds the portal response from entity-service's GetUserMeResponse.
func MapUserMe(u entity.GetUserMeResponse) UserMeResponse {
	return UserMeResponse{
		ID:        u.ID,
		Email:     u.Email,
		FirstName: u.FirstName,
		LastName:  u.LastName,
		TimeZone:  u.TimeZone,
		Roles:     u.Roles,
	}
}

// UserUpdateResponse is the portal's response for PATCH /users/me. Only the
// fields that were actually updated are set — phoneNumber via SCIM,
// timeZone via entity-service.
type UserUpdateResponse struct {
	PhoneNumber *string `json:"phoneNumber,omitempty"`
	TimeZone    *string `json:"timeZone,omitempty"`
}
