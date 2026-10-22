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
  Box,
  Button,
  FormControl,
  IconButton,
  InputLabel,
  MenuItem,
  Select,
  Typography,
} from "@wso2/oxygen-ui";
import { Plus, Trash2 } from "@wso2/oxygen-ui-icons-react";
import { useState, type JSX } from "react";
import type { OperationsAdvancedField } from "@features/csm-operations/utils/operationsAdvancedFields";

interface Row {
  key: string;
  field: string;
  op: string;
  values: string[];
  draft: boolean;
}

interface OperationsAdvancedFiltersProps<F> {
  idPrefix: string;
  fields: OperationsAdvancedField<F>[];
  filters: F;
  onChange: (next: F) => void;
}

/**
 * The change request / incident lists' "Advanced" filters: Field / Operator /
 * Value rows with an "Add filter" button, the same pattern as the service
 * request list's builder (`AdvancedFiltersBuilder`). A row only lives in local
 * state while it has no value yet; once it has one it is written to the
 * filter state and read back from it.
 */
export default function OperationsAdvancedFilters<F>({
  idPrefix,
  fields,
  filters,
  onChange,
}: OperationsAdvancedFiltersProps<F>): JSX.Element {
  const [drafts, setDrafts] = useState<Array<{ id: number; field: string; op: string }>>([]);
  const [nextId, setNextId] = useState(1);
  const byKey = new Map(fields.map((f) => [f.key, f]));

  const committed: Row[] = fields.flatMap((f) =>
    f
      .read(filters)
      .filter((r) => r.values.length > 0)
      .map((r) => ({ key: `${f.key}:${r.op}`, field: f.key, op: r.op, values: r.values, draft: false })),
  );
  const rows: Row[] = [
    ...committed,
    ...drafts.map((d) => ({ key: `draft:${d.id}`, field: d.field, op: d.op, values: [], draft: true })),
  ];

  const dropDraft = (row: Row): void =>
    setDrafts((ds) => ds.filter((d) => `draft:${d.id}` !== row.key));

  const clear = (row: Row, f: F): F => (row.draft ? f : byKey.get(row.field)!.write(f, row.op, []));

  const changeField = (row: Row, fieldKey: string): void => {
    const field = byKey.get(fieldKey)!;
    if (!row.draft) onChange(clear(row, filters));
    else dropDraft(row);
    setDrafts((ds) => [...ds, { id: nextId, field: fieldKey, op: field.ops[0].op }]);
    setNextId((n) => n + 1);
  };

  const changeOp = (row: Row, op: string): void => {
    if (row.draft) {
      setDrafts((ds) => ds.map((d) => (`draft:${d.id}` === row.key ? { ...d, op } : d)));
      return;
    }
    const field = byKey.get(row.field)!;
    onChange(field.write(field.write(filters, row.op, []), op, row.values));
  };

  const changeValues = (row: Row, values: string[]): void => {
    const field = byKey.get(row.field)!;
    if (row.draft && values.length > 0) dropDraft(row);
    if (!row.draft || values.length > 0) onChange(field.write(filters, row.op, values));
  };

  const remove = (row: Row): void => {
    if (row.draft) dropDraft(row);
    else onChange(clear(row, filters));
  };

  const add = (): void => {
    const first = fields[0];
    setDrafts((ds) => [...ds, { id: nextId, field: first.key, op: first.ops[0].op }]);
    setNextId((n) => n + 1);
  };

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
      <Typography variant="subtitle2">Advanced filters</Typography>
      {rows.length === 0 && (
        <Typography variant="body2" color="text.secondary">
          No advanced filters. Add one to narrow the list further.
        </Typography>
      )}
      {rows.map((row, index) => {
        const field = byKey.get(row.field)!;
        const rowId = `${idPrefix}-advanced-${index}`;
        return (
          <Box key={row.key} sx={{ display: "flex", gap: 1, alignItems: "flex-start", flexWrap: "wrap" }}>
            <FormControl size="small" sx={{ minWidth: 200 }}>
              <InputLabel id={`${rowId}-field-label`}>Field</InputLabel>
              <Select
                labelId={`${rowId}-field-label`}
                label="Field"
                value={row.field}
                onChange={(e) => changeField(row, String(e.target.value))}
              >
                {fields.map((f) => (
                  <MenuItem key={f.key} value={f.key}>
                    {f.label}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
            <FormControl size="small" sx={{ minWidth: 160 }}>
              <InputLabel id={`${rowId}-op-label`}>Operator</InputLabel>
              <Select
                labelId={`${rowId}-op-label`}
                label="Operator"
                value={row.op}
                onChange={(e) => changeOp(row, String(e.target.value))}
              >
                {field.ops.map((o) => (
                  <MenuItem key={o.op} value={o.op}>
                    {o.label}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
            <Box sx={{ flex: "1 1 240px", minWidth: 240 }}>
              {field.renderValue({
                id: `${rowId}-value`,
                op: row.op,
                values: row.values,
                onChange: (values) => changeValues(row, values),
              })}
            </Box>
            <IconButton size="small" aria-label="Remove filter row" onClick={() => remove(row)} sx={{ mt: 0.5 }}>
              <Trash2 size={16} />
            </IconButton>
          </Box>
        );
      })}
      <Box>
        <Button size="small" variant="outlined" startIcon={<Plus size={16} />} onClick={add}>
          Add filter
        </Button>
      </Box>
    </Box>
  );
}
