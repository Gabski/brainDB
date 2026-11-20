// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useMemo, useState, type ReactNode } from 'react';
import type { GraphIndex } from '../../model/graph';
import { isConceptNode, isLit } from '../../model/graph';
import { snippet } from '../../model/browse';
import { HiText } from '../common/HiText';
import { Icon } from '../common/Icon';

export interface ResultRowProps {
  g: GraphIndex;
  id: string;
  /** Terms to highlight (query words + the narrowing box). */
  terms: string[];
  right?: ReactNode;
  onGo?: (id: string) => void;
  /** Federated source shown as a tag before the metrics. */
  source?: string;
  /** Extra line under the text (tree depth, route, …). */
  foot?: ReactNode;
}

/**
 * One search hit: kind dot, id, a snippet around the match with the query highlighted, metrics
 * on the right. The caret expands the whole memory in place, so reading never leaves the results.
 */
export function ResultRow({ g, id, terms, right, onGo, source, foot }: ResultRowProps) {
  const [open, setOpen] = useState(false);
  const n = g.nodes.get(id);
  const text = n?.text ?? '';
  const s = useMemo(() => snippet(text, terms, 260), [text, terms]);
  const concept = n ? isConceptNode(n) : id.startsWith('A:');
  const long = text.length > s.text.length;
  return (
    <div className={'result-row' + (isLit(n) ? '' : ' dim') + (open ? ' open' : '')}>
      <button className="caret" aria-expanded={open} aria-label={open ? 'Collapse' : 'Expand'} onClick={() => setOpen(!open)} disabled={!long && !n?.fields}>
        {long || n?.fields ? (open ? '▾' : '▸') : ''}
      </button>
      <button className="main" onClick={() => onGo?.(id)} title={n ? id + ' · ' + n.text : id}>
        <span className="head">
          <i className={'dot' + (concept ? ' diamond' : '') + (n?.external ? ' ring' : '')} style={{ background: g.color(id), color: g.color(id) }} />
          <span className="id mono xs muted">{id}</span>
          {n && <span className="kind xs muted">{g.kind(n.kind).label}</span>}
          {n?.pinned && <Icon name="pin" size={14} label="pinned" />}
          {n?.supersededBy && <span className="xs muted">⇄ superseded</span>}
          <span className="spacer" />
          {source && source !== 'local' && (
            <span className="xs muted">
              <Icon name="source" size={14} /> {source}
            </span>
          )}
          <span className="n">{right}</span>
        </span>
        <span className="t">{n ? <HiText text={open ? text : s.text} terms={terms} /> : <span className="muted">{id} (not in this snapshot)</span>}</span>
        {open && n?.fields && (
          <span className="fields xs muted">
            {Object.entries(n.fields).map(([k, v]) => (
              <span key={k}>
                {k}=<b className="mono">{v}</b>
              </span>
            ))}
          </span>
        )}
        {foot}
      </button>
    </div>
  );
}
