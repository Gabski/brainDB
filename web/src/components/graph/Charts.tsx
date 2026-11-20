// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { BrainNode } from '../../api/types';
import { DEPTH_COLORS, isConceptNode, isLit, type GraphIndex } from '../../model/graph';
import { short } from '../../model/util';

/** Reach by depth: memories (coloured by depth) + concepts (◆, stacked). */
export function DepthChart({ nodes, depth, height = 110 }: { nodes: BrainNode[]; depth: number; height?: number }) {
  const D = Math.max(depth, 1);
  const mem = new Array(D + 1).fill(0),
    con = new Array(D + 1).fill(0);
  for (const n of nodes) if (isLit(n) && (n.depth ?? 0) <= D) (isConceptNode(n) ? con : mem)[n.depth ?? 0]++;
  const W = 260,
    H = height;
  const mx = Math.max(1, ...mem.map((m, i) => m + con[i])),
    bw = (W - 20) / (D + 1);
  return (
    <svg className="chart" viewBox={`0 0 ${W} ${H}`} style={{ height: H }} role="img" aria-label="Reach by depth">
      {mem.map((m, d) => {
        const x = 10 + d * bw,
          hm = (m / mx) * (H - 34),
          ha = (con[d] / mx) * (H - 34);
        return (
          <g key={d}>
            <rect x={x + 4} y={H - 18 - hm} width={bw - 8} height={hm} fill={DEPTH_COLORS[Math.min(d, 6)]} />
            <rect x={x + 4} y={H - 18 - hm - ha} width={bw - 8} height={ha} fill="var(--mut-2)" opacity={0.6} />
            <text x={x + bw / 2} y={H - 5} fontSize={10} textAnchor="middle">
              d{d}
            </text>
            {m + con[d] > 0 && (
              <text x={x + bw / 2} y={H - 22 - hm - ha} fontSize={10} textAnchor="middle" style={{ fill: 'var(--fg)' }}>
                {m}
                {con[d] ? `+${con[d]}◆` : ''}
              </text>
            )}
          </g>
        );
      })}
    </svg>
  );
}

/** Weight × age (log) scatter. Lit = solid, faded = translucent, superseded = ×. */
export function DecayScatter({ nodes, kindColor, height = 110 }: { nodes: BrainNode[]; kindColor: (k: string) => string; height?: number }) {
  const W = 260,
    H = height;
  const X = (a: number) => 22 + (Math.log1p(Math.min(a, 320)) / Math.log1p(320)) * (W - 30);
  const Y = (w: number) => H - 16 - w * (H - 24);
  return (
    <svg className="chart" viewBox={`0 0 ${W} ${H}`} style={{ height: H }} role="img" aria-label="Weight by age">
      <path className="axis" d={`M22 6 L22 ${H - 16} L${W - 6} ${H - 16}`} fill="none" />
      <text x={8} y={12} fontSize={9}>
        w
      </text>
      <text x={W - 6} y={H - 4} fontSize={9} textAnchor="end">
        age (log) →
      </text>
      {nodes
        .filter((n) => !isConceptNode(n) && !n.external)
        .map((n) => {
          const x = X(n.age),
            y = Y(n.weight);
          if (n.supersededBy)
            return <path key={n.id} d={`M${x - 2} ${y - 2}L${x + 2} ${y + 2}M${x + 2} ${y - 2}L${x - 2} ${y + 2}`} stroke="#8a5a9a" />;
          const lit = isLit(n);
          return <circle key={n.id} cx={x} cy={y} r={lit ? 3 : 2} fill={kindColor(n.kind)} opacity={lit ? 1 : 0.3}><title>{`${n.id} · w ${n.weight.toFixed(2)} · ${n.age}d`}</title></circle>;
        })}
    </svg>
  );
}

/** Top lit concepts by activation. */
export function ConceptBars({ g, height = 150, onSelect }: { g: GraphIndex; height?: number; onSelect?: (id: string) => void }) {
  const list = g.conceptIds
    .map((id) => g.nodes.get(id)!)
    .filter(isLit)
    .sort((a, b) => (b.activation ?? 0) - (a.activation ?? 0))
    .slice(0, 8);
  const W = 260,
    H = height,
    bh = (H - 8) / 8;
  return (
    <svg className="chart" viewBox={`0 0 ${W} ${H}`} style={{ height: H }} role="img" aria-label="Top lit concepts">
      {!list.length && (
        <text x={10} y={20} fontSize={11}>
          no active concepts
        </text>
      )}
      {list.map((n, i) => {
        const y = 4 + i * bh;
        return (
          <g key={n.id} style={{ cursor: onSelect ? 'pointer' : undefined }} onClick={() => onSelect?.(n.id)}>
            <rect x={110} y={y + 3} width={W - 118} height={bh - 7} fill="var(--bg)" stroke="var(--line)" />
            <rect x={110} y={y + 3} width={(W - 118) * (n.activation ?? 0)} height={bh - 7} fill={DEPTH_COLORS[Math.min(n.depth ?? 0, 6)]} />
            <text x={104} y={y + bh / 2 + 3} fontSize={10.5} textAnchor="end" style={{ fill: 'var(--fg)' }}>
              {short(n.text, 18)}
            </text>
          </g>
        );
      })}
    </svg>
  );
}
