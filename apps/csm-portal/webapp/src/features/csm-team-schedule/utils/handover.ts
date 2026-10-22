/*
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

import type { ScheduleShift } from "../types";

/** Which office's day a CRE window belongs to. */
export type CoverSide = "lk" | "americas";

/** The zones the Sri Lankan day is worked on. */
const LK_DAY_ZONES = new Set(["Asia/Colombo", "Asia/Kolkata"]);

/**
 * Which side of the day a window covers.
 *
 * The Sri Lankan day is worked on the Colombo clock between 06:00 and 21:00:
 * the morning, regular hours, the India region shift, the evening and the
 * weekend rotation. Anything else is the Americas' -- a window that runs
 * through the Colombo night (Americas cover, the Americas weekend), or one
 * authored on another office's clock (Americas regular hours, Pacific time).
 * A card with no window of its own counts as the Sri Lankan day.
 */
export function coverSideOf(shift: ScheduleShift | undefined): CoverSide {
  if (!shift) return "lk";
  const zone = shift.authoringTimeZone || "Asia/Colombo";
  if (!LK_DAY_ZONES.has(zone)) return "americas";
  return shift.endMinute > 1260 ? "americas" : "lk";
}

/** One card of the day, placed on the reader's clock. */
export interface SidedSpan {
  startMin: number;
  endMin: number;
  side: CoverSide;
}

/** A stretch of the day where the Sri Lankan day and the Americas day are
 *  both at work. */
export interface HandoverBand {
  startMin: number;
  endMin: number;
}

function union(spans: readonly SidedSpan[]): Array<[number, number]> {
  const sorted = [...spans].sort((a, b) => a.startMin - b.startMin);
  const out: Array<[number, number]> = [];
  for (const s of sorted) {
    const last = out[out.length - 1];
    if (last && s.startMin <= last[1]) last[1] = Math.max(last[1], s.endMin);
    else out.push([s.startMin, s.endMin]);
  }
  return out;
}

/**
 * Where the Sri Lankan day and the Americas day overlap on the day being read.
 *
 * Worked out from the cards themselves, never from fixed hours, so it follows
 * whatever the windows say (Americas regular hours are 08:00-18:00 on a fixed
 * UTC-7 clock: 20:30-06:30 IST, a half hour at each end of the Sri Lankan
 * day). A reader looking at one team, or a day with nobody on the other side,
 * sees none. Where the two only touch -- one starting as the other ends --
 * nothing is marked.
 */
export function handoverBands(spans: readonly SidedSpan[]): HandoverBand[] {
  const lk = union(spans.filter((s) => s.side === "lk"));
  const americas = union(spans.filter((s) => s.side === "americas"));
  const bands: HandoverBand[] = [];
  for (const [ls, le] of lk) {
    for (const [as, ae] of americas) {
      const start = Math.max(ls, as);
      const end = Math.min(le, ae);
      if (end > start) bands.push({ startMin: start, endMin: end });
    }
  }
  return bands.sort((a, b) => a.startMin - b.startMin);
}

/** "20:30" for a minute of the reader's day. */
export function minuteLabel(m: number): string {
  const h = Math.floor(m / 60) % 24;
  return `${String(h).padStart(2, "0")}:${String(m % 60).padStart(2, "0")}`;
}

/** "30 min", "1 h 30 min". */
export function spanLength(minutes: number): string {
  const h = Math.floor(minutes / 60);
  const m = minutes % 60;
  if (h === 0) return `${m} min`;
  return m === 0 ? `${h} h` : `${h} h ${m} min`;
}
