// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { LiveStatus } from '../../api/client';
import type { State } from '../../api/types';
import { Icon } from '../common/Icon';

export interface StatusBarProps {
  db?: string | null;
  connection: string;
  live: LiveStatus;
  brain: State | null;
  lastOp: { name: string; ms: number; ok: boolean; error?: string } | null;
  storage?: string;
  format?: string;
  /** reviewers only: number of quarantined items */
  quarantine?: number;
  onQuarantine?: () => void;
}

/** Bottom status bar: connection, step, lit/total, tokens, depth/threshold, focus, storage/format, last op, SSE. */
export function StatusBar({ db, connection, live, brain, lastOp, storage, format, quarantine, onQuarantine }: StatusBarProps) {
  return (
    <footer className="statusbar" role="contentinfo">
      <span className="seg-item">
        <span className={'dot ' + (live === 'live' ? 'ok' : live === 'connecting' ? 'warn' : 'error')} aria-hidden />
        <b>{db ?? connection}</b>
      </span>
      {brain && (
        <>
          <span className="seg-item">
            step <b>{brain.step}</b>
          </span>
          <span className="seg-item">
            lit <b>{brain.stats.lit}/{brain.stats.memories}</b> + <b>{brain.stats.concepts}</b>◆
          </span>
          <span className="seg-item" style={brain.stats.tokens > 4000 ? { color: 'var(--bad)' } : undefined}>
            ~<b>{brain.stats.tokens}</b> tok
          </span>
          <span className="seg-item">
            depth <b>{brain.params.depth}</b> thr <b>{brain.params.threshold.toFixed(2)}</b>
          </span>
          <span className="seg-item ellipsis" style={{ maxWidth: 220 }}>
            focus <b>{brain.focus.join(',') || '—'}</b>
          </span>
        </>
      )}
      {storage && (
        <span className="seg-item">
          storage <b>{storage}{format ? ` · ${format}` : ''}</b>
        </span>
      )}
      {quarantine !== undefined && (
        <button className="seg-item link" style={{ color: quarantine ? 'var(--warn)' : 'var(--mut)', fontSize: 11.5 }} onClick={onQuarantine} title="Open the review queue">
          <Icon name="inbox" size={14} /> review <b style={{ color: 'inherit' }}>{quarantine}</b>
        </button>
      )}
      {lastOp && (
        <span className="seg-item" title={lastOp.error}>
          last <b style={{ color: lastOp.ok ? undefined : 'var(--bad)' }}>{lastOp.name}</b> {lastOp.ms} ms
        </span>
      )}
      <span className="spacer" />
      <span className="seg-item">
        SSE <b style={{ color: live === 'live' ? 'var(--ok)' : 'var(--warn)' }}>{live === 'live' ? '● live' : '○ ' + live}</b>
      </span>
      <span className="seg-item xs" style={{ color: 'var(--mut-2)' }}>
        brainDB · © Gabriel Koziestański
      </span>
    </footer>
  );
}
