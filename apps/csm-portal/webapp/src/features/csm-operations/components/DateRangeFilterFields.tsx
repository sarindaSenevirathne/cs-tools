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

import { AdapterDateFns, DatePickers, Grid } from "@wso2/oxygen-ui";
import type { JSX } from "react";
import { formatDateOnly, parseDateOnly, todayUTCDateOnly } from "@features/csm-operations/utils/dateOnly";

const { DatePicker, LocalizationProvider } = DatePickers;

export interface DateRange {
  /** `YYYY-MM-DD` (a UTC calendar date on the wire), or empty. */
  from: string;
  /** `YYYY-MM-DD` (a UTC calendar date on the wire), or empty. */
  to: string;
}

interface DateRangeFilterFieldsProps {
  /** e.g. "Updated" → "Updated from (UTC)" / "Updated to (UTC)". */
  label: string;
  value: DateRange;
  onChange: (next: DateRange) => void;
  /** Caps both ends at today (UTC) — for events that cannot happen in the
   * future (created, updated, closed). Off for planned dates. */
  capAtToday?: boolean;
  /** Grid column sizing of each of the two pickers. */
  size?: { xs?: number; sm?: number; md?: number };
}

/**
 * A from/to date pair for a list filter bar, following this app's date-range
 * rules (webapp CLAUDE.md, "Filter-bar date ranges must cap at today"): "from"
 * can't pass "to", and when `capAtToday` is set neither can pass today. Every
 * bound is re-checked in onChange too, since MUI still fires onChange for an
 * out-of-range value typed into the field.
 */
export default function DateRangeFilterFields({
  label,
  value,
  onChange,
  capAtToday = true,
  size = { xs: 12, sm: 6, md: 3 },
}: DateRangeFilterFieldsProps): JSX.Element {
  const today = capAtToday ? todayUTCDateOnly() : null;
  const from = parseDateOnly(value.from);
  const to = parseDateOnly(value.to);
  const fromMax = to && (!today || to < today) ? to : today;

  const handleFrom = (date: unknown): void => {
    if (!(date instanceof Date) || Number.isNaN(date.getTime())) {
      onChange({ ...value, from: "" });
      return;
    }
    if (fromMax && date > fromMax) return;
    onChange({ ...value, from: formatDateOnly(date) });
  };

  const handleTo = (date: unknown): void => {
    if (!(date instanceof Date) || Number.isNaN(date.getTime())) {
      onChange({ ...value, to: "" });
      return;
    }
    if (today && date > today) return;
    if (from && date < from) return;
    onChange({ ...value, to: formatDateOnly(date) });
  };

  return (
    <LocalizationProvider dateAdapter={AdapterDateFns}>
      <Grid size={size}>
        <DatePicker
          label={`${label} from (UTC)`}
          value={from}
          maxDate={fromMax ?? undefined}
          onChange={handleFrom}
          slotProps={{ textField: { size: "small", fullWidth: true }, field: { clearable: true } }}
        />
      </Grid>
      <Grid size={size}>
        <DatePicker
          label={`${label} to (UTC)`}
          value={to}
          minDate={from ?? undefined}
          maxDate={today ?? undefined}
          onChange={handleTo}
          slotProps={{ textField: { size: "small", fullWidth: true }, field: { clearable: true } }}
        />
      </Grid>
    </LocalizationProvider>
  );
}
