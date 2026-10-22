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

import { BackendApiError } from "@api/backend/client";

/** One record a bulk action is applied to. */
export interface BulkTarget {
  id: string;
  /** The record's number, for the summary ("INC0012 — refused: ..."). */
  number?: string | null;
}

/** A record the action could not be applied to, with the reason in words. */
export interface BulkFailure {
  id: string;
  number: string;
  message: string;
}

export interface BulkResult {
  succeeded: string[];
  failed: BulkFailure[];
  /** Records left out before anything was sent, with the reason (e.g. a field an approved change request no longer takes). */
  skipped: BulkFailure[];
}

/** How many records are updated at once: enough to be quick, few enough not to flood the backend. */
export const BULK_CONCURRENCY = 4;

/**
 * The reason a single update failed, in words the engineer can act on. A
 * refusal (4xx) carries the backend's own message, which says what is wrong
 * with that record; anything else is a generic line (a 5xx message is not for
 * people).
 */
export function bulkFailureMessage(err: unknown): string {
  if (err instanceof BackendApiError && err.status >= 400 && err.status < 500 && err.message.trim()) {
    return err.message;
  }
  return "Could not update this record. Try again.";
}

/**
 * `fn` over every item, at most `concurrency` at a time, results in the items'
 * order. A rejection is kept as `undefined` for that item (the caller decides
 * what a missing result means); it never stops the others.
 */
export async function mapWithConcurrency<I, R>(
  items: I[],
  fn: (item: I) => Promise<R>,
  concurrency = BULK_CONCURRENCY,
): Promise<Array<R | undefined>> {
  const results: Array<R | undefined> = new Array(items.length);
  let next = 0;
  const worker = async (): Promise<void> => {
    for (;;) {
      const i = next++;
      if (i >= items.length) return;
      try {
        results[i] = await fn(items[i]);
      } catch {
        results[i] = undefined;
      }
    }
  };
  await Promise.all(Array.from({ length: Math.max(1, Math.min(concurrency, items.length)) }, worker));
  return results;
}

/**
 * Applies `update` to every target, at most {@link BULK_CONCURRENCY} at a time,
 * and reports each outcome: there is no bulk endpoint, so this is one request
 * per record, and one refusal never stops the others (the same model as the
 * time cards' bulk approve). The order of `succeeded` / `failed` follows the
 * order of `targets`.
 */
export async function runBulkUpdate(
  targets: BulkTarget[],
  update: (target: BulkTarget) => Promise<unknown>,
  concurrency = BULK_CONCURRENCY,
): Promise<Omit<BulkResult, "skipped">> {
  const outcomes: Array<{ ok: true } | { ok: false; message: string }> = new Array(targets.length);
  let next = 0;
  const worker = async (): Promise<void> => {
    for (;;) {
      const i = next++;
      if (i >= targets.length) return;
      try {
        await update(targets[i]);
        outcomes[i] = { ok: true };
      } catch (err) {
        outcomes[i] = { ok: false, message: bulkFailureMessage(err) };
      }
    }
  };
  await Promise.all(Array.from({ length: Math.max(1, Math.min(concurrency, targets.length)) }, worker));
  const succeeded: string[] = [];
  const failed: BulkFailure[] = [];
  targets.forEach((t, i) => {
    const outcome = outcomes[i];
    if (outcome?.ok) succeeded.push(t.id);
    else failed.push({ id: t.id, number: t.number || t.id, message: outcome && !outcome.ok ? outcome.message : "Not sent." });
  });
  return { succeeded, failed };
}

/** One line for the banner: "Updated 4 of 5 incidents. 1 could not be updated (see below)." */
export function bulkResultSummary(result: BulkResult, nounPlural: string, verb = "Updated"): string {
  const attempted = result.succeeded.length + result.failed.length;
  const parts = [`${verb} ${result.succeeded.length} of ${attempted + result.skipped.length} ${nounPlural}.`];
  if (result.failed.length > 0) parts.push(`${result.failed.length} could not be updated.`);
  if (result.skipped.length > 0) parts.push(`${result.skipped.length} skipped.`);
  return parts.join(" ");
}
