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

package service

import (
	"fmt"
	"strconv"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
)

// This file is the one work-item-agnostic SLA filter layer. A work-item type
// (case, incident, ...) that wants the two SLA filters registers by:
//
//  1. listing slaFilterFieldBreached / slaFilterFieldPercent in its own
//     filter-field allow-list and routing both to parseSLAFilter from its
//     parser (one parser, one error-message set);
//  2. building its Postgres predicate with the repository package's shared
//     SLA predicate builders, keyed by its work-item id expression;
//  3. embedding slaDelegate in its service and implementing setSLADelegate,
//     so WithSLASearchDelegate (SLA_DATA_SOURCE routing) works unchanged.

// SLA filter wire field names, identical for every work-item type.
const (
	// slaFilterFieldBreached: eq, one boolean value. true/false restricts to
	// items with/without a currently-breached SLA record.
	slaFilterFieldBreached = "slaBreached"
	// slaFilterFieldPercent: gte/lte, one non-negative integer. Restricts to
	// items having an SLA record whose business-elapsed percentage is in range.
	slaFilterFieldPercent = "taskSLABusinessElapsedPercent"
)

// isSLAFilterField reports whether field is one of the two shared SLA filters.
func isSLAFilterField(field string) bool {
	return field == slaFilterFieldBreached || field == slaFilterFieldPercent
}

// parseSLAFilter applies one SLA filter entry (field must satisfy
// isSLAFilterField) to *taskSLA / *breached. Both bounds of a percent range
// accumulate into the same TaskSLAFilter, so gte+lte describe one SLA record.
func parseSLAFilter(field, op string, values []string, taskSLA **domain.TaskSLAFilter, breached **bool) error {
	switch field {
	case slaFilterFieldPercent:
		if len(values) == 0 {
			return &apierror.ValidationError{Msg: fmt.Sprintf("filters: field %q op %q requires a non-empty values array", field, op)}
		}
		pct, err := parseSLAFilterPercent(field, op, values[0])
		if err != nil {
			return err
		}
		if *taskSLA == nil {
			*taskSLA = &domain.TaskSLAFilter{}
		}
		switch op {
		case "gte":
			(*taskSLA).MinBusinessElapsedPercent = &pct
		case "lte":
			(*taskSLA).MaxBusinessElapsedPercent = &pct
		default:
			return &apierror.ValidationError{Msg: fmt.Sprintf("filters: field %q does not support op %q", field, op)}
		}
	case slaFilterFieldBreached:
		if op != "eq" {
			return &apierror.ValidationError{Msg: fmt.Sprintf("filters: field %q does not support op %q", field, op)}
		}
		if len(values) == 0 {
			return &apierror.ValidationError{Msg: fmt.Sprintf("filters: field %q op %q requires a non-empty values array", field, op)}
		}
		if len(values) != 1 {
			return &apierror.ValidationError{Msg: "filters: slaBreached eq requires exactly one value"}
		}
		b, err := parseBoolFilterValue(field, op, values[0])
		if err != nil {
			return err
		}
		*breached = &b
	}
	return nil
}

// parseSLAFilterPercent parses a single filter value into a non-negative
// integer percentage. No upper bound: a long-overdue, never-resolved SLA's
// business-elapsed percentage keeps climbing well past 100 (observed up to
// 45950) rather than capping there -- a value like 100 means "breached", not
// "the maximum possible value".
func parseSLAFilterPercent(field, op, value string) (int, error) {
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return 0, &apierror.ValidationError{Msg: fmt.Sprintf("filters: field %q op %q value %q must be a non-negative integer", field, op, value)}
	}
	return n, nil
}

// parseBoolFilterValue accepts only the exact lowercase "true"/"false" the
// OpenAPI contract documents -- not every string strconv.ParseBool tolerates.
func parseBoolFilterValue(field, op, value string) (bool, error) {
	switch value {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, &apierror.ValidationError{Msg: fmt.Sprintf("filters: field %q op %q value %q must be a boolean", field, op, value)}
	}
}

// slaDelegate is the work-item-agnostic SLA_DATA_SOURCE routing state: a
// backing-system service of the same type S as the owning service, that a
// search is forwarded to wholesale (there is no narrower endpoint answering
// "which ids match this SLA condition alone") whenever it carries an
// SLA-shaped filter and the caller is internal. The zero value delegates
// nothing, so a service behaves exactly as before when SLA_DATA_SOURCE is
// postgres.
type slaDelegate[S any] struct {
	backend S
	set     bool
}

// route returns the delegate and true when the search should be forwarded.
// internalCaller is false for a customer-portal-scoped caller, which keeps the
// owning data source's own row-level security regardless of the toggle.
func (d *slaDelegate[S]) route(internalCaller, slaShaped bool) (S, bool) {
	if d.set && internalCaller && slaShaped {
		return d.backend, true
	}
	var zero S
	return zero, false
}

// slaDelegateAcceptor is implemented by every service type that supports SLA
// delegation; S is the service interface (CaseService, IncidentService, ...).
type slaDelegateAcceptor[S any] interface {
	setSLADelegate(delegate S)
}

// WithSLASearchDelegate attaches the backing-system service that SLA-filtered
// searches are forwarded to (wired only when config.SLADataSource is
// servicenow). Works for any work-item service implementing
// slaDelegateAcceptor; a no-op otherwise -- defensive, every real
// construction path is one. D is separate from S only so callers may pass a
// concrete delegate type.
func WithSLASearchDelegate[S any, D any](svc S, delegate D) S {
	acceptor, ok := any(svc).(slaDelegateAcceptor[S])
	if !ok {
		return svc
	}
	if d, ok := any(delegate).(S); ok {
		acceptor.setSLADelegate(d)
	}
	return svc
}
