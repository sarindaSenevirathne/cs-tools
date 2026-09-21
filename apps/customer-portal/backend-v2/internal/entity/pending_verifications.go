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

package entity

import (
	"context"
	"fmt"
	"net/url"
)

// CreatePendingVerification calls POST /pending-verifications.
func (c *Client) CreatePendingVerification(ctx context.Context, req CreatePendingVerificationRequest) (CreatePendingVerificationResponse, error) {
	var out CreatePendingVerificationResponse
	err := c.postJSON(ctx, "/pending-verifications", req, &out)
	return out, err
}

// SearchPendingVerifications calls POST /pending-verifications/search.
func (c *Client) SearchPendingVerifications(ctx context.Context, req SearchPendingVerificationsRequest) (SearchPendingVerificationsResponse, error) {
	var out SearchPendingVerificationsResponse
	err := c.postJSON(ctx, "/pending-verifications/search", req, &out)
	return out, err
}

// VerifyPendingVerification calls PATCH /pending-verifications/{id}.
func (c *Client) VerifyPendingVerification(ctx context.Context, id string) (VerifyPendingVerificationResponse, error) {
	var out VerifyPendingVerificationResponse
	err := c.patchJSON(ctx, fmt.Sprintf("/pending-verifications/%s", url.PathEscape(id)), struct{}{}, &out)
	return out, err
}
