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

import { useId, useRef, useState, type FormEvent, type JSX, type ReactNode, type RefObject } from "react";
import { familyOfLadder } from "../utils/caseAlertChainSettings";
import { useCaseAlertDrafts } from "../utils/caseAlertDrafts";
import type {
  CaseAlertFileOnly,
  CaseAlertLadderChannels,
  CaseAlertRoomLadder,
  CaseAlertLadderValue,
  CaseAlertRunningValue,
  CaseAlertSettings as Settings,
} from "../types";
import {
  ADVANCED_GROUPS,
  channelWords,
  draftFor,
  fileOnlyRows,
  formatLadderValue,
  ladderKeysFor,
  limitHint,
  parseDraft,
  sameLadderValue,
  type LadderDraft,
  type LadderKeyGroup,
  type LadderKeySpec,
} from "../utils/caseAlertLadderKeys";
import CaseAlertModal from "./CaseAlertModal";

function when(iso: string | undefined): string {
  return iso ? new Date(iso).toLocaleString() : "";
}

/** The saved keys of a ladder this page doesn't know. */
function unknownSaved(s: Settings, ladder: CaseAlertRoomLadder): CaseAlertLadderValue[] {
  const known = new Set(ladderKeysFor(ladder).map((k) => k.key));
  return s.values.filter((v) => v.ladder === ladder && !known.has(v.key));
}

interface GroupProps {
  ladder: CaseAlertRoomLadder;
  settings: Settings;
  group: LadderKeyGroup;
  /** Shown above the table, e.g. the ladder's name when a family has two. */
  heading?: string;
  /** Says what was saved, for the settings' confirmation line. */
  onSaved: (what: string) => void;
}

/**
 * One group of a ladder's paging file, key by key. Each row shows the value
 * the alert service runs with and where it came from, and, when one is saved
 * here that isn't in use yet, the saved one. A saved value replaces that one
 * key of the file; going back to the file's removes it. Changes are made in a
 * dialog from the row's Edit. Who may change a ladder's values is who may
 * change its rooms (canManage.roomLadders); everyone else reads it. Nothing
 * when the ladder has no key in the group.
 */
export function LadderValueGroup({ ladder, settings: s, group, heading, onSaved }: GroupProps): JSX.Element | null {
  const rows = ladderKeysFor(ladder).filter((k) => k.group === group);
  if (rows.length === 0) return null;
  const canEdit = s.canManage.roomLadders.includes(ladder);
  const running = s.running?.ladders[ladder] ?? {};
  return (
    <div className="cas-block">
      {heading ? <h5>{heading}</h5> : null}
      <ValueTable canEdit={canEdit}>
        {rows.map((spec) => (
          <ValueRow
            key={spec.key}
            ladder={ladder}
            spec={spec}
            saved={s.values.find((v) => v.ladder === ladder && v.key === spec.key)}
            running={running[spec.key]}
            reported={Boolean(s.running)}
            canEdit={canEdit}
            onSaved={onSaved}
          />
        ))}
      </ValueTable>
    </div>
  );
}

interface AdvancedProps {
  ladder: CaseAlertRoomLadder;
  settings: Settings;
  /** The ladder's step of copying the file's values here, when it has one. */
  setup?: ReactNode;
  onSaved: (what: string) => void;
}

/**
 * The rest of a ladder, under the family's Advanced: copying the file's values
 * here, when a ladder starts, how it climbs, the heads, the chat messages, any
 * saved key this page doesn't know, who gets which channel as running, and
 * what only the file sets.
 */
export function LadderAdvanced({ ladder, settings: s, setup, onSaved }: AdvancedProps): JSX.Element {
  const canEdit = s.canManage.roomLadders.includes(ladder);
  const running = s.running?.ladders[ladder] ?? {};
  // A saved key this page doesn't know is still shown, so nothing saved is
  // hidden, and can be taken back to the file's.
  const others = unknownSaved(s, ladder);
  return (
    <div className="cas-ladder" role="group" aria-label={`${ladder} ladder`}>
      <h4>{ladder} ladder</h4>
      {setup}
      {ADVANCED_GROUPS.map((group) => (
        <LadderValueGroup key={group} ladder={ladder} settings={s} group={group} heading={group} onSaved={onSaved} />
      ))}
      {others.length > 0 ? (
        <div className="cas-block">
          <h5>Other saved settings</h5>
          <ValueTable canEdit={canEdit}>
            {others.map((v) => (
              <ValueRow
                key={v.id}
                ladder={ladder}
                spec={{
                  key: v.key,
                  ladders: [ladder],
                  group: "How it climbs",
                  label: v.key,
                  help: "Not a setting this page knows. Go back to the file's value to remove it.",
                  editor: "text",
                }}
                saved={v}
                running={running[v.key]}
                reported={Boolean(s.running)}
                canEdit={canEdit}
                unknown
                onSaved={onSaved}
              />
            ))}
          </ValueTable>
        </div>
      ) : null}
      <ChannelsGroup ladder={ladder} channels={s.running?.channels?.[ladder]} />
      <FileOnlyGroup ladder={ladder} fileOnly={s.running?.fileOnly?.[ladder]} onTab={new Set(Object.keys(running))} />
    </div>
  );
}

/** Who gets which channel, as the alert service runs it now: whether the
 *  ladder runs, its channel, and each team's (the ladder's narrowed by the
 *  team's own). Read-only: the ladder's switches are edited under On/off and
 *  channel, a team's channel and room under Where alerts go. */
function ChannelsGroup({
  ladder,
  channels,
}: {
  ladder: CaseAlertRoomLadder;
  channels?: CaseAlertLadderChannels;
}): JSX.Element | null {
  if (!channels) return null;
  return (
    <div className="cas-block" role="group" aria-label={`${ladder}: who gets which channel`}>
      <h5>Who gets which channel</h5>
      <p className="cas-sub">
        {channels.enabled
          ? `The ${ladder} ladder is on: ${channelWords(channels.channel) ?? channels.channel}.`
          : `The ${ladder} ladder is off: nobody is alerted.`}{" "}
        A team's own channel can only narrow the ladder's.
      </p>
      {channels.teams.length > 0 ? (
        <table className="ra-tbl cas-values cas-channels">
          <thead>
            <tr>
              <th>Team</th>
              <th>Channel in use</th>
              <th>Own room</th>
            </tr>
          </thead>
          <tbody>
            {channels.teams.map((t) => (
              <tr key={t.team}>
                <td>
                  <b>{t.team}</b>
                </td>
                <td>
                  <span className="cas-val">
                    {channels.enabled ? (channelWords(t.channel) ?? t.channel) : "Off"}
                    {t.teamChannel ? " (team's own)" : ""}
                  </span>
                </td>
                <td>{t.room ? "Yes" : "No (shared room)"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : (
        <p className="cas-sub cas-empty">Every team uses the ladder's channel.</p>
      )}
    </div>
  );
}

/** The settings only the alert service's file can set, read-only for
 *  everyone. Whether a ladder runs, its channel and its safety holds are
 *  edited under On/off and channel once the alert service reports them; the
 *  rest stay a reviewed change to the file. */
function FileOnlyGroup({
  ladder,
  fileOnly,
  onTab,
}: {
  ladder: CaseAlertRoomLadder;
  fileOnly?: CaseAlertFileOnly;
  onTab: ReadonlySet<string>;
}): JSX.Element {
  return (
    <div className="cas-block" role="group" aria-label={`${ladder}: set in the alert service's file`}>
      <h5>Set in the alert service&apos;s file (read-only)</h5>
      <p className="cas-sub">
        These decide what alerts may cost. They change only with a reviewed change to the file.
      </p>
      {fileOnly ? (
        <table className="ra-tbl cas-values cas-file-only">
          <colgroup>
            <col className="cas-col-setting" />
            <col />
          </colgroup>
          <thead>
            <tr>
              <th>Setting</th>
              <th>In use</th>
            </tr>
          </thead>
          <tbody>
            {fileOnlyRows(ladder, fileOnly, onTab).map((r) => (
              <tr key={r.label}>
                <td>
                  <b>{r.label}</b>
                </td>
                <td>
                  <span className="cas-val">{r.value}</span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : (
        <p className="cas-sub cas-empty">
          Not reported yet: the alert service reports these when it runs in shadow or db mode.
        </p>
      )}
    </div>
  );
}

/** One group's rows. Every group is its own table, so the widths are fixed
 *  here (cas-col-*) and the columns line up from one group to the next. */
function ValueTable({ canEdit, children }: { canEdit: boolean; children: JSX.Element[] }): JSX.Element {
  return (
    <table className="ra-tbl cas-values">
      <colgroup>
        <col className="cas-col-setting" />
        <col />
        {/* Kept when it stays empty, so a read-only page lines up the same. */}
        <col className="cas-col-actions" />
      </colgroup>
      <thead>
        <tr>
          <th>Setting</th>
          <th>In use</th>
          {canEdit ? <th aria-label="Actions" /> : null}
        </tr>
      </thead>
      <tbody>{children}</tbody>
    </table>
  );
}

interface ValueRowProps {
  ladder: CaseAlertRoomLadder;
  spec: LadderKeySpec;
  saved?: CaseAlertLadderValue;
  running?: CaseAlertRunningValue;
  /** Whether the alert service has reported at all. */
  reported: boolean;
  canEdit: boolean;
  /** A key this page doesn't know: it can only go back to the file's. */
  unknown?: boolean;
  onSaved: (what: string) => void;
}

function ValueRow({
  ladder,
  spec,
  saved,
  running,
  reported,
  canEdit,
  unknown,
  onSaved,
}: ValueRowProps): JSX.Element {
  const [editing, setEditing] = useState(false);
  const editRef = useRef<HTMLButtonElement>(null);
  // The saved value, while it isn't the one the alert service runs with.
  const pending = saved && !(running && sameLadderValue(spec, saved.value, running.value));

  return (
    <tr>
      <td>
        <b>{spec.label}</b>
        <div className="cas-sub">{spec.help}</div>
      </td>
      <td>
        {running ? (
          <>
            <span className="cas-val">{formatLadderValue(spec, running.value)}</span>
            <span className={`cas-chip cas-src-${running.source}`}>
              {running.source === "file" ? "from the file" : "saved here"}
            </span>
          </>
        ) : (
          <>
            <span className="cas-sub">—</span>
            {reported ? null : <span className="cas-chip">not reported yet</span>}
          </>
        )}
        {pending ? (
          <div className="cas-sub cas-saved" title={`Saved by ${saved.updatedBy} at ${when(saved.updatedOn)}`}>
            Saved: {formatLadderValue(spec, saved.value)} · not in use yet
          </div>
        ) : null}
      </td>
      {canEdit ? (
        <td>
          <div className="cas-acts">
            <button ref={editRef} type="button" className="btn sm" onClick={() => setEditing(true)}>
              Edit
            </button>
          </div>
          {editing ? (
            <ValueDialog
              ladder={ladder}
              spec={spec}
              saved={saved}
              start={saved ? saved.value : running?.value}
              unknown={unknown}
              returnFocus={editRef}
              onClose={() => setEditing(false)}
              onSaved={onSaved}
            />
          ) : null}
        </td>
      ) : null}
    </tr>
  );
}

interface ValueDialogProps {
  ladder: CaseAlertRoomLadder;
  spec: LadderKeySpec;
  saved?: CaseAlertLadderValue;
  /** What the editor starts from: the saved value, else the one in use. */
  start: unknown;
  unknown?: boolean;
  /** The Edit button, which gets focus back when the dialog closes. */
  returnFocus: RefObject<HTMLButtonElement | null>;
  onClose: () => void;
  /** Says what was saved, for the confirmation line. */
  onSaved: (what: string) => void;
}

/** Change one ladder value, or go back to the file's, in the Alert Settings
 *  modal (CaseAlertModal). Focus goes back to the row's Edit. */
function ValueDialog({
  ladder,
  spec,
  saved,
  start,
  unknown,
  returnFocus,
  onClose,
  onSaved,
}: ValueDialogProps): JSX.Element {
  const drafts = useCaseAlertDrafts();
  const family = familyOfLadder(ladder);
  const ids = useId();
  const titleId = `${ids}-title`;
  const helpId = `${ids}-help`;
  const [draft, setDraft] = useState<LadderDraft>(() => draftFor(spec, start));
  const [error, setError] = useState<string | null>(null);
  const [confirmFile, setConfirmFile] = useState(false);
  const busy = drafts.submitting === family;
  const hint = unknown ? null : limitHint(spec);

  // min and max are hints: the server checks every limit and its message
  // names the key, so the browser doesn't stop the send.
  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (busy || unknown) return;
    setError(null);
    const parsed = parseDraft(spec, draft);
    if (!parsed.ok) {
      setError(parsed.error);
      return;
    }
    const what = `${ladder} ${spec.label}`;
    drafts.add({ family, what, kind: "values", input: { ladder, values: { [spec.key]: parsed.value } } });
    onSaved(what);
    onClose();
  };

  const goBackToFile = () => {
    if (!saved || busy) return;
    setError(null);
    const what = `${ladder} ${spec.label} uses the file's value again`;
    drafts.add({ family, what, kind: "deleteValue", valueId: saved.id });
    onSaved(what);
    onClose();
  };

  return (
    <CaseAlertModal titleId={titleId} describedBy={helpId} returnFocus={returnFocus} onClose={onClose}>
      <form className="cas-edit" noValidate onSubmit={submit}>
        <h3 id={titleId}>{spec.label}</h3>
        <p id={helpId} className="cp-help">
          {spec.help}
          {hint ? ` ${hint}` : ""}
        </p>
        {unknown ? (
          <p className="cp-help">Saved here: {formatLadderValue(undefined, saved?.value)}</p>
        ) : (
          <Editor spec={spec} draft={draft} onChange={setDraft} />
        )}
        {error ? (
          <p className="cp-err" role="alert">
            {error}
          </p>
        ) : null}
        {saved ? (
          <div className="cas-acts cas-dlg-file">
            {confirmFile ? (
              <>
                <span className="cas-sub">The saved value is removed and the file&apos;s is used.</span>
                <button
                  type="button"
                  className="btn sm danger"
                  aria-disabled={busy || undefined}
                  onClick={goBackToFile}
                >
                  Use the file&apos;s value
                </button>
                <button type="button" className="btn sm" onClick={() => setConfirmFile(false)}>
                  Keep it
                </button>
              </>
            ) : (
              <button type="button" className="btn sm" onClick={() => setConfirmFile(true)}>
                Use the file&apos;s value
              </button>
            )}
          </div>
        ) : null}
        <div className="cp-modal-actions">
          <button type="button" className="btn sm" onClick={onClose}>
            Cancel
          </button>
          {unknown ? null : (
            <button type="submit" className="btn sm primary" aria-disabled={busy || undefined}>
              Add to changes
            </button>
          )}
        </div>
      </form>
    </CaseAlertModal>
  );
}

interface EditorProps {
  spec: LadderKeySpec;
  draft: LadderDraft;
  onChange: (d: LadderDraft) => void;
}

/** The editor for one key, by the kind of value it takes. */
function Editor({ spec, draft, onChange }: EditorProps): JSX.Element {
  const text = (type: "text" | "number" = "text") => (
    <label>
      New value
      <input
        type={type}
        min={type === "number" ? spec.min : undefined}
        max={type === "number" ? spec.max : undefined}
        value={draft.text}
        placeholder={spec.placeholder}
        onChange={(e) => onChange({ ...draft, text: e.target.value })}
      />
    </label>
  );
  switch (spec.editor) {
    case "bool":
      return (
        <label className="cas-check">
          <input
            type="checkbox"
            checked={draft.checked}
            onChange={(e) => onChange({ ...draft, checked: e.target.checked })}
          />
          On
        </label>
      );
    case "int":
      return text("number");
    case "enum":
      return (
        <label>
          New value
          <select value={draft.text} onChange={(e) => onChange({ ...draft, text: e.target.value })}>
            {(spec.options ?? []).map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </select>
        </label>
      );
    case "head":
      return (
        <>
          <label>
            Name
            <input value={draft.name} onChange={(e) => onChange({ ...draft, name: e.target.value })} />
          </label>
          <label>
            Email
            <input type="email" value={draft.email} onChange={(e) => onChange({ ...draft, email: e.target.value })} />
          </label>
        </>
      );
    default:
      return text();
  }
}
