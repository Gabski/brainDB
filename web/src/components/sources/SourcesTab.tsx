// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useState } from 'react';
import { useStore, useStoreApi } from '../../state/StoreProvider';
import type { Source, SourceInput } from '../../api/types';
import { ErrorBox, Loading } from '../common/Async';
import { ConfirmDialog } from '../common/Modal';
import { sourceColor } from '../graph/sceneShared';
import { Icon } from '../common/Icon';

/** Table of mounted sources with sync/remove. */
export function SourceList({ sources, onSync, onRemove, onToggle }: { sources: Source[]; onSync: (n: string) => void; onRemove: (n: string) => void; onToggle?: (s: Source) => void }) {
  if (!sources.length) return <div className="empty">No sources mounted. Add another brain, a brain file, a snapshot, a published binary or a web page.</div>;
  return (
    <table className="tbl">
      <thead>
        <tr>
          <th>status</th>
          <th>name</th>
          <th>uri</th>
          <th className="num">weight</th>
          <th>mode</th>
          <th className="num">ttl</th>
          <th className="num">depth</th>
          <th className="num">nodes</th>
          <th className="num">latency</th>
          <th>last sync</th>
          <th />
        </tr>
      </thead>
      <tbody>
        {sources.map((s) => (
          <tr key={s.name} title={s.lastError}>
            <td>
              <span className={'dot ' + s.status} /> <span className="xs">{s.status}</span>
            </td>
            <td>
              <i className="dot ring" style={{ color: sourceColor(s.name) }} /> <b className="mono">{s.name}</b>
            </td>
            <td className="mono ellipsis" style={{ maxWidth: 260 }}>
              {s.uri}
            </td>
            <td className="num">{s.weight.toFixed(2)}</td>
            <td>{s.mode}</td>
            <td className="num">{s.ttl}</td>
            <td className="num">{s.federateDepth}</td>
            <td className="num">{s.nodes}</td>
            <td className="num">{s.status === 'ok' ? `${s.latencyMs} ms` : '–'}</td>
            <td className="mono xs">{s.lastSync ? s.lastSync.replace('T', ' ').replace(/:\d\dZ$/, 'Z') : '–'}</td>
            <td>
              <span className="row nowrap">
                {onToggle && (
                  <button className="btn sm" onClick={() => onToggle(s)}>
                    {s.enabled ? 'Disable' : 'Enable'}
                  </button>
                )}
                <button className="btn sm" onClick={() => onSync(s.name)} disabled={!s.enabled}>
                  <Icon name="refresh" size={14} /> Sync
                </button>
                <button className="btn sm danger" onClick={() => onRemove(s.name)}>
                  Remove
                </button>
              </span>
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

/** Add/update a source. */
export function SourceForm({ onSubmit, initial }: { onSubmit: (s: SourceInput) => void; initial?: Partial<SourceInput> }) {
  const [s, set] = useState<SourceInput>({ name: '', uri: '', weight: 0.7, mode: 'live', ttl: 300, federateDepth: 1, enabled: true, tokenEnv: '', ...initial });
  const bad = s.name === 'A' || /[:\s]/.test(s.name);
  return (
    <form
      className="col"
      onSubmit={(e) => {
        e.preventDefault();
        if (s.name && s.uri && !bad) onSubmit({ ...s, tokenEnv: s.tokenEnv || undefined });
      }}
    >
      <div className="row">
        <input className="input" style={{ width: 120 }} aria-label="Source name" placeholder="name (e.g. lab)" value={s.name} onChange={(e) => set({ ...s, name: e.target.value })} />
        <input className="input grow mono" aria-label="Source URI" placeholder="http://lab:7791 · file:///… · snap:///… · exec:///… · url+https://…" value={s.uri} onChange={(e) => set({ ...s, uri: e.target.value })} />
      </div>
      <div className="row small">
        <label className="row">
          weight <input className="input" type="number" min={0} max={1} step={0.05} style={{ width: 70 }} value={s.weight} onChange={(e) => set({ ...s, weight: +e.target.value })} />
        </label>
        <label className="row">
          mode
          <select className="input" value={s.mode} onChange={(e) => set({ ...s, mode: e.target.value })}>
            <option>live</option>
            <option>cached</option>
          </select>
        </label>
        <label className="row">
          ttl s <input className="input" type="number" min={0} style={{ width: 80 }} value={s.ttl} onChange={(e) => set({ ...s, ttl: +e.target.value })} />
        </label>
        <label className="row">
          federate depth <input className="input" type="number" min={0} max={4} style={{ width: 56 }} value={s.federateDepth} onChange={(e) => set({ ...s, federateDepth: +e.target.value })} />
        </label>
        <label className="row">
          token env <input className="input mono" placeholder="LAB_TOKEN" style={{ width: 120 }} value={s.tokenEnv} onChange={(e) => set({ ...s, tokenEnv: e.target.value })} />
        </label>
        <button className="btn primary" type="submit" disabled={!s.name || !s.uri || bad}>
          Save source
        </button>
      </div>
      {bad && <div className="xs bad">A source name cannot be "A" or contain ":" or spaces.</div>}
      <div className="xs muted">Tokens are never stored: only the name of the environment variable that holds them. Sources are read-only.</div>
    </form>
  );
}

export function SourcesTab() {
  const store = useStoreApi();
  const sources = useStore((s) => s.sources);
  const [rm, setRm] = useState<string | null>(null);
  return (
    <div className="tab-scroll">
      <div className="card" style={{ marginBottom: 12 }}>
        <div className="card-h">Add source</div>
        <div className="card-b">
          <SourceForm onSubmit={(s) => store.addSource(s)} />
        </div>
      </div>
      <div className="card">
        <div className="card-h">
          Mounted sources
          <span className="spacer" />
          <button className="btn sm ghost" onClick={() => store.loadSources()}>
            <Icon name="refresh" size={14} /> Refresh
          </button>
        </div>
        <div className="card-b flush" style={{ maxHeight: 'none' }}>
          {sources.loading && !sources.data ? (
            <Loading />
          ) : sources.error && !sources.data ? (
            <div style={{ padding: 10 }}>
              <ErrorBox error={sources.error} onRetry={() => store.loadSources()} />
            </div>
          ) : (
            <SourceList
              sources={sources.data ?? []}
              onSync={(n) => store.syncSource(n)}
              onRemove={setRm}
              onToggle={(s) => store.addSource({ name: s.name, uri: s.uri, weight: s.weight, mode: s.mode, ttl: s.ttl, federateDepth: s.federateDepth, enabled: !s.enabled })}
            />
          )}
        </div>
      </div>
      {rm && (
        <ConfirmDialog
          title={`Remove source "${rm}"?`}
          text="External links to its nodes stay as offline stubs until you unlink them."
          confirmLabel="Remove"
          danger
          onCancel={() => setRm(null)}
          onConfirm={() => {
            store.removeSource(rm);
            setRm(null);
          }}
        />
      )}
    </div>
  );
}
