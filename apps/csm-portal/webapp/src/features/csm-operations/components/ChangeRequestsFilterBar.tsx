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

import {
  AdapterDateFns,
  Box,
  Button,
  DatePickers,
  Divider,
  Grid,
  IconButton,
  InputAdornment,
  Paper,
  TextField,
  Typography,
} from "@wso2/oxygen-ui";
import {
  ChevronDown,
  ChevronUp,
  ListFilter,
  Search,
  X,
} from "@wso2/oxygen-ui-icons-react";
import { useMemo, useState, type JSX } from "react";
import { useTeams } from "@features/csm-dashboard/api/useTeams";
import {
  CHANGE_REQUEST_FILTER_TYPE_OPTIONS,
  CHANGE_REQUEST_IMPACTS,
  CHANGE_REQUEST_STATES,
  changeRequestImpactLabel,
  changeRequestStateLabel,
  countActiveCRFilters,
  type ChangeRequestFilters,
} from "@features/csm-operations/utils/changeRequests";
import {
  readChangeRequestFiltersFromUrl,
  writeChangeRequestFiltersToUrl,
} from "@features/csm-operations/utils/changeRequestsFiltersUrl";
import SavedViewsMenu from "@features/csm-operations/components/SavedViewsMenu";
import MultiSelectField from "@components/MultiSelectField";
import OperationsAdvancedFilters from "@features/csm-operations/components/OperationsAdvancedFilters";
import {
  countAdvancedRows,
  dateAdvancedField,
  multiAdvancedField,
  type OperationsAdvancedField,
} from "@features/csm-operations/utils/operationsAdvancedFields";
import OperationsAnyOfGroups, { type AnyOfFieldDef } from "@features/csm-operations/components/OperationsAnyOfGroups";
import { completeAnyOfGroups } from "@features/csm-operations/utils/operationsAnyOf";
import AsyncTagMultiSelect from "@features/csm-cases/components/AsyncTagMultiSelect";
import AsyncAssignedUserIdMultiSelect from "@features/csm-cases/components/AsyncAssignedUserIdMultiSelect";
import AsyncProjectMultiSelect from "@features/csm-cases/components/AsyncProjectMultiSelect";

const { DatePicker, LocalizationProvider } = DatePickers;

/** "YYYY-MM-DD" to a local-midnight Date (avoids the UTC-parse day-shift
 * `new Date(dateString)` can cause depending on the viewer's timezone). */
function parseDateOnly(value: string): Date | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
  if (!match) return null;
  const date = new Date(Number(match[1]), Number(match[2]) - 1, Number(match[3]));
  return Number.isNaN(date.getTime()) ? null : date;
}

/** Local-midnight Date back to "YYYY-MM-DD", matching ChangeRequestFilters'
 * closedStartDate/closedEndDate wire format. */
function formatDateOnly(date: Date): string {
  const y = date.getFullYear();
  const m = String(date.getMonth() + 1).padStart(2, "0");
  const d = String(date.getDate()).padStart(2, "0");
  return `${y}-${m}-${d}`;
}

/** Today's UTC calendar date, as a local-midnight Date (see `parseDateOnly`)
 * — the upper bound for both closed-date fields (`crDateOnlyToISOStart`/`End`
 * send them to the backend as UTC day boundaries, same as
 * `IncidentsFilterBar`'s created-date range), since a case can't close in
 * the future. */
function todayUTCDateOnly(): Date {
  const now = new Date();
  return new Date(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate());
}

interface ChangeRequestsFilterBarProps {
  filters: ChangeRequestFilters;
  onChange: (next: ChangeRequestFilters) => void;
  onReset: () => void;
  isFiltersOpen: boolean;
  onFiltersToggle: () => void;
}


export default function ChangeRequestsFilterBar({
  filters,
  onChange,
  onReset,
  isFiltersOpen,
  onFiltersToggle,
}: ChangeRequestsFilterBarProps): JSX.Element {
  const activeCount = countActiveCRFilters(filters);
  const hasActive = activeCount > 0;

  // Recomputed every render (not memoized) — a `useMemo(..., [])` would
  // freeze this at the component's mount date and stop matching "today" for
  // any session left open across a UTC midnight.
  const today = todayUTCDateOnly();
  const closedStartDate = parseDateOnly(filters.closedStartDate);
  const closedEndDate = parseDateOnly(filters.closedEndDate);
  const fromMaxDate = closedEndDate && closedEndDate < today ? closedEndDate : today;

  const stateOptions = useMemo(
    () =>
      CHANGE_REQUEST_STATES.map((s) => ({
        value: s,
        label: changeRequestStateLabel(s),
      })),
    [],
  );
  const impactOptions = useMemo(
    () =>
      CHANGE_REQUEST_IMPACTS.map((i) => ({
        value: i,
        label: changeRequestImpactLabel(i),
      })),
    [],
  );


  // SRE Team: change requests are SRE-owned, so this control is scoped to the
  // `sre-abt` team family and listed the way the cases list lists its CRE
  // teams — see `IncidentsFilterBar`'s equivalent note on why this isn't
  // `cre-abt`. It matches the record's assignment group (the team's group id).
  const { data: teams } = useTeams(true, "sre-abt");
  const sreTeamOptions = useMemo(
    () =>
      (teams ?? [])
        .filter(
          (t): t is typeof t & { sreGroupId: string } =>
            Boolean(t.sreGroupId) && t.family === "sre-abt",
        )
        .map((t) => ({ value: t.sreGroupId, label: t.name })),
    [teams],
  );

  // Value pickers shared by the Advanced rows and the OR-group conditions, so
  // a field is chosen the same way wherever it appears.
  const pickers = useMemo(() => {
    type P = { id: string; values: string[]; onChange: (values: string[]) => void };
    return {
      state: ({ id, values, onChange: set }: P) => (
        <MultiSelectField id={id} label="Value(s)" values={values} options={stateOptions} onChange={set} />
      ),
      impact: ({ id, values, onChange: set }: P) => (
        <MultiSelectField id={id} label="Value(s)" values={values} options={impactOptions} onChange={set} />
      ),
      type: ({ id, values, onChange: set }: P) => (
        <MultiSelectField
          id={id}
          label="Value(s)"
          values={values}
          options={CHANGE_REQUEST_FILTER_TYPE_OPTIONS}
          onChange={set}
        />
      ),
      project: ({ id, values, onChange: set }: P) => (
        <AsyncProjectMultiSelect id={id} label="Value(s)" values={values} onChange={set} />
      ),
      tag: ({ id, values, onChange: set }: P) => (
        <AsyncTagMultiSelect id={id} label="Value(s)" values={values} onChange={set} />
      ),
      group: ({ id, values, onChange: set }: P) => (
        <MultiSelectField id={id} label="Value(s)" values={values} options={sreTeamOptions} onChange={set} />
      ),
      assignee: ({ id, values, onChange: set }: P) => (
        <AsyncAssignedUserIdMultiSelect id={id} values={values} onChange={set} />
      ),
    };
  }, [stateOptions, impactOptions, sreTeamOptions]);

  // Advanced offers every filter as a Field / Operator / Value row, as on the
  // service request list; each row reads and writes the same state the quick
  // grid does, so a filter set either way shows in both.
  const advancedFields = useMemo<OperationsAdvancedField<ChangeRequestFilters>[]>(
    () => [
      multiAdvancedField<ChangeRequestFilters>({ key: "state", label: "State", prop: "states", render: pickers.state }),
      multiAdvancedField<ChangeRequestFilters>({ key: "impact", label: "Impact", prop: "impacts", render: pickers.impact }),
      multiAdvancedField<ChangeRequestFilters>({ key: "project", label: "Project", prop: "projectIds", render: pickers.project }),
      multiAdvancedField<ChangeRequestFilters>({ key: "type", label: "Type", prop: "types", render: pickers.type }),
      multiAdvancedField<ChangeRequestFilters>({ key: "tag", label: "Tags", prop: "tags", render: pickers.tag }),
      multiAdvancedField<ChangeRequestFilters>({ key: "group", label: "SRE Team", prop: "sreTeamIds", render: pickers.group }),
      multiAdvancedField<ChangeRequestFilters>({ key: "assignee", label: "Assigned engineer", prop: "assigneeIds", render: pickers.assignee }),
      dateAdvancedField<ChangeRequestFilters>({ key: "created", label: "Created", from: "createdStartDate", to: "createdEndDate" }),
      dateAdvancedField<ChangeRequestFilters>({ key: "updated", label: "Updated", from: "updatedStartDate", to: "updatedEndDate" }),
      dateAdvancedField<ChangeRequestFilters>({ key: "closed", label: "Closed", from: "closedStartDate", to: "closedEndDate" }),
      dateAdvancedField<ChangeRequestFilters>({
        key: "plannedStart", label: "Planned start", from: "plannedStartFrom", to: "plannedStartTo", capAtToday: false,
      }),
      dateAdvancedField<ChangeRequestFilters>({
        key: "plannedEnd", label: "Planned end", from: "plannedEndFrom", to: "plannedEndTo", capAtToday: false,
      }),
    ],
    [pickers],
  );
  // OR-group conditions: the fields the search accepts inside a group.
  const anyOfFields = useMemo<AnyOfFieldDef[]>(
    () => [
      { field: "state", label: "State", render: pickers.state },
      { field: "impact", label: "Impact", render: pickers.impact },
      { field: "projectId", label: "Project", render: pickers.project },
      { field: "type", label: "Type", render: pickers.type },
      { field: "tag", label: "Tags", render: pickers.tag },
      { field: "assignmentGroupId", label: "SRE Team", render: pickers.group },
      { field: "assignedUserId", label: "Assigned engineer", render: pickers.assignee },
    ],
    [pickers],
  );
  const advancedCount =
    countAdvancedRows(advancedFields, filters) + completeAnyOfGroups(filters.anyOf).length;
  const [mode, setMode] = useState<"quick" | "advanced">("quick");

  /**
   * `minDate`/`maxDate` only constrain the calendar popup — MUI's DatePicker
   * still fires `onChange` for an out-of-range value typed directly into the
   * field, so each handler re-checks the same bound here before accepting it,
   * rather than trusting the picker's UI-only validation.
   */
  const handleClosedStartChange = (date: unknown): void => {
    if (!(date instanceof Date) || Number.isNaN(date.getTime())) {
      onChange({ ...filters, closedStartDate: "" });
      return;
    }
    if (date > fromMaxDate) return;
    onChange({ ...filters, closedStartDate: formatDateOnly(date) });
  };

  const handleClosedEndChange = (date: unknown): void => {
    if (!(date instanceof Date) || Number.isNaN(date.getTime())) {
      onChange({ ...filters, closedEndDate: "" });
      return;
    }
    if (date > today) return;
    if (closedStartDate && date < closedStartDate) return;
    onChange({ ...filters, closedEndDate: formatDateOnly(date) });
  };

  return (
    <Paper sx={{ p: 2.5, display: "flex", flexDirection: "column", gap: 1.5 }}>
      {/* Search bar + filters toggle */}
      <Box
        sx={{ display: "flex", alignItems: "center", gap: 1.5, flexWrap: "wrap" }}
      >
        <Box sx={{ position: "relative", flex: 1, minWidth: 240 }}>
          <TextField
            fullWidth
            size="small"
            placeholder="Search by number or subject…"
            value={filters.search}
            onChange={(e) => onChange({ ...filters, search: e.target.value })}
            slotProps={{
              input: {
                startAdornment: (
                  <InputAdornment position="start">
                    <Search size={16} />
                  </InputAdornment>
                ),
                endAdornment: filters.search ? (
                  <InputAdornment position="end">
                    <IconButton
                      size="small"
                      edge="end"
                      onClick={() => onChange({ ...filters, search: "" })}
                      aria-label="Clear search"
                    >
                      <X size={16} />
                    </IconButton>
                  </InputAdornment>
                ) : undefined,
              },
            }}
          />
        </Box>

        <SavedViewsMenu
          currentQs={writeChangeRequestFiltersToUrl(filters).toString()}
          canonicalizeQs={(qs) =>
            writeChangeRequestFiltersToUrl(
              readChangeRequestFiltersFromUrl(new URLSearchParams(qs)),
            ).toString()
          }
          activeCount={activeCount}
          hasSearch={filters.search.trim().length > 0}
          onApply={(qs) => onChange(readChangeRequestFiltersFromUrl(new URLSearchParams(qs)))}
          listKey="change_requests"
        />

        <Button
          variant="outlined"
          size="small"
          color="primary"
          onClick={onFiltersToggle}
          startIcon={<ListFilter size={16} />}
          endIcon={isFiltersOpen ? <ChevronUp size={16} /> : <ChevronDown size={16} />}
        >
          {hasActive ? `Filters (${activeCount})` : "Filters"}
        </Button>
        {hasActive && (
          <Button
            variant="text"
            size="small"
            color="primary"
            onClick={onReset}
            startIcon={<X size={16} />}
          >
            Clear filters
          </Button>
        )}
      </Box>

      {/* Collapsible filter grid */}
      {isFiltersOpen && (
        <>
          <Divider />
          <Box role="group" aria-label="Filter mode" sx={{ display: "flex", alignSelf: "flex-start" }}>
            <Button
              size="small"
              variant={mode === "quick" ? "contained" : "outlined"}
              color={mode === "quick" ? "primary" : "inherit"}
              aria-pressed={mode === "quick"}
              onClick={() => setMode("quick")}
              sx={{ borderTopRightRadius: 0, borderBottomRightRadius: 0 }}
            >
              Quick filters
            </Button>
            <Button
              size="small"
              variant={mode === "advanced" ? "contained" : "outlined"}
              color={mode === "advanced" ? "primary" : "inherit"}
              aria-pressed={mode === "advanced"}
              onClick={() => setMode("advanced")}
              sx={{ borderTopLeftRadius: 0, borderBottomLeftRadius: 0, ml: "-1px" }}
            >
              {advancedCount > 0 ? `Advanced (${advancedCount})` : "Advanced"}
            </Button>
          </Box>
          {mode === "advanced" ? (
            <>
              <OperationsAdvancedFilters
                idPrefix="cr"
                fields={advancedFields}
                filters={filters}
                onChange={onChange}
              />
              <Divider />
              <OperationsAnyOfGroups
                idPrefix="cr"
                fields={anyOfFields}
                groups={filters.anyOf}
                onChange={(anyOf) => onChange({ ...filters, anyOf })}
              />
            </>
          ) : (
          <Grid container spacing={2}>
            <Grid size={{ xs: 12, sm: 6, md: 3 }}>
              <MultiSelectField
                id="cr-filter-state"
                label="State"
                values={filters.states}
                options={stateOptions}
                onChange={(next) => onChange({ ...filters, states: next })}
              />
            </Grid>
            <Grid size={{ xs: 12, sm: 6, md: 3 }}>
              <MultiSelectField
                id="cr-filter-impact"
                label="Impact"
                values={filters.impacts}
                options={impactOptions}
                onChange={(next) => onChange({ ...filters, impacts: next })}
              />
            </Grid>
            <Grid size={{ xs: 12, sm: 6, md: 3 }}>
              <MultiSelectField
                id="cr-filter-sre-team"
                label="SRE Team"
                values={filters.sreTeamIds}
                options={sreTeamOptions}
                onChange={(next) => onChange({ ...filters, sreTeamIds: next })}
              />
            </Grid>
            <Grid size={{ xs: 12, sm: 6, md: 3 }}>
              <AsyncProjectMultiSelect
                id="cr-filter-project"
                values={filters.projectIds}
                onChange={(next) => onChange({ ...filters, projectIds: next })}
              />
            </Grid>
            <Grid size={{ xs: 12, sm: 6, md: 3 }}>
              <MultiSelectField
                id="cr-filter-type"
                label="Type"
                values={filters.types}
                options={CHANGE_REQUEST_FILTER_TYPE_OPTIONS}
                onChange={(next) => onChange({ ...filters, types: next })}
              />
            </Grid>
            <Grid size={{ xs: 12, sm: 6, md: 3 }}>
              <AsyncTagMultiSelect
                id="cr-filter-tag"
                label="Tags"
                values={filters.tags}
                onChange={(next) => onChange({ ...filters, tags: next })}
              />
            </Grid>
            <Grid size={{ xs: 12, sm: 6, md: 3 }}>
              <LocalizationProvider dateAdapter={AdapterDateFns}>
                <DatePicker
                  label="Closed from"
                  value={closedStartDate}
                  maxDate={fromMaxDate}
                  onChange={handleClosedStartChange}
                  slotProps={{
                    textField: { size: "small", fullWidth: true },
                    field: { clearable: true },
                  }}
                />
              </LocalizationProvider>
            </Grid>
            <Grid size={{ xs: 12, sm: 6, md: 3 }}>
              <LocalizationProvider dateAdapter={AdapterDateFns}>
                <DatePicker
                  label="Closed to"
                  value={closedEndDate}
                  minDate={closedStartDate ?? undefined}
                  maxDate={today}
                  onChange={handleClosedEndChange}
                  slotProps={{
                    textField: { size: "small", fullWidth: true },
                    field: { clearable: true },
                  }}
                />
              </LocalizationProvider>
            </Grid>
          </Grid>
          )}
          {activeCount > 0 && (
            <Box sx={{ display: "flex", justifyContent: "flex-end" }}>
              <Typography variant="caption" color="text.secondary">
                {activeCount} {activeCount === 1 ? "filter" : "filters"} active
              </Typography>
            </Box>
          )}
        </>
      )}
    </Paper>
  );
}
