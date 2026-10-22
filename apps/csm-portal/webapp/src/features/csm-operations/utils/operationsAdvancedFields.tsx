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


import { AdapterDateFns, DatePickers } from "@wso2/oxygen-ui";
import type { ReactNode } from "react";
import { formatDateOnly, parseDateOnly, todayUTCDateOnly } from "@features/csm-operations/utils/dateOnly";

const { DatePicker, LocalizationProvider } = DatePickers;

/** One field the Advanced builder offers. A field reads its rows from, and
 * writes them back to, the list's own filter state, so the quick bar, the URL
 * and saved views keep a single source of truth. */
export interface OperationsAdvancedField<F> {
  key: string;
  label: string;
  ops: Array<{ op: string; label: string }>;
  /** The rows this field contributes now, one per op that holds a value. */
  read: (filters: F) => Array<{ op: string; values: string[] }>;
  /** Writes one op's values; an empty list clears it. */
  write: (filters: F, op: string, values: string[]) => F;
  renderValue: (props: {
    id: string;
    op: string;
    values: string[];
    onChange: (values: string[]) => void;
  }) => ReactNode;
}

/** How many Advanced rows hold a value (for the "Advanced (n)" toggle). */
export function countAdvancedRows<F>(fields: OperationsAdvancedField<F>[], filters: F): number {
  return fields.reduce((n, f) => n + f.read(filters).filter((r) => r.values.length > 0).length, 0);
}

/**
 * An Advanced field for a date the list keeps as a from/to pair of
 * `YYYY-MM-DD` (UTC) values: "on or after" writes the from, "on or before"
 * the to. With `capAtToday` neither can pass today (created / updated dates).
 */
export function dateAdvancedField<F>(opts: {
  key: string;
  label: string;
  from: keyof F & string;
  to: keyof F & string;
  capAtToday?: boolean;
}): OperationsAdvancedField<F> {
  const { key, label, from, to, capAtToday = true } = opts;
  return {
    key,
    label,
    ops: [
      { op: "gte", label: "on or after" },
      { op: "lte", label: "on or before" },
    ],
    read: (f) => [
      { op: "gte", values: f[from] ? [String(f[from])] : [] },
      { op: "lte", values: f[to] ? [String(f[to])] : [] },
    ],
    write: (f, op, values) => {
      const prop = op === "gte" ? from : to;
      const next = { ...f, [prop]: values[0] ?? "" } as F;
      // An end before the start can only match nothing: drop the other bound.
      const a = String(next[from] ?? "");
      const b = String(next[to] ?? "");
      if (a && b && a > b) return { ...next, [op === "gte" ? to : from]: "" } as F;
      return next;
    },
    renderValue: ({ values, onChange }) => {
      const today = capAtToday ? todayUTCDateOnly() : null;
      return (
        <LocalizationProvider dateAdapter={AdapterDateFns}>
          <DatePicker
            label={`${label} (UTC)`}
            value={values[0] ? parseDateOnly(values[0]) : null}
            maxDate={today ?? undefined}
            onChange={(date: unknown) => {
              if (!(date instanceof Date) || Number.isNaN(date.getTime())) return onChange([]);
              if (today && date > today) return;
              onChange([formatDateOnly(date)]);
            }}
            slotProps={{ textField: { size: "small", fullWidth: true }, field: { clearable: true } }}
          />
        </LocalizationProvider>
      );
    },
  };
}

/** An Advanced field for a multi-valued filter kept as one array on the
 * list's state ("is one of"). */
export function multiAdvancedField<F>(opts: {
  key: string;
  label: string;
  prop: keyof F & string;
  render: (props: { id: string; values: string[]; onChange: (values: string[]) => void }) => ReactNode;
}): OperationsAdvancedField<F> {
  const { key, label, prop, render } = opts;
  return {
    key,
    label,
    ops: [{ op: "in", label: "is one of" }],
    read: (f) => [{ op: "in", values: (f[prop] as unknown as string[]) ?? [] }],
    write: (f, _op, values) => ({ ...f, [prop]: values }) as F,
    renderValue: ({ id, values, onChange }) => render({ id, values, onChange }),
  };
}

/** An Advanced field for a Yes-only flag (e.g. "SLA violated"): the filter is
 * either on or absent, so the only value offered is Yes. */
export function yesAdvancedField<F>(opts: {
  key: string;
  label: string;
  prop: keyof F & string;
  render: (props: { id: string; values: string[]; onChange: (values: string[]) => void }) => ReactNode;
}): OperationsAdvancedField<F> {
  const { key, label, prop, render } = opts;
  return {
    key,
    label,
    ops: [{ op: "eq", label: "is" }],
    read: (f) => [{ op: "eq", values: f[prop] ? ["true"] : [] }],
    write: (f, _op, values) => ({ ...f, [prop]: values[0] === "true" }) as F,
    renderValue: ({ id, values, onChange }) => render({ id, values, onChange }),
  };
}
