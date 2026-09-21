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
  Briefcase,
  GitBranch,
  Megaphone,
  MessageSquare,
  Shield,
  Wrench,
} from "@wso2/oxygen-ui-icons-react";
import { SEVERITY_LEGEND_ORDER } from "@features/dashboard/constants/dashboard";
import { SeverityLegendKey } from "@features/dashboard/types/dashboard";
import type {
  PendingVerificationRecordType,
  PendingVerificationView,
} from "@features/support/types/pendingVerification";

// Matches each type's existing icon elsewhere in this app (Engagements/
// Security Center/Announcements nav items, the "Related Change Requests"
// tab) rather than inventing new ones — shared by the list page's type
// filter, the group headers, and each card's identifier line.
export const RECORD_TYPE_ICONS: Record<PendingVerificationRecordType, typeof MessageSquare> = {
  Case: MessageSquare,
  Engagement: Briefcase,
  "Security Report": Shield,
  "Service Request": Wrench,
  Announcement: Megaphone,
  "Change Request": GitBranch,
};

/**
 * Maps a pending-verification record's recordType to its own existing detail
 * page route — every one of these routes already exists in App.tsx today.
 * Uses workItemId (the real UUID), not caseId (the display-only record number).
 *
 * @param {string} projectId - The project id.
 * @param {PendingVerificationView} record - The pending-verification record.
 * @returns {string} The path to the record's own detail page.
 */
export function buildPendingVerificationRecordPath(
  projectId: string,
  record: PendingVerificationView,
): string {
  const base = `/projects/${projectId}`;
  switch (record.recordType) {
    case "Case":
      return `${base}/support/cases/${record.workItemId}`;
    case "Engagement":
      return `${base}/engagements/${record.workItemId}`;
    case "Security Report":
      return `${base}/security-center/security-report-analysis/${record.workItemId}`;
    case "Service Request":
      return `${base}/operations/service-requests/${record.workItemId}`;
    case "Announcement":
      return `${base}/announcements/${record.workItemId}`;
    case "Change Request":
      return `${base}/operations/change-requests/${record.workItemId}`;
    default:
      return `${base}/support/cases/${record.workItemId}`;
  }
}

// Bare severity code ("S1") → this app's own established severity legend key
// (SEVERITY_LEGEND_ORDER, src/features/dashboard/constants/dashboard.ts),
// positionally derived from that same real, existing order — S0=Catastrophic
// through S4=Low. Not a guessed mapping.
const SEVERITY_CODE_TO_LEGEND_KEY: Record<string, SeverityLegendKey> = {
  S0: SeverityLegendKey.Catastrophic,
  S1: SeverityLegendKey.Critical,
  S2: SeverityLegendKey.High,
  S3: SeverityLegendKey.Medium,
  S4: SeverityLegendKey.Low,
};

export interface SeverityDisplay {
  label: string;
  color: string;
}

/**
 * Formats a bare severity code (e.g. "S1") into this app's real display
 * label + legend color (e.g. "S1 — Critical", red) — reusing the same
 * SEVERITY_LEGEND_ORDER the dashboard's own severity legend/chips use,
 * rather than a separately-invented mapping.
 *
 * @param {string} [code] - Bare severity code from the API (e.g. "S1").
 * @returns {SeverityDisplay | null} Formatted label + color, or null if unrecognized.
 */
export function getSeverityDisplay(code?: string): SeverityDisplay | null {
  if (!code) return null;
  const key = SEVERITY_CODE_TO_LEGEND_KEY[code.trim().toUpperCase()];
  if (!key) return null;
  const entry = SEVERITY_LEGEND_ORDER.find((item) => item.key === key);
  if (!entry) return null;
  const word = entry.displayName.split(" - ")[1] ?? entry.label;
  return { label: `${code.trim().toUpperCase()} — ${word}`, color: entry.color };
}
