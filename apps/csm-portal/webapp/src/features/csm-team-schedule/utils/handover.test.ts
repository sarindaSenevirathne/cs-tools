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

import { describe, expect, it } from "vitest";
import type { ScheduleShift } from "../types";
import { coverSideOf, handoverBands, minuteLabel, spanLength, type SidedSpan } from "./handover";

const win = (startMinute: number, endMinute: number, authoringTimeZone = "Asia/Colombo"): ScheduleShift =>
  ({ code: "X", startMinute, endMinute, authoringTimeZone }) as ScheduleShift;

const h = (hh: number, mm = 0): number => hh * 60 + mm;
const lk = (s: number, e: number): SidedSpan => ({ startMin: s, endMin: e, side: "lk" });
const us = (s: number, e: number): SidedSpan => ({ startMin: s, endMin: e, side: "americas" });

describe("coverSideOf", () => {
  it("puts the Colombo day on the Sri Lankan side", () => {
    expect(coverSideOf(win(360, 540))).toBe("lk");
    expect(coverSideOf(win(540, 1080))).toBe("lk");
    expect(coverSideOf(win(540, 1080, "Asia/Kolkata"))).toBe("lk");
    expect(coverSideOf(win(1080, 1260))).toBe("lk");
    expect(coverSideOf(win(360, 1260))).toBe("lk");
  });

  it("puts the Colombo night and another office's clock on the Americas side", () => {
    expect(coverSideOf(win(1260, 1800))).toBe("americas");
    expect(coverSideOf(win(480, 1080, "Etc/GMT+7"))).toBe("americas");
  });

  it("counts a card with no window as the Sri Lankan day", () => {
    expect(coverSideOf(undefined)).toBe("lk");
  });
});

describe("handoverBands", () => {
  // A weekday read in IST: the Americas day (08:00-18:00 UTC-7) is 20:30-06:30,
  // so yesterday's runs in until 06:30.
  const weekday = [
    us(0, h(6, 30)),
    lk(h(6), h(9)),
    lk(h(9), h(18)),
    lk(h(18), h(21)),
    us(h(20, 30), 1440),
  ];

  it("finds both half-hour handovers", () => {
    expect(handoverBands(weekday)).toEqual([
      { startMin: h(6), endMin: h(6, 30) },
      { startMin: h(20, 30), endMin: h(21) },
    ]);
  });

  it("marks nothing where one day starts as the other ends", () => {
    expect(handoverBands([us(0, h(6)), lk(h(6), h(21)), us(h(21), 1440)])).toEqual([]);
  });

  it("marks nothing for a hole between the two days", () => {
    expect(handoverBands([lk(h(6), h(21)), us(h(21, 30), 1440)])).toEqual([]);
  });

  it("marks nothing when only one side is on screen", () => {
    expect(handoverBands([lk(h(6), h(21))])).toEqual([]);
    expect(handoverBands([us(0, h(6, 30)), us(h(20, 30), 1440)])).toEqual([]);
  });

  it("joins cards on one side that overlap each other before comparing", () => {
    expect(handoverBands([lk(h(9), h(18)), lk(h(17), h(21)), us(h(20), h(23))])).toEqual([
      { startMin: h(20), endMin: h(21) },
    ]);
  });
});

describe("labels", () => {
  it("formats minutes and lengths", () => {
    expect(minuteLabel(h(20, 30))).toBe("20:30");
    expect(minuteLabel(1440)).toBe("00:00");
    expect(spanLength(30)).toBe("30 min");
    expect(spanLength(90)).toBe("1 h 30 min");
    expect(spanLength(120)).toBe("2 h");
  });
});
