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

import { describe, expect, it } from "vitest";
import type { CaseAlertSettings } from "../types";
import {
  LADDER_KEYS,
  draftFor,
  fileOnlyRows,
  formatLadderValue,
  ladderKeysFor,
  parseDraft,
  runningLine,
  valuesToCopy,
  type LadderKeySpec,
} from "./caseAlertLadderKeys";

function spec(key: string): LadderKeySpec {
  const s = LADDER_KEYS.find((k) => k.key === key);
  if (!s) throw new Error(`no key ${key}`);
  return s;
}

const draft = { text: "", checked: false, name: "", email: "" };

describe("caseAlertLadderKeys", () => {
  it("lists each key once, on the ladders the design gives it", () => {
    expect(new Set(LADDER_KEYS.map((k) => k.key)).size).toBe(LADDER_KEYS.length);
    expect(ladderKeysFor("CRE")).toHaveLength(27);
    expect(ladderKeysFor("SRE")).toHaveLength(19);
    expect(ladderKeysFor("SRE").map((k) => k.key)).not.toContain("heads.cre");
    expect(ladderKeysFor("CRE").map((k) => k.key)).not.toContain("timing.interval");
  });

  it("writes values for people", () => {
    expect(formatLadderValue(spec("teams.abts"), ["atlas", "castor"])).toBe("atlas, castor");
    expect(formatLadderValue(spec("trigger.priorities"), [])).toBe("any");
    expect(formatLadderValue(spec("trigger.excludeTeams"), [])).toBe("none");
    expect(formatLadderValue(spec("trigger.requireKnownTeam"), false)).toBe("No");
    expect(formatLadderValue(spec("alertDuty.perTeam"), 0)).toBe("0 (all of them)");
    expect(formatLadderValue(spec("teams.unassignedTeamLead"), "pool")).toBe("A lead from the pool");
    expect(formatLadderValue(spec("teams.rotaMembersToCall"), { LK_EVENING: 7 })).toBe("LK_EVENING=7");
    expect(formatLadderValue(spec("teams.rotaMembersToCall"), {})).toBe("no cap");
    expect(formatLadderValue(spec("heads.cre"), { name: "N", email: "n@x", phone: "+94770000000" })).toBe("N <n@x>");
    expect(formatLadderValue(spec("timing.interval"), undefined)).toBe("—");
  });

  it("never shows a link", () => {
    expect(formatLadderValue(spec("teams.leadership"), "https://chat.googleapis.com/v1/x?token=t")).toBe(
      "a link (not shown)",
    );
    expect(formatLadderValue(undefined, { url: "https://chat.googleapis.com/v1/x" })).not.toContain("googleapis");
  });

  it("reads each editor back into the value's own shape", () => {
    expect(parseDraft(spec("teams.abts"), { ...draft, text: " atlas,, castor " })).toEqual({
      ok: true,
      value: ["atlas", "castor"],
    });
    expect(parseDraft(spec("trigger.teams"), draft)).toEqual({ ok: true, value: [] });
    expect(parseDraft(spec("teams.rotaMembersToCall"), { ...draft, text: "lk_evening=7, USA = 2" })).toEqual({
      ok: true,
      value: { LK_EVENING: 7, USA: 2 },
    });
    expect(parseDraft(spec("teams.rotaMembersToCall"), { ...draft, text: "LK seven" }).ok).toBe(false);
    expect(parseDraft(spec("teams.teamLeadsToCall"), { ...draft, text: "3" })).toEqual({ ok: true, value: 3 });
    expect(parseDraft(spec("teams.teamLeadsToCall"), { ...draft, text: "three" }).ok).toBe(false);
    expect(parseDraft(spec("heads.cs"), { ...draft, name: "N" }).ok).toBe(false);
    expect(parseDraft(spec("timing.interval"), { ...draft, text: " 5m " })).toEqual({ ok: true, value: "5m" });
    expect(draftFor(spec("teams.rotaMembersToCall"), { LK: 2, USA: 1 }).text).toBe("LK=2, USA=1");
    expect(draftFor(spec("onLeave"), undefined).text).toBe("skip");
  });

  it("words the file-only settings, and leaves the heads row to CRE", () => {
    const f = { channel: "chat", "safety.maxCallsPerLadder": 4, "safety.allowedNumbers": 1, "phones.source": "x" };
    const rows = Object.fromEntries(fileOnlyRows("SRE", f).map((r) => [r.label, r.value]));
    expect(rows).toMatchObject({
      Channel: "Chat only",
      "Max calls per ladder": "4",
      "Highest step": "no cap",
      "Allowed numbers": "only 1 listed number",
      "Alerts on": "—",
      "Phone numbers from": "x",
    });
    expect(rows).not.toHaveProperty("Call the heads when nobody below can be called");
    expect(fileOnlyRows("CRE", f).map((r) => r.label)).toContain("Call the heads when nobody below can be called");
  });

  it("leaves out of the file-only block what the tab now edits", () => {
    const f = { enabled: true, channel: "both", "safety.maxCallsPerLadder": 4, "safety.allowedNumbers": 0 };
    const onTab = new Set(["enabled", "channel", "safety.maxCallsPerLadder", "safety.callWithoutVerifiedLeads"]);
    const labels = fileOnlyRows("SRE", f, onTab).map((r) => r.label);
    expect(labels).toEqual(["Highest step", "Allowed numbers", "Phone numbers from"]);
    // An alert service that does not report them on the tab yet: all still shown here.
    expect(fileOnlyRows("SRE", f).map((r) => r.label)).toContain("Alerts on");
  });

  it("puts each family's on/off and channel on the tab, and the safety holds on CRE and SRE only", () => {
    const keys = (l: "CRE" | "SRE" | "SME") =>
      ladderKeysFor(l)
        .filter((k) => k.group === "On/off and channel")
        .map((k) => k.key);
    expect(keys("SME")).toEqual(["enabled", "channel"]);
    expect(keys("CRE")).toEqual([
      "enabled",
      "channel",
      "safety.maxCallsPerLadder",
      "safety.callWithoutVerifiedLeads",
      "safety.callHeadsWithoutLowerTiers",
    ]);
    expect(keys("SRE")).toEqual(keys("CRE"));
    expect(parseDraft(spec("channel"), { ...draft, text: "chat" })).toEqual({ ok: true, value: "chat" });
    expect(parseDraft(spec("safety.maxCallsPerLadder"), { ...draft, text: "4" })).toEqual({ ok: true, value: 4 });
    expect(formatLadderValue(spec("channel"), "both")).toBe("Calls and chat");
  });

  it("copies the file's named heads without a phone, and skips an unnamed one", () => {
    const s = {
      values: [],
      running: {
        reportedAt: "2026-10-13T10:00:00Z",
        mode: "shadow",
        ladders: {
          CRE: {
            "heads.cre": { value: { name: "N", email: "n@x", phone: "+94770000000" }, source: "file" },
            "heads.cs": { value: { name: "", email: "" }, source: "file" },
            "teams.americas": { value: null, source: "file" },
          },
        },
      },
    } as unknown as CaseAlertSettings;
    expect(valuesToCopy(s, "CRE")).toEqual({ "heads.cre": { name: "N", email: "n@x" } });
    expect(valuesToCopy(s, "SRE")).toEqual({});
  });
});

describe("runningLine", () => {
  const settings = (running: CaseAlertSettings["running"]) => ({ running }) as CaseAlertSettings;

  it("says when the alert service reported, and in which mode", () => {
    const line = runningLine(settings({ reportedAt: "2026-10-13T10:00:00Z", mode: "db", ladders: {} }));
    expect(line).toMatch(/^"In use" is what the alert service reported at .+, in db mode\.$/);
    expect(line).not.toContain("Invalid Date");
  });

  it("leaves the time out when the report has none", () => {
    expect(runningLine(settings({ mode: "shadow", ladders: {} }))).toBe(
      '"In use" is what the alert service reported, in shadow mode. In shadow mode it still uses its file; saved values take over in db mode.',
    );
  });

  it("says it hasn't reported yet when there is no report", () => {
    expect(runningLine(settings(undefined))).toMatch(/hasn't reported what it uses yet/);
  });
});
