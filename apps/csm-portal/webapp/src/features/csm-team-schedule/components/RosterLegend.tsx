/**
 * Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { useState, type JSX } from "react";

export type LegendGroup = "Rota" | "Hours" | "Leave" | "Allocations";

export interface LegendItem {
  /** Unique within the legend: two windows can share a short code. */
  key: string;
  group: LegendGroup;
  code: string;
  /** The chip colour token, as on the grid. */
  token: string;
  label: string;
  /** Drawn dashed, as an on-call window is on the grid. */
  onCall?: boolean;
}

const ORDER: LegendGroup[] = ["Rota", "Hours", "Leave", "Allocations"];

/** Where the open/closed choice is kept, per browser. */
const STORE_KEY = "csm-ts.roster-legend.open";

function readOpen(): boolean {
  try {
    return window.localStorage.getItem(STORE_KEY) !== "0";
  } catch {
    return true;
  }
}

function writeOpen(open: boolean): void {
  try {
    window.localStorage.setItem(STORE_KEY, open ? "1" : "0");
  } catch {
    // A browser that keeps nothing simply opens the legend every time.
  }
}

/**
 * What the codes on the roster mean.
 *
 * The codes on screen, plus every leave, allocation and move tag a rota
 * marker can add, so a lead finds what can be marked without opening a cell.
 * Open by default -- the codes are the rota sheet's shorthand, which a new
 * lead or a manager does not know -- and a reader who knows them can fold it
 * away; the choice is remembered.
 */
export default function RosterLegend({ items }: { items: LegendItem[] }): JSX.Element | null {
  const [open, setOpen] = useState(readOpen);
  if (items.length === 0) return null;
  const groups = ORDER.map((g) => ({ group: g, items: items.filter((i) => i.group === g) })).filter(
    (g) => g.items.length > 0,
  );
  return (
    <details
      className="rlegend"
      open={open}
      onToggle={(e) => {
        const now = e.currentTarget.open;
        setOpen(now);
        writeOpen(now);
      }}
    >
      <summary>
        Legend <small>what the codes mean, and every tag that can be marked</small>
      </summary>
      <div className="rl-groups">
        {groups.map((g) => (
          <div className="rl-g" key={g.group}>
            <span className="rl-h">{g.group}</span>
            <ul>
              {g.items.map((it) => (
                <li key={it.key}>
                  <span className={`chip sm ${it.token}${it.onCall ? " oc" : ""}`}>{it.code}</span>
                  <span className="rl-l">{it.label}</span>
                </li>
              ))}
            </ul>
          </div>
        ))}
      </div>
    </details>
  );
}
