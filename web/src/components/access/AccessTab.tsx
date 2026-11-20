// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useEffect, useState } from 'react';
import { useStore, useStoreApi } from '../../state/StoreProvider';
import type { AccessData, AccessRows } from '../../state/store';
import type { BdqlError } from '../../api/types';
import { CodeView } from '../common/CodeView';
import { ErrorBox, Loading } from '../common/Async';
import { LabelChip } from './AccessUi';
import { cellText } from '../../model/export';
import { Icon } from '../common/Icon';

function RowsTable({ r, render }: { r: AccessRows; render?: (col: string, v: unknown, row: Record<string, unknown>) => React.ReactNode }) {
  return (
    <table className="tbl">
      <thead>
        <tr>
          {r.columns.map((c) => (
            <th key={c} className={c === 'nodes' ? 'num' : ''}>
              {c}
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {r.rows.map((row, i) => (
          <tr key={i} style={row.disabled === true ? { opacity: 0.5 } : undefined}>
            {r.columns.map((c) => (
              <td key={c} className={c === 'nodes' ? 'num' : c === 'name' || c === 'principal' ? 'mono' : ''}>
                {render ? render(c, row[c], row) : cellText(row[c])}
              </td>
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  );
}

export interface GrantEditorProps {
  labels: string[];
  users: string[];
  roles: string[];
  /** parse-only validation */
  validate: (bdql: string) => Promise<BdqlError[]>;
  onApply: (bdql: string) => void;
}

/** Builds GRANT / REVOKE / DENY statements, validates them via /api/run/explain, then applies. */
export function GrantEditor({ labels, users, roles, validate, onApply }: GrantEditorProps) {
  const [verb, setVerb] = useState<'GRANT' | 'REVOKE' | 'DENY'>('GRANT');
  const [actions, setActions] = useState<string[]>(['read']);
  const [label, setLabel] = useState(labels[0] ?? '');
  const [ptype, setPtype] = useState<'ROLE' | 'USER'>('ROLE');
  const [who, setWho] = useState(roles[0] ?? '');
  const [section, setSection] = useState('');
  const [errs, setErrs] = useState<BdqlError[] | null>(null);
  const bdql = `${verb} ${actions.join(', ') || 'read'} ON LABEL ${label || '?'} ${verb === 'REVOKE' ? 'FROM' : 'TO'} ${ptype} ${who || '?'}${section ? ` IN SECTION ${section}` : ''};`;
  useEffect(() => {
    let live = true;
    const t = setTimeout(async () => {
      try {
        const e = await validate(bdql);
        if (live) setErrs(e);
      } catch {
        if (live) setErrs(null);
      }
    }, 300);
    return () => {
      live = false;
      clearTimeout(t);
    };
  }, [bdql, validate]);
  const toggle = (a: string) => setActions(actions.includes(a) ? actions.filter((x) => x !== a) : [...actions, a]);
  const ok = !!label && !!who && actions.length > 0 && (!errs || errs.length === 0);
  return (
    <div className="col">
      <div className="row">
        <select className="input" aria-label="Statement" value={verb} onChange={(e) => setVerb(e.target.value as 'GRANT')}>
          <option>GRANT</option>
          <option>REVOKE</option>
          <option>DENY</option>
        </select>
        {['read', 'write', 'admin'].map((a) => (
          <label key={a} className="check small">
            <input type="checkbox" checked={actions.includes(a)} onChange={() => toggle(a)} /> {a}
          </label>
        ))}
        <span className="muted small">on label</span>
        <input className="input mono" list="acc-labels" aria-label="Label" style={{ width: 130 }} value={label} onChange={(e) => setLabel(e.target.value.trim())} />
        <datalist id="acc-labels">
          {labels.map((l) => (
            <option key={l} value={l} />
          ))}
          <option value="*" />
        </datalist>
      </div>
      <div className="row">
        <span className="muted small">{verb === 'REVOKE' ? 'from' : 'to'}</span>
        <select className="input" aria-label="Principal type" value={ptype} onChange={(e) => setPtype(e.target.value as 'ROLE')}>
          <option>ROLE</option>
          <option>USER</option>
        </select>
        <input className="input mono" list="acc-who" aria-label="Principal" style={{ width: 140 }} value={who} onChange={(e) => setWho(e.target.value.trim())} />
        <datalist id="acc-who">
          {(ptype === 'ROLE' ? roles : users).map((x) => (
            <option key={x} value={x} />
          ))}
        </datalist>
        <span className="muted small">in section</span>
        <input className="input" aria-label="Section (optional)" placeholder="(any)" style={{ width: 120 }} value={section} onChange={(e) => setSection(e.target.value.trim())} />
      </div>
      <CodeView text={bdql} lang="bdql" />
      {errs && errs.length > 0 && (
        <div className="err-box">
          {errs.map((e, i) => (
            <div key={i}>
              {e.line}:{e.col} {e.message}
            </div>
          ))}
        </div>
      )}
      {errs && errs.length === 0 && <div className="xs ok"><Icon name="check" size={14} /> statement is valid</div>}
      <div className="row" style={{ justifyContent: 'flex-end' }}>
        <button className="btn primary" disabled={!ok} onClick={() => onApply(bdql)}>
          Apply
        </button>
      </div>
    </div>
  );
}

/** EXPLAIN ACCESS: why a user can or cannot see a node. */
export function ExplainAccessTool({ users, run, initialNode = '', initialUser = '', initialText }: { users: string[]; run: (bdql: string) => Promise<string>; initialNode?: string; initialUser?: string; initialText?: string }) {
  const [node, setNode] = useState(initialNode);
  const [user, setUser] = useState(initialUser || users[1] || users[0] || '');
  const [text, setText] = useState<string | null>(initialText ?? null);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  return (
    <div className="col">
      <form
        className="row"
        onSubmit={async (e) => {
          e.preventDefault();
          setBusy(true);
          setErr(null);
          try {
            setText(await run(`EXPLAIN ACCESS ${node} FOR ${user};`));
          } catch (x) {
            setErr(x instanceof Error ? x.message : String(x));
          } finally {
            setBusy(false);
          }
        }}
      >
        <input className="input mono" aria-label="Node id" placeholder="node id, e.g. n154" style={{ width: 160 }} value={node} onChange={(e) => setNode(e.target.value.trim())} />
        <span className="muted small">for</span>
        <select className="input" aria-label="User" value={user} onChange={(e) => setUser(e.target.value)}>
          {users.map((u) => (
            <option key={u}>{u}</option>
          ))}
        </select>
        <button className="btn primary" type="submit" disabled={!node || !user || busy}>
          Explain
        </button>
      </form>
      {err && <ErrorBox error={err} />}
      {text && <CodeView text={text} lang="text" />}
    </div>
  );
}

/** Presentational Access panels (users, roles, grants, labels). */
export function AccessPanels({ data, onRun, validate, explain, onViewAs }: { data: AccessData; onRun: (bdql: string) => void; validate: GrantEditorProps['validate']; explain: (bdql: string) => Promise<string>; onViewAs?: (u: string) => void }) {
  const users = data.users.rows.map((r) => String(r.name));
  // SHOW roles names the column "role", not "name": reading r.name gave a list of "undefined".
  const roles = data.roles.rows.map((r) => String(r.role ?? r.name)).filter((r) => r && r !== 'undefined');
  const labels = data.labels.rows.map((r) => String(r.label));
  return (
    <div className="cards">
      <div className="card">
        <div className="card-h">
          Users <span className="muted mono">{users.length}</span>
        </div>
        <div className="card-b flush">
          <RowsTable
            r={data.users}
            render={(c, v, row) =>
              c === 'name' && onViewAs ? (
                <span className="row nowrap">
                  {String(v)}
                  {row.name !== 'admin' && (
                    <button className="btn sm ghost" onClick={() => onViewAs(String(v))} title={`Preview the brain as ${String(v)}`}>
                      <Icon name="eye" label={`View as ${String(v)}`} />
                    </button>
                  )}
                </span>
              ) : c === 'disabled' ? (
                v ? <span className="bad">disabled</span> : ''
              ) : (
                cellText(v)
              )
            }
          />
        </div>
      </div>
      <div className="card">
        <div className="card-h">
          Labels <span className="muted mono">{labels.length}</span>
        </div>
        <div className="card-b flush">
          <RowsTable r={data.labels} render={(c, v) => (c === 'label' ? <LabelChip label={String(v)} /> : cellText(v))} />
        </div>
      </div>
      <div className="card">
        <div className="card-h">
          Roles <span className="muted mono">{roles.length}</span>
        </div>
        <div className="card-b flush">
          <RowsTable r={data.roles} />
        </div>
      </div>
      <div className="card">
        <div className="card-h">
          Grants <span className="muted mono">{data.grants.rows.length}</span>
        </div>
        <div className="card-b flush">
          <RowsTable r={data.grants} render={(c, v) => (c === 'effect' || c === 'deny' ? (v === 'deny' || v === true ? <b className="bad">DENY</b> : 'allow') : cellText(v))} />
        </div>
      </div>
      <div className="card">
        <div className="card-h">Grant editor</div>
        <div className="card-b">
          <GrantEditor labels={labels} users={users} roles={roles} validate={validate} onApply={onRun} />
        </div>
      </div>
      <div className="card">
        <div className="card-h">EXPLAIN ACCESS</div>
        <div className="card-b">
          <ExplainAccessTool users={users} run={explain} />
        </div>
      </div>
    </div>
  );
}

/** Admin-only Access tab. */
export function AccessTab() {
  const store = useStoreApi();
  const who = useStore((s) => s.whoami);
  const access = useStore((s) => s.access);
  useEffect(() => {
    if (who?.admin && !access.data && !access.loading) store.loadAccess();
  }, [who, access.data, access.loading, store]);
  if (who && !who.admin) return <div className="tab-scroll"><ErrorBox error="The Access tab needs the admin role." /></div>;
  if (access.loading && !access.data) return <Loading />;
  if (access.error && !access.data) return <div className="tab-scroll"><ErrorBox error={access.error} onRetry={() => store.loadAccess()} /></div>;
  if (!access.data) return <Loading />;
  return (
    <div className="tab-scroll">
      <AccessPanels
        data={access.data}
        onRun={async (bdql) => {
          await store.runBdql(bdql, 'access');
          store.loadAccess();
        }}
        validate={async (bdql) => (await store.api.explain(bdql)).errors}
        explain={async (bdql) => {
          const r = await store.api.run(bdql);
          if (r.error) throw new Error(r.error.message);
          return r.results.map((x) => x.text ?? JSON.stringify(x.rows ?? x.changes, null, 2)).join('\n');
        }}
        onViewAs={(u) => store.setViewAs(u)}
      />
    </div>
  );
}
