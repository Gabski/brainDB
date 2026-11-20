// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { Kind } from '../../api/types';
import { CONNECTOR_COLOR } from './sceneShared';

/** Kind colours plus edge-type swatches. */
export function Legend({ kinds, showConnectors = true, sources = [] }: { kinds: Kind[]; showConnectors?: boolean; sources?: { name: string; color: string }[] }) {
  return (
    <div className="canvas-overlay legend" aria-label="Legend">
      {kinds.map((k) => (
        <span key={k.name}>
          <i className={'dot' + (k.name === 'concept' ? ' diamond' : '')} style={{ background: k.color }} />
          {k.label}
        </span>
      ))}
      {sources.map((s) => (
        <span key={s.name}>
          <i className="dot ring" style={{ color: s.color }} />
          {s.name}
        </span>
      ))}
      <span><i className="dot" style={{ background: '#26c6da' }} />~ vector similarity</span>
      <span><i className="dot" style={{ background: '#74c94a' }} />file → chunks</span>
      <span><i className="dot" style={{ background: '#3a2a44' }} />superseded</span>
      <span><i className="dot" style={{ background: '#ffd43b' }} />+ new link</span>
      <span><i className="dot" style={{ background: '#bf73ff' }} />external</span>
      {showConnectors && <span><i className="dot ring" style={{ color: CONNECTOR_COLOR }} />⧉ connector</span>}
    </div>
  );
}
