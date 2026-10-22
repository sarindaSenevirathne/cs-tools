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

import { useState, type JSX, type ReactNode } from "react";

/** Where the sections a person opened are kept, for this browser. */
const OPEN_KEY = "csm-ts.alert-settings.open";
/** And the ones that start open and were closed. */
const CLOSED_KEY = "csm-ts.alert-settings.closed";

function readIds(key: string): Set<string> {
  try {
    const raw = window.localStorage.getItem(key);
    const ids: unknown = raw ? JSON.parse(raw) : [];
    return new Set(Array.isArray(ids) ? ids.filter((x): x is string => typeof x === "string") : []);
  } catch {
    return new Set();
  }
}

function startsOpen(id: string, defaultOpen: boolean): boolean {
  return defaultOpen ? !readIds(CLOSED_KEY).has(id) : readIds(OPEN_KEY).has(id);
}

function remember(id: string, open: boolean, defaultOpen: boolean): void {
  // A section that starts open remembers being closed; the others, being opened.
  const key = defaultOpen ? CLOSED_KEY : OPEN_KEY;
  const keep = defaultOpen ? !open : open;
  try {
    const ids = readIds(key);
    if (keep) ids.add(id);
    else ids.delete(id);
    window.localStorage.setItem(key, JSON.stringify([...ids]));
  } catch {
    // Not kept: it stays as it is until the page is left.
  }
}

interface Props {
  /** Names the section in what is remembered, e.g. "SRE.where". */
  id: string;
  /** The region's name, for assistive tech and tests. */
  label: string;
  /** The one line shown while it is closed: enough to know what's inside. */
  summary: ReactNode;
  /** Starts open until the person closes it. */
  defaultOpen?: boolean;
  children: ReactNode;
}

/**
 * A section of the Alert Settings that starts closed (or open, with
 * defaultOpen), so the page shows a few one-line summaries instead of
 * everything at once. The summary is a native
 * <summary>, so it takes focus and opens with Enter or Space. Which sections
 * a person opened or closed is kept for this browser; storage that is blocked or throws
 * only means it starts closed next time. What's inside is drawn only while it
 * is open.
 */
export default function CaseAlertSection({
  id,
  label,
  summary,
  defaultOpen = false,
  children,
}: Props): JSX.Element {
  const [open, setOpen] = useState(() => startsOpen(id, defaultOpen));
  return (
    <section className="cas-sec" aria-label={label}>
      <details className="cas-det" open={open}>
        <summary
          onClick={(e) => {
            // The open state is React's, so a re-render never closes it.
            e.preventDefault();
            setOpen(!open);
            remember(id, !open, defaultOpen);
          }}
        >
          <span className="cas-mark" aria-hidden="true">
            {open ? "▾" : "▸"}
          </span>
          {summary}
        </summary>
        {open ? children : null}
      </details>
    </section>
  );
}
