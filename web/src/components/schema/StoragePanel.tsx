// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useState } from 'react';
import type { ServerStats } from '../../api/types';
import { ConfirmDialog } from '../common/Modal';
import { ErrorBox, Loading } from '../common/Async';
import { Icon } from '../common/Icon';

export interface StoragePanelProps {
  stats?: ServerStats;
  loading?: boolean;
  error?: string;
  onRefresh?: () => void;
  onSnapshot: () => void;
  onMigrate: (to: 'file' | 'sqlite') => void;
  onConvert: (to: 'xml' | 'bin' | 'both') => void;
  onExportXml: () => void;
  /** Message shown after a convert attempt the server does not support. */
  convertNote?: string | null;
}

const fmtBytes = (b: number) => (b > 1e9 ? (b / 1e9).toFixed(2) + ' GB' : b > 1e6 ? (b / 1e6).toFixed(1) + ' MB' : b > 1e3 ? (b / 1e3).toFixed(1) + ' KB' : b + ' B');

/** Storage & performance (Addenda F and L): backend, format, files, counts, indexes, cache, latencies, actions. */
export function StoragePanel({ stats, loading, error, onRefresh, onSnapshot, onMigrate, onConvert, onExportXml, convertNote }: StoragePanelProps) {
  const [confirm, setConfirm] = useState<null | { kind: 'migrate'; to: 'file' | 'sqlite' } | { kind: 'convert'; to: 'xml' | 'bin' | 'both' }>(null);
  const [convTo, setConvTo] = useState<'xml' | 'bin' | 'both'>('bin');
  if (loading && !stats) return <Loading />;
  if (error && !stats) return <ErrorBox error={error} onRetry={onRefresh} />;
  if (!stats) return null;
  const other: 'file' | 'sqlite' = stats.storage === 'sqlite' ? 'file' : 'sqlite';
  return (
    <div>
      <table className="props">
        <tbody>
          <tr>
            <th>backend</th>
            <td>
              {stats.storage}
              {stats.redis && (
                <span className={stats.redis === 'ok' ? 'ok' : 'bad'}>
                  {' '}
                  · redis {stats.redis}
                </span>
              )}
            </td>
          </tr>
          {stats.format && (
            <tr>
              <th>file format</th>
              <td>{stats.format}</td>
            </tr>
          )}
          <tr>
            <th>nodes / edges</th>
            <td>
              {stats.nodes.toLocaleString()} / {stats.edges.toLocaleString()}
            </td>
          </tr>
          {stats.cacheHitRate !== undefined && (
            <tr>
              <th>cache hit rate</th>
              <td>
                <div className="row nowrap">
                  {(stats.cacheHitRate * 100).toFixed(0)}%
                  <div className="bar grow">
                    <div style={{ width: `${stats.cacheHitRate * 100}%` }} />
                  </div>
                </div>
              </td>
            </tr>
          )}
        </tbody>
      </table>
      {stats.files?.length ? (
        <>
          <h3 className="section-h" style={{ marginTop: 12 }}>
            Files
          </h3>
          <table className="tbl">
            <thead>
              <tr>
                <th>file</th>
                <th className="num">size</th>
              </tr>
            </thead>
            <tbody>
              {stats.files.map((f) => (
                <tr key={f.name}>
                  <td className="mono">{f.name}</td>
                  <td className="num">{fmtBytes(f.bytes)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </>
      ) : null}
      {stats.indexes?.length ? (
        <>
          <h3 className="section-h" style={{ marginTop: 12 }}>
            Indexes
          </h3>
          <table className="tbl">
            <thead>
              <tr>
                <th>index</th>
                <th className="num">entries</th>
              </tr>
            </thead>
            <tbody>
              {stats.indexes.map((i) => (
                <tr key={i.name}>
                  <td>{i.name}</td>
                  <td className="num">{i.size.toLocaleString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </>
      ) : null}
      {stats.latency?.length ? (
        <>
          <h3 className="section-h" style={{ marginTop: 12 }}>
            Latency (ms)
          </h3>
          <table className="tbl">
            <thead>
              <tr>
                <th>op</th>
                <th className="num">p50</th>
                <th className="num">p95</th>
              </tr>
            </thead>
            <tbody>
              {stats.latency.map((l) => (
                <tr key={l.op}>
                  <td>{l.op}</td>
                  <td className="num">{l.p50.toFixed(1)}</td>
                  <td className={'num' + (l.p95 > 20 ? ' warn' : '')}>{l.p95.toFixed(1)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </>
      ) : null}
      {!stats.indexes?.length && !stats.latency?.length && (
        <p className="xs muted" style={{ marginTop: 12 }}>
          Index sizes, cache hit rate and latencies arrive with the SQLite query pushdown.
        </p>
      )}
      <div className="row" style={{ marginTop: 10 }}>
        <button className="btn" onClick={onSnapshot}>
          <Icon name="snapshot" /> Snapshot now
        </button>
        <button className="btn" onClick={() => setConfirm({ kind: 'migrate', to: other })}>
          <Icon name="database" /> Migrate to {other}
        </button>
        <span className="row nowrap">
          <select className="input" aria-label="Convert to format" value={convTo} onChange={(e) => setConvTo(e.target.value as 'xml' | 'bin' | 'both')}>
            {['xml', 'bin', 'both'].map((f) => (
              <option key={f} value={f} disabled={f === stats.format}>
                {f}
              </option>
            ))}
          </select>
          <button className="btn" disabled={convTo === stats.format} onClick={() => setConfirm({ kind: 'convert', to: convTo })}>
            Convert to…
          </button>
        </span>
        <button className="btn" onClick={onExportXml}>
          <Icon name="download" /> Export XML
        </button>
        {onRefresh && (
          <button className="btn ghost" onClick={onRefresh}>
            <Icon name="refresh" label="Refresh" />
          </button>
        )}
      </div>
      {convertNote && (
        <div className="xs warn" style={{ marginTop: 6 }}>
          {convertNote}
        </div>
      )}
      {confirm && (
        <ConfirmDialog
          title={confirm.kind === 'migrate' ? `Migrate storage to ${confirm.to}?` : `Convert files to ${confirm.to}?`}
          text={
            confirm.kind === 'migrate' ? (
              <>Copies every node, edge and index to the <b>{confirm.to}</b> backend. Writes are paused while it runs. `braindb export` keeps working either way.</>
            ) : (
              <>Rewrites the database files in the <b>{confirm.to}</b> format (current: {stats.format ?? 'unknown'}). A snapshot is taken first.</>
            )
          }
          confirmLabel={confirm.kind === 'migrate' ? 'Migrate' : 'Convert'}
          onCancel={() => setConfirm(null)}
          onConfirm={() => {
            if (confirm.kind === 'migrate') onMigrate(confirm.to);
            else onConvert(confirm.to);
            setConfirm(null);
          }}
        />
      )}
    </div>
  );
}
