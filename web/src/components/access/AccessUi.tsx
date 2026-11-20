// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useEffect, useRef, useState } from 'react';
import type { Whoami } from '../../api/types';
import { labelColor } from '../graph/sceneShared';
import { Icon } from '../common/Icon';

/** Access label chip (hatched, coloured by label). */
export function LabelChip({ label }: { label: string }) {
  const c = labelColor(label);
  return (
    <span className="chip label-chip" style={{ height: 20, borderColor: c, background: `repeating-linear-gradient(135deg, transparent 0 4px, color-mix(in srgb, ${c} 22%, transparent) 4px 7px)` }} title={'access label ' + label}>
      <Icon name="lock" size={14} /> <span className="mono xs">{label}</span>
    </span>
  );
}

export function LabelChips({ labels }: { labels?: string[] }) {
  if (!labels?.length) return null;
  return (
    <span className="chips" style={{ display: 'inline-flex' }}>
      {labels.map((l) => (
        <LabelChip key={l} label={l} />
      ))}
    </span>
  );
}

/** Top-bar user menu: WHOAMI details and "Switch token…". */
export function UserMenu({ who, onSwitchToken, defaultOpen }: { who: Whoami | null; onSwitchToken: () => void; defaultOpen?: boolean }) {
  const [open, setOpen] = useState(!!defaultOpen);
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const h = (e: MouseEvent) => !ref.current?.contains(e.target as Node) && setOpen(false);
    document.addEventListener('mousedown', h);
    return () => document.removeEventListener('mousedown', h);
  }, [open]);
  if (!who) return null;
  return (
    <div className="conn" ref={ref} onKeyDown={(e) => e.key === 'Escape' && setOpen(false)}>
      <button className="btn" aria-haspopup="menu" aria-expanded={open} onClick={() => setOpen(!open)} title="Signed in as">
        <Icon name="user" /> <span className="mono">{who.name}</span>
        {who.admin && <span className="xs muted">admin</span>} ▾
      </button>
      {open && (
        <div className="conn-menu" role="menu" style={{ right: 0, left: 'auto', minWidth: 300, padding: 10 }}>
          <div style={{ fontWeight: 600 }}>{who.display ?? who.name}</div>
          <table className="props" style={{ marginTop: 6 }}>
            <tbody>
              <tr>
                <th>user</th>
                <td>{who.name}</td>
              </tr>
              <tr>
                <th>roles</th>
                <td>{(who.roles ?? []).join(', ') || '—'}</td>
              </tr>
              <tr>
                <th>labels</th>
                <td>{(who.labels ?? []).includes('*') ? 'all (*)' : <LabelChips labels={who.labels ?? []} />}</td>
              </tr>
              <tr>
                <th>databases</th>
                <td>{(who.dbs ?? []).join(', ') || 'all'}</td>
              </tr>
              {who.defaultWriteLabels?.length ? (
                <tr>
                  <th>writes get</th>
                  <td>{who.defaultWriteLabels.join(', ')}</td>
                </tr>
              ) : null}
            </tbody>
          </table>
          <button
            className="item"
            role="menuitem"
            style={{ marginTop: 6 }}
            onClick={() => {
              setOpen(false);
              onSwitchToken();
            }}
          >
            <Icon name="key" /> Switch token…
          </button>
        </div>
      )}
    </div>
  );
}

/** Admin "View as…" selector. */
export function ViewAsSelector({ users, value, onChange }: { users: string[]; value: string | null; onChange: (u: string | null) => void }) {
  return (
    <label className="row small" title="Render the brain exactly as another user sees it">
      <span className="muted">View as</span>
      <select className="input" value={value ?? ''} onChange={(e) => onChange(e.target.value || null)}>
        <option value="">myself (admin)</option>
        {users.map((u) => (
          <option key={u} value={u}>
            {u}
          </option>
        ))}
      </select>
    </label>
  );
}

/** Banner shown while an admin previews another user's view. */
export function ViewAsBanner({ user, onExit }: { user: string; onExit: () => void }) {
  return (
    <div className="viewas-banner" role="status">
      <Icon name="eye" /> Viewing as <b className="mono">{user}</b> — knowledge {user} cannot see is switched off everywhere (state, search, queries, counts). Writes are disabled in preview.
      <span className="spacer" />
      <button className="btn sm" onClick={onExit}>
        Exit preview
      </button>
    </div>
  );
}

/** Legend for the Graph 3D access overlay. */
export function AccessLegend({ labels }: { labels: { label: string; nodes: number }[] }) {
  return (
    <div className="canvas-overlay" style={{ right: 10, bottom: 10, display: 'flex', flexDirection: 'column', gap: 3 }} aria-label="Access overlay legend">
      <b className="small" style={{ color: 'var(--fg)' }}>
        <Icon name="lock" size={14} /> Access overlay
      </b>
      {labels.map((l) => (
        <span key={l.label} className="row nowrap">
          <i className="dot" style={{ background: labelColor(l.label) }} /> <span className="mono xs">{l.label}</span> <span className="xs">{l.nodes}</span>
        </span>
      ))}
      <span className="row nowrap">
        <i className="dot" style={{ background: labelColor(undefined) }} /> <span className="xs">unlabelled (everyone)</span>
      </span>
    </div>
  );
}
