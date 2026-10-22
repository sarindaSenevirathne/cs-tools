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
	"context"
	"fmt"
	"strings"

	"github.com/wso2-open-operations/cs-tools/entity-service/internal/apierror"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/domain"
	"github.com/wso2-open-operations/cs-tools/entity-service/internal/repository"
)

// Bounds on an anyOf request, so one search cannot build an unbounded OR.
const (
	maxListFilterBranches        = 10
	maxListFilterRowsPerBranch   = 10
	maxListFilterValuesPerFilter = 100
)

func anyOfFilterError(branch int, format string, args ...any) error {
	return &apierror.ValidationError{Msg: fmt.Sprintf("filters.anyOf[%d]: ", branch) + fmt.Sprintf(format, args...)}
}

func checkAnyOfShape(branches, rows, values int, branch int, field, op string) error {
	switch {
	case branches > maxListFilterBranches:
		return &apierror.ValidationError{Msg: fmt.Sprintf("filters.anyOf: at most %d branches", maxListFilterBranches)}
	case rows == 0:
		return anyOfFilterError(branch, "a branch needs at least one filter")
	case rows > maxListFilterRowsPerBranch:
		return anyOfFilterError(branch, "at most %d filters per branch", maxListFilterRowsPerBranch)
	case field != "" && op != "in":
		return anyOfFilterError(branch, "field %q supports op \"in\" only", field)
	case field != "" && values == 0:
		return anyOfFilterError(branch, "field %q needs at least one value", field)
	case values > maxListFilterValuesPerFilter:
		return anyOfFilterError(branch, "field %q takes at most %d values", field, maxListFilterValuesPerFilter)
	}
	return nil
}

// parseIncidentAnyOfPostgres parses SearchIncidentsFilters.AnyOf into OrGroups.
func parseIncidentAnyOfPostgres(f *domain.SearchIncidentsFilters) error {
	if len(f.AnyOf) == 0 {
		return nil
	}
	if err := checkAnyOfShape(len(f.AnyOf), 1, 0, 0, "", ""); err != nil {
		return err
	}
	groups := make([]domain.IncidentFilterGroup, 0, len(f.AnyOf))
	for bi, b := range f.AnyOf {
		if err := checkAnyOfShape(len(f.AnyOf), len(b.Filters), 0, bi, "", ""); err != nil {
			return err
		}
		var g domain.IncidentFilterGroup
		for _, ff := range b.Filters {
			if err := checkAnyOfShape(len(f.AnyOf), len(b.Filters), len(ff.Values), bi, ff.Field, ff.Op); err != nil {
				return err
			}
			switch ff.Field {
			case "state":
				for _, v := range ff.Values {
					st := domain.IncidentState(v)
					if !validIncidentState[st] {
						return anyOfFilterError(bi, "state contains invalid value: %s", v)
					}
					g.States = append(g.States, incidentStateToEnum(st))
				}
			case "priority":
				for _, v := range ff.Values {
					e, ok := incidentPriorityToEnum(domain.IncidentPriority(v))
					if !ok {
						return anyOfFilterError(bi, "priority has no equivalent on this data source: %s", v)
					}
					g.Priorities = append(g.Priorities, e)
				}
			case "category":
				for _, v := range ff.Values {
					g.Categories = append(g.Categories, strings.ToUpper(strings.TrimSpace(v)))
				}
			case "businessServiceId", "assignmentGroupId", "assignedUserId":
				if err := validateUUIDs(fmt.Sprintf("filters.anyOf[%d]: %s", bi, ff.Field), ff.Values); err != nil {
					return err
				}
				switch ff.Field {
				case "businessServiceId":
					g.ServiceIDs = append(g.ServiceIDs, ff.Values...)
				case "assignmentGroupId":
					g.AssignmentGroupIDs = append(g.AssignmentGroupIDs, ff.Values...)
				default:
					g.AssignedUserIDs = append(g.AssignedUserIDs, ff.Values...)
				}
			default:
				return anyOfFilterError(bi, "unsupported field: %s", ff.Field)
			}
		}
		groups = append(groups, g)
	}
	f.OrGroups = groups
	return nil
}

// parseChangeRequestAnyOfPostgres parses SearchChangeRequestsFilters.AnyOf into
// OrGroups. A tag inside a branch is internal callers only, like the top-level
// tag filter (tags are never shown to a customer).
func parseChangeRequestAnyOfPostgres(ctx context.Context, f *domain.SearchChangeRequestsFilters) error {
	if len(f.AnyOf) == 0 {
		return nil
	}
	if err := checkAnyOfShape(len(f.AnyOf), 1, 0, 0, "", ""); err != nil {
		return err
	}
	groups := make([]domain.ChangeRequestFilterGroup, 0, len(f.AnyOf))
	for bi, b := range f.AnyOf {
		if err := checkAnyOfShape(len(f.AnyOf), len(b.Filters), 0, bi, "", ""); err != nil {
			return err
		}
		var g domain.ChangeRequestFilterGroup
		for _, ff := range b.Filters {
			if err := checkAnyOfShape(len(f.AnyOf), len(b.Filters), len(ff.Values), bi, ff.Field, ff.Op); err != nil {
				return err
			}
			switch ff.Field {
			case "state":
				for _, v := range ff.Values {
					if !validChangeRequestState[domain.ChangeRequestState(v)] {
						return anyOfFilterError(bi, "state contains invalid value: %s", v)
					}
					g.States = append(g.States, strings.ToUpper(v))
				}
			case "impact":
				for _, v := range ff.Values {
					if !validChangeRequestImpact[domain.ChangeRequestImpact(v)] {
						return anyOfFilterError(bi, "impact contains invalid value: %s", v)
					}
					g.Impacts = append(g.Impacts, strings.ToUpper(v))
				}
			case "type":
				for _, v := range ff.Values {
					t := domain.ChangeRequestType(strings.ToLower(strings.TrimSpace(v)))
					model, ok := repository.ChangeModelForType(t)
					if !ok {
						return anyOfFilterError(bi, "type has no equivalent on this data source: %s", v)
					}
					g.ChangeModels = append(g.ChangeModels, model)
				}
			case "tag":
				if scope, ok := repository.CallerIdentityFromContext(ctx); !ok || !scope.Unrestricted {
					return &apierror.ForbiddenError{Msg: "filters: field \"tag\" is only available to internal callers"}
				}
				g.Tags = append(g.Tags, ff.Values...)
			case "projectId", "assignmentGroupId", "assignedUserId":
				if err := validateUUIDs(fmt.Sprintf("filters.anyOf[%d]: %s", bi, ff.Field), ff.Values); err != nil {
					return err
				}
				switch ff.Field {
				case "projectId":
					g.ProjectIDs = append(g.ProjectIDs, ff.Values...)
				case "assignmentGroupId":
					g.AssignmentGroupIDs = append(g.AssignmentGroupIDs, ff.Values...)
				default:
					g.AssignedUserIDs = append(g.AssignedUserIDs, ff.Values...)
				}
			default:
				return anyOfFilterError(bi, "unsupported field: %s", ff.Field)
			}
		}
		groups = append(groups, g)
	}
	f.OrGroups = groups
	return nil
}
