// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { ReactNode } from 'react';
import type { GraphIndex } from '../../model/graph';
import { isConceptNode, isLit } from '../../model/graph';

export interface NodeRowProps {
  g: GraphIndex;
  id: string;
  right?: ReactNode;
  onGo?: (id: string) => void;
  showId?: boolean;
}

/** One node in a list: kind dot, text, value on the right. Click jumps to the node. */
export function NodeRow({ g, id, right, onGo, showId }: NodeRowProps) {
  const n = g.nodes.get(id);
  const concept = n ? isConceptNode(n) : id.startsWith('A:');
  return (
    <button type="button" className={'li' + (isLit(n) ? '' : ' dim')} onClick={() => onGo?.(id)} title={n ? `${id} · ${n.text}` : id}>
      <i className={'dot' + (concept ? ' diamond' : '') + (n?.external ? ' ring' : '')} style={{ background: g.color(id), color: g.color(id) }} />
      <span className="t">
        {showId && <span className="id">{id}</span>}
        {n ? (concept ? '◆ ' : '') + n.text : <span className="muted">{id} (not loaded)</span>}
        {n?.supersededBy && <span className="muted"> (superseded)</span>}
      </span>
      <span className="n">{right}</span>
    </button>
  );
}
