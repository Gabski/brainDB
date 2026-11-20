// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useMemo, useState } from 'react';
import type { InspectLink, InspectNear, VectorResult } from '../../api/types';
import { EDGE_GLYPH, type GraphIndex } from '../../model/graph';
import { NodeRow } from '../common/NodeRow';
import { Empty } from '../common/Async';
import { Icon } from '../common/Icon';

/**
 * Links of a node. With more than one edge type they are bucketed per type (strongest first
 * inside each bucket) and the type chips narrow the list.
 */
export function LinksList({ g, links, onGo }: { g: GraphIndex; links: InspectLink[]; onGo: (id: string) => void }) {
  const [only, setOnly] = useState<string | null>(null);
  const byType = useMemo(() => {
    const m = new Map<string, InspectLink[]>();
    for (const l of links) {
      if (!m.has(l.type)) m.set(l.type, []);
      m.get(l.type)!.push(l);
    }
    for (const list of m.values()) list.sort((a, b) => b.w - a.w);
    return [...m.entries()].sort((a, b) => b[1].length - a[1].length || (a[0] < b[0] ? -1 : 1));
  }, [links]);
  if (!links.length) return <Empty>no links</Empty>;
  const shown = only ? byType.filter(([t]) => t === only) : byType;
  return (
    <div>
      {byType.length > 1 && (
        <div className="chips" style={{ margin: '0 0 6px' }}>
          {byType.map(([t, list]) => (
            <button key={t} className={'chip' + (only === t ? ' on' : '')} aria-pressed={only === t} onClick={() => setOnly(only === t ? null : t)} title={`${t} links`}>
              {EDGE_GLYPH[t] ?? '·'} {t} <span className="muted mono xs">{list.length}</span>
            </button>
          ))}
        </div>
      )}
      {shown.map(([t, list]) => (
        <div key={t} className="link-group">
          {shown.length > 1 && (
            <div className="link-group-h xs muted">
              {EDGE_GLYPH[t] ?? '·'} {t} <span className="mono">{list.length}</span>
            </div>
          )}
          <div className="list">
            {list.map((l) => (
              <NodeRow key={l.to + l.type} g={g} id={l.to} onGo={onGo} right={`${EDGE_GLYPH[l.type] ?? '·'} ${l.w.toFixed(2)}`} />
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}

/** Nearest neighbours by cosine; unlinked ones get a "+ link" button. */
export function NearestList({ g, near, onGo, onLink, use }: { g: GraphIndex; near: InspectNear[]; onGo: (id: string) => void; onLink?: (id: string, cos: number) => void; use?: string }) {
  if (!near.length) return <Empty>no neighbours</Empty>;
  return (
    <div className="list" aria-label={'Nearest (' + (use ?? '') + ')'}>
      {near.map((x) => (
        <NodeRow
          key={x.id}
          g={g}
          id={x.id}
          onGo={onGo}
          right={
            <>
              cos {x.cos.toFixed(2)}{' '}
              {x.linked ? (
                <Icon name="link" size={14} label="already linked" />
              ) : (
                onLink && (
                  <span
                    role="button"
                    tabIndex={0}
                    className="btn sm icon"
                    title="add a similarity link"
                    onClick={(e) => {
                      e.stopPropagation();
                      onLink(x.id, x.cos);
                    }}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') {
                        e.stopPropagation();
                        onLink(x.id, x.cos);
                      }
                    }}
                  >
                    <Icon name="add" size={14} />
                  </span>
                )
              )}
            </>
          }
        />
      ))}
    </div>
  );
}

function VecSvg({ v, color }: { v: number[]; color: string }) {
  const n = Math.min(64, v.length),
    w = 300 / Math.max(1, n);
  let mx = 0;
  for (let i = 0; i < n; i++) mx = Math.max(mx, Math.abs(v[i]));
  return (
    <svg className="vec" viewBox="0 0 300 34" preserveAspectRatio="none" aria-hidden>
      {v.slice(0, n).map((x, i) => {
        const h = (x / (mx || 1)) * 16;
        return <rect key={i} x={i * w} y={h > 0 ? 17 - h : 17} width={Math.max(0.5, w - 0.6)} height={Math.abs(h)} fill={color} />;
      })}
    </svg>
  );
}

/** First 64 dimensions of the raw and enriched vectors. */
export function VectorBars({ vector, model, alpha, hops, concept }: { vector: VectorResult; model: string; alpha: number; hops: number; concept?: boolean }) {
  return (
    <div>
      <div className="xs muted">
        raw (from text{concept ? ' + mean of members' : ''}), first 64 of {vector.dim} dims · {model}
      </div>
      <VecSvg v={vector.raw} color="var(--mut-2)" />
      <div className="xs muted">
        graph-enriched (α {alpha.toFixed(2)}, {hops} hops) · drift from raw {vector.drift.toFixed(3)}
      </div>
      <VecSvg v={vector.enriched} color="var(--accent)" />
    </div>
  );
}
