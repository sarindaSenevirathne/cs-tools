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

import { Box, Button, Chip, FormControl, IconButton, InputLabel, MenuItem, Select, Typography } from "@wso2/oxygen-ui";
import { Plus, Trash2 } from "@wso2/oxygen-ui-icons-react";
import { Fragment, useState, type JSX, type ReactNode } from "react";
import { completeAnyOfGroups, type AnyOfGroup } from "@features/csm-operations/utils/operationsAnyOf";

/** The groups as the search (and the URL) sees them: complete conditions only. */
const completeKey = (groups: AnyOfGroup[]): string => JSON.stringify(completeAnyOfGroups(groups));

/** A field an OR group condition can use ("is one of"). */
export interface AnyOfFieldDef {
  /** The search field it is sent as (e.g. `assignmentGroupId`). */
  field: string;
  label: string;
  render: (props: { id: string; values: string[]; onChange: (values: string[]) => void }) => ReactNode;
}

interface OperationsAnyOfGroupsProps {
  idPrefix: string;
  fields: AnyOfFieldDef[];
  groups: AnyOfGroup[];
  onChange: (next: AnyOfGroup[]) => void;
}

/**
 * The OR groups under the Advanced filters, as on the service request list
 * (`AnyOfGroupsBuilder`): the conditions in a group must all match, any one
 * group may match, and the result is ANDed with every other filter.
 *
 * The groups being edited live here, not only in `groups`: the list keeps its
 * filters in the URL, which holds complete conditions only, so a group just
 * added (no values yet), a condition just added or one whose field was just
 * changed would vanish on the round trip. Every edit is still forwarded to
 * `onChange`; the local copy is replaced from `groups` only when the complete
 * form coming in differs from the one held here (Clear filters, a saved view,
 * Back) -- the same idea as the Advanced rows' drafts.
 */
export default function OperationsAnyOfGroups({
  idPrefix,
  fields,
  groups,
  onChange,
}: OperationsAnyOfGroupsProps): JSX.Element {
  const [local, setLocal] = useState<AnyOfGroup[]>(groups);
  const [seen, setSeen] = useState(() => completeKey(groups));
  const incoming = completeKey(groups);
  if (incoming !== seen) {
    setSeen(incoming);
    if (incoming !== completeKey(local)) setLocal(groups);
  }
  const update = (next: AnyOfGroup[]): void => {
    setLocal(next);
    onChange(next);
  };
  const byField = new Map(fields.map((f) => [f.field, f]));
  const blankCondition = () => ({ field: fields[0].field, values: [] as string[] });

  const setGroup = (gi: number, next: AnyOfGroup | null): void =>
    update(next === null ? local.filter((_, i) => i !== gi) : local.map((g, i) => (i === gi ? next : g)));

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5 }}>
      <Typography variant="subtitle2" color="text.secondary">
        OR groups
      </Typography>
      <Typography variant="caption" color="text.secondary">
        Matches if any one group matches. Every condition inside a group must match.
      </Typography>
      {local.map((group, gi) => (
        <Fragment key={gi}>
          {gi > 0 && (
            <Box>
              <Chip size="small" label="OR" />
            </Box>
          )}
          <Box
            role="group"
            aria-label={`OR group ${gi + 1}`}
            sx={{ border: 1, borderColor: "divider", borderRadius: 1, p: 1.5, display: "flex", flexDirection: "column", gap: 1 }}
          >
            <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
              <Typography variant="caption" color="text.secondary">
                Group {gi + 1}
              </Typography>
              <IconButton size="small" aria-label={`Remove OR group ${gi + 1}`} onClick={() => setGroup(gi, null)}>
                <Trash2 size={16} />
              </IconButton>
            </Box>
            {group.conditions.map((c, ci) => {
              const def = byField.get(c.field) ?? fields[0];
              const rowId = `${idPrefix}-anyof-${gi}-${ci}`;
              const setCondition = (next: { field: string; values: string[] } | null): void =>
                setGroup(gi, {
                  conditions:
                    next === null
                      ? group.conditions.filter((_, i) => i !== ci)
                      : group.conditions.map((x, i) => (i === ci ? next : x)),
                });
              return (
                <Box key={ci} sx={{ display: "flex", gap: 1, alignItems: "flex-start", flexWrap: "wrap" }}>
                  {ci > 0 && (
                    <Typography variant="caption" color="text.secondary" sx={{ lineHeight: "40px", minWidth: 28 }}>
                      AND
                    </Typography>
                  )}
                  <FormControl size="small" sx={{ minWidth: 200 }}>
                    <InputLabel id={`${rowId}-field-label`}>Field</InputLabel>
                    <Select
                      labelId={`${rowId}-field-label`}
                      label="Field"
                      value={def.field}
                      onChange={(e) => setCondition({ field: String(e.target.value), values: [] })}
                    >
                      {fields.map((f) => (
                        <MenuItem key={f.field} value={f.field}>
                          {f.label}
                        </MenuItem>
                      ))}
                    </Select>
                  </FormControl>
                  <Typography variant="body2" color="text.secondary" sx={{ lineHeight: "40px" }}>
                    is one of
                  </Typography>
                  <Box sx={{ flex: "1 1 240px", minWidth: 240 }}>
                    {def.render({
                      id: `${rowId}-value`,
                      values: c.values,
                      onChange: (values) => setCondition({ field: def.field, values }),
                    })}
                  </Box>
                  <IconButton size="small" aria-label="Remove condition" onClick={() => setCondition(null)} sx={{ mt: 0.5 }}>
                    <Trash2 size={16} />
                  </IconButton>
                </Box>
              );
            })}
            <Box>
              <Button
                size="small"
                startIcon={<Plus size={16} />}
                onClick={() => setGroup(gi, { conditions: [...group.conditions, blankCondition()] })}
              >
                Add condition
              </Button>
            </Box>
          </Box>
        </Fragment>
      ))}
      <Box>
        <Button
          size="small"
          variant="outlined"
          startIcon={<Plus size={16} />}
          onClick={() => update([...local, { conditions: [blankCondition()] }])}
        >
          Add group
        </Button>
      </Box>
    </Box>
  );
}
