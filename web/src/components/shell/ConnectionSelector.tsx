// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useEffect, useRef, useState } from 'react';
import type { DbInfo, Source } from '../../api/types';
import { Icon } from '../common/Icon';

export interface ConnectionSelectorProps {
  current: string;
  dbs?: DbInfo[];
  sources?: Source[];
  live: 'connecting' | 'live' | 'offline';
  onSwitchDb: (name: string) => void;
  onNewDb?: () => void;
  /** false: this server runs a single --brain file, so there is nothing to create. */
  canManageDbs?: boolean;
  onSource?: (name: string) => void;
  onAddSource?: () => void;
  defaultOpen?: boolean;
}

const fmt = (b: number) => (b > 1e6 ? (b / 1e6).toFixed(1) + ' MB' : (b / 1e3).toFixed(0) + ' KB');

/** Databases on this server + mounted sources, with health dots. */
export function ConnectionSelector({ current, dbs, sources, live, onSwitchDb, onNewDb, onSource, onAddSource, canManageDbs = true, defaultOpen }: ConnectionSelectorProps) {
  const [open, setOpen] = useState(!!defaultOpen);
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const h = (e: MouseEvent) => !ref.current?.contains(e.target as Node) && setOpen(false);
    document.addEventListener('mousedown', h);
    return () => document.removeEventListener('mousedown', h);
  }, [open]);
  return (
    <div className="conn" ref={ref} onKeyDown={(e) => e.key === 'Escape' && setOpen(false)}>
      <button className="btn" aria-haspopup="menu" aria-expanded={open} onClick={() => setOpen(!open)}>
        <span className={'dot ' + (live === 'live' ? 'ok' : live === 'connecting' ? 'warn' : 'error')} />
        <span className="mono">{current}</span> ▾
      </button>
      {open && (
        <div className="conn-menu" role="menu">
          <div className="section-h" style={{ margin: '4px 8px' }}>
            Databases
          </div>
          {(dbs ?? []).map((d) => (
            <button
              key={d.name}
              role="menuitemradio"
              aria-checked={d.name === current}
              className="item"
              onClick={() => {
                onSwitchDb(d.name);
                setOpen(false);
              }}
              title={d.goal}
            >
              <span className="dot ok" />
              <b className="mono">{d.name}</b>
              {d.name === current && <span className="xs muted">(active)</span>}
              <span className="meta">
                {d.nodes} nodes · {fmt(d.sizeBytes)}
              </span>
            </button>
          ))}
          {!dbs?.length && <div className="item muted">this server hosts one database</div>}
          {onNewDb && canManageDbs && (
            <button
              role="menuitem"
              className="item"
              onClick={() => {
                onNewDb();
                setOpen(false);
              }}
            >
              <Icon name="add" /> New database…
            </button>
          )}
          {onNewDb && !canManageDbs && (
            <div className="item muted" title="Start the server with --data-dir to host several databases">
              <Icon name="info" /> started with --brain: one database only
            </div>
          )}
          <div className="section-h" style={{ margin: '8px 8px 4px' }}>
            Mounted sources
          </div>
          {(sources ?? []).map((s) => (
            <button
              key={s.name}
              role="menuitem"
              className="item"
              onClick={() => {
                onSource?.(s.name);
                setOpen(false);
              }}
              title={s.lastError ?? s.uri}
            >
              <span className={'dot ' + s.status} />
              <span className="mono">{s.name}</span>
              <span className="meta">{s.status === 'ok' ? `${s.latencyMs} ms · ${s.nodes} nodes` : s.status}</span>
            </button>
          ))}
          {!sources?.length && <div className="item muted">no sources</div>}
          {onAddSource && (
            <button
              role="menuitem"
              className="item"
              onClick={() => {
                onAddSource();
                setOpen(false);
              }}
            >
              <Icon name="add" /> Add source…
            </button>
          )}
        </div>
      )}
    </div>
  );
}
