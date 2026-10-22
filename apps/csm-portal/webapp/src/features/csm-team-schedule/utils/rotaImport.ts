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

import type { RotaImportFile } from "../api/useImportRota";

/** The standard structure, offered as a template to start from. */
export const ROTA_IMPORT_TEMPLATE = {
  schemaVersion: 1,
  family: "SRE",
  rotaCode: "SRE_SAAS",
  period: { from: "2026-11-01", to: "2026-11-30" },
  // Merge, the mode that removes nothing: the template is what people edit and send back.
  mode: "merge",
  assignments: [
    { email: "user1@example.com", teamKey: "apollo", shiftCode: "SRE_TZ1_L1", from: "2026-11-02", to: "2026-11-06" },
    { email: "user2@example.com", teamKey: "apollo", shiftCode: "SRE_TZ1", tier: "L2", from: "2026-11-02", to: "2026-11-06" },
    { email: "user3@example.com", teamKey: "artemis", shiftCode: "SRE_TZ3", tier: "L1", from: "2026-11-02", to: "2026-11-08" },
    { email: "user4@example.com", teamKey: "apollo", shiftCode: "SRE_WE_TZ1", tier: "L1", from: "2026-11-07", to: "2026-11-08" },
  ],
  absences: [
    { email: "user2@example.com", teamKey: "apollo", kindCode: "ANNUAL_LEAVE", from: "2026-11-16", to: "2026-11-20" },
  ],
};

export type ParsedRotaImport = { file: RotaImportFile; error?: undefined } | { file?: undefined; error: string };

/** Reads an import file's text and checks the shape the server needs before anything is sent, and that it is the rota on screen. */
export function parseRotaImportFile(text: string, rotaCode: string): ParsedRotaImport {
  let data: unknown;
  try {
    data = JSON.parse(text);
  } catch {
    return { error: "The file is not valid JSON." };
  }
  if (typeof data !== "object" || data === null || Array.isArray(data)) {
    return { error: "The file must hold one JSON object." };
  }
  const f = data as Record<string, unknown>;
  const period = f.period as Record<string, unknown> | undefined;
  if (f.schemaVersion !== 1) return { error: "schemaVersion must be 1." };
  if (typeof f.rotaCode !== "string" || f.rotaCode.trim() === "") return { error: "rotaCode is missing." };
  if (f.rotaCode.trim().toUpperCase() !== rotaCode.toUpperCase()) {
    return { error: `This file is for ${f.rotaCode}, but the roster shows ${rotaCode}. Switch the rota first.` };
  }
  if (typeof period !== "object" || period === null || typeof period.from !== "string" || typeof period.to !== "string") {
    return { error: "period must have from and to dates (YYYY-MM-DD)." };
  }
  if (f.mode !== "replace" && f.mode !== "merge") return { error: 'mode must be "replace" or "merge".' };
  for (const key of ["assignments", "absences"] as const) {
    if (f[key] !== undefined && !Array.isArray(f[key])) return { error: `${key} must be a list.` };
  }
  const entries = ((f.assignments as unknown[]) ?? []).length + ((f.absences as unknown[]) ?? []).length;
  if (entries === 0) return { error: "The file has no assignments and no absences." };
  return { file: data as RotaImportFile };
}

/** Whether a reader may be offered "Import rota" for a rota: an SRE rota with one of its teams in the teams they may edit; the server checks every entry again. */
export function rotaImportOffered(
  rotaCode: string | undefined,
  rotas: readonly { code: string; family: string }[],
  teams: readonly { key: string; rotaCode?: string | null }[],
  editableTeams: readonly string[],
): boolean {
  if (!rotaCode || !rotas.some((r) => r.code === rotaCode && r.family === "SRE")) return false;
  const editable = new Set(editableTeams.map((k) => k.toLowerCase()));
  return teams.some((t) => t.rotaCode === rotaCode && editable.has(t.key.toLowerCase()));
}
