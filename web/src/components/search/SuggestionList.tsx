// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { Suggestion } from '../../api/types';
import { clamp, short } from '../../model/util';
import { isConceptNode, type GraphIndex } from '../../model/graph';
import { useState } from 'react';
import { Icon } from '../common/Icon';

export interface SuggestionListProps {
  g: GraphIndex;
  title: string;
  list: Suggestion[];
  onGo: (id: string) => void;
  onPreview?: (u: string, v: string) => void;
  onAdd?: (u: string, v: string, w: number) => void;
}

/** Proposed links with score breakdown, preview (pulse) and one-click add. */
export function SuggestionList({ g, title, list, onGo, onPreview, onAdd }: SuggestionListProps) {
  const [added, setAdded] = useState<Set<string>>(new Set());
  return (
    <div className="tool">
      <h4>{title}</h4>
      <div className="xs muted">Score = Adamic-Adar over shared neighbours (weighted, through concepts) + shared word stems + vector similarity (cos &gt; 0.2). Added links are gold.</div>
      <div className="list" style={{ marginTop: 6 }}>
        {list.length === 0 && <span className="muted small">no candidates</span>}
        {list.map((s) => {
          const w = clamp(0.3 + s.score * 0.6, 0.2, 0.9);
          const key = s.u + '|' + s.v;
          return (
            <div key={key} className="li" style={{ cursor: 'default', gridTemplateColumns: '10px minmax(0,1fr) auto', alignItems: 'start', padding: '4px 6px' }}>
              <i className="dot" style={{ background: g.color(s.v), marginTop: 5 }} />
              <span className="t" style={{ whiteSpace: 'normal' }}>
                <button className="link" onClick={() => onGo(s.u)}>
                  {short(g.nodes.get(s.u)?.text ?? s.u, 26)}
                </button>{' '}
                ↔{' '}
                <button className="link" onClick={() => onGo(s.v)}>
                  {short(g.nodes.get(s.v)?.text ?? s.v, 26)}
                </button>
                <br />
                <span className="xs muted">
                  via:{' '}
                  {s.via
                    .slice(0, 3)
                    .map((z) => {
                      const n = g.nodes.get(z);
                      return n && isConceptNode(n) ? '◆' + n.text : z;
                    })
                    .join(', ') || '—'}
                  {s.commonStems ? ` · ${s.commonStems} shared words` : ''}
                  {s.cos > 0.2 ? ` · cos ${s.cos.toFixed(2)}` : ''}
                </span>
              </span>
              <span className="n" style={{ flexDirection: 'column', alignItems: 'flex-end' }}>
                {s.score.toFixed(2)}
                <span className="row nowrap">
                  {onPreview && (
                    <button className="btn sm icon" title="preview" aria-label={`Preview ${s.u} ↔ ${s.v}`} onClick={() => onPreview(s.u, s.v)}>
                      <Icon name="eye" size={14} />
                    </button>
                  )}
                  {onAdd && (
                    <button
                      className="btn sm"
                      disabled={added.has(key)}
                      onClick={() => {
                        onAdd(s.u, s.v, w);
                        setAdded(new Set([...added, key]));
                      }}
                    >
                      {added.has(key) ? <><Icon name="check" size={14} /> added</> : <><Icon name="add" size={14} /> w{w.toFixed(2)}</>}
                    </button>
                  )}
                </span>
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
}
