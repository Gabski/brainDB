// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

// Flat map for the 2D graph view. Concepts sit on a header row; each memory
// sits under the concept it links to most strongly, the same reading as the
// website example. Memories with no concept line up in section columns after that.

import type { BrainNode, Edge } from '../api/types';
import { isConceptNode } from './graph';

export interface Pt {
  x: number;
  y: number;
}

export interface MapCaption {
  text: string;
  x: number;
  y: number;
}

export interface MapLayout {
  pos: Map<string, Pt>;
  /** Memory id → concept it was placed under. */
  home: Map<string, string>;
  captions: MapCaption[];
  /** Faded nodes, packed under the reading so they do not stretch it. */
  shelf: Set<string>;
}

const COL = 156;
const ROW = 112;
const TOP = 48;
const PER = 7;
const GAP = 28;
const ROW_WRAP = 980;

const KIND_ORDER: Record<string, number> = {
  decision: 0,
  summary: 1,
  fact: 2,
  file: 3,
  record: 4,
  chunk: 5,
  task: 6,
  question: 7,
  idea: 8,
  note: 9,
};

function byRank(byId: Map<string, BrainNode>) {
  return (a: string, b: string) => {
    const na = byId.get(a);
    const nb = byId.get(b);
    const ka = KIND_ORDER[na?.kind ?? ''] ?? 50;
    const kb = KIND_ORDER[nb?.kind ?? ''] ?? 50;
    if (ka !== kb) return ka - kb;
    const wa = na?.weight ?? 0;
    const wb = nb?.weight ?? 0;
    if (wa !== wb) return wb - wa;
    return a < b ? -1 : a > b ? 1 : 0;
  };
}

/** Column layout. Deterministic: no randomness, stable under a reshuffle of equal-weight edges.
 *  `featured` is the lit context: those memories form the reading, and the rest sit on a shelf. */
export function layoutMap(nodes: BrainNode[], edges: Edge[], conceptIds: string[], featured?: ReadonlySet<string>): MapLayout {
  const byId = new Map(nodes.map((n) => [n.id, n]));
  const seen = new Set<string>();
  const concepts: string[] = [];
  for (const id of conceptIds) {
    const n = byId.get(id);
    if (n && isConceptNode(n) && !seen.has(id)) {
      seen.add(id);
      concepts.push(id);
    }
  }
  for (const n of nodes) {
    if (isConceptNode(n) && !seen.has(n.id)) {
      seen.add(n.id);
      concepts.push(n.id);
    }
  }

  const homeW = new Map<string, { id: string; w: number }>();
  for (const e of edges) {
    const a = byId.get(e.a);
    const b = byId.get(e.b);
    if (!a || !b) continue;
    const aC = isConceptNode(a);
    const bC = isConceptNode(b);
    if (aC === bC) continue;
    const mem = aC ? e.b : e.a;
    const con = aC ? e.a : e.b;
    const prev = homeW.get(mem);
    if (!prev || e.w > prev.w || (e.w === prev.w && con < prev.id)) homeW.set(mem, { id: con, w: e.w });
  }

  const lit = featured && featured.size > 0 ? featured : null;
  const columns = new Map<string, string[]>();
  for (const id of concepts) columns.set(id, []);
  const loose = new Map<string, string[]>();
  const shelfIds: string[] = [];
  const cmp = byRank(byId);
  for (const n of nodes) {
    if (isConceptNode(n)) continue;
    if (lit && !lit.has(n.id)) {
      shelfIds.push(n.id);
      continue;
    }
    const h = homeW.get(n.id)?.id;
    if (h && columns.has(h)) columns.get(h)!.push(n.id);
    else {
      const sec = n.section || 'other';
      if (!loose.has(sec)) loose.set(sec, []);
      loose.get(sec)!.push(n.id);
    }
  }
  for (const list of columns.values()) list.sort(cmp);
  for (const list of loose.values()) list.sort(cmp);
  shelfIds.sort(cmp);

  const mainConcepts = concepts.filter((id) => !lit || lit.has(id) || (columns.get(id)?.length ?? 0) > 0);
  if (lit) {
    for (const id of concepts) if (!mainConcepts.includes(id)) shelfIds.push(id);
  }

  const pos = new Map<string, Pt>();
  const captions: MapCaption[] = [];
  type Cluster = { header: string | null; caption?: string; ids: string[] };
  const clusters: Cluster[] = [];
  for (const id of mainConcepts) clusters.push({ header: id, ids: columns.get(id) ?? [] });
  if (mainConcepts.length && loose.size) clusters.push({ header: null, ids: [] });
  for (const sec of [...loose.keys()].sort()) clusters.push({ header: null, caption: sec, ids: loose.get(sec)! });

  let x = 0;
  let y = 0;
  let rowH = 0;
  const place = (cl: Cluster) => {
    if (!cl.header && !cl.caption && cl.ids.length === 0) {
      x += GAP;
      return;
    }
    const span = Math.max(1, Math.ceil(cl.ids.length / PER));
    const width = span * COL;
    if (x > 0 && x + width > ROW_WRAP) {
      x = 0;
      y += rowH + GAP;
      rowH = 0;
    }
    const headerY = y + TOP;
    if (cl.header) pos.set(cl.header, { x: x + width / 2, y: headerY });
    if (cl.caption) captions.push({ text: cl.caption, x: x + width / 2, y: headerY });
    cl.ids.forEach((id, i) => {
      const c = Math.floor(i / PER);
      const r = i % PER;
      pos.set(id, { x: x + c * COL + COL / 2, y: headerY + ROW * (r + 1) });
    });
    const rows = cl.ids.length === 0 ? 0 : Math.min(PER, cl.ids.length);
    const bottom = cl.ids.length ? headerY + rows * ROW : headerY;
    rowH = Math.max(rowH, bottom - y);
    x += width + GAP;
  };
  for (const cl of clusters) place(cl);

  if (shelfIds.length) {
    const shelfTop = y + rowH + ROW;
    const per = 18;
    shelfIds.forEach((id, i) => {
      pos.set(id, { x: (i % per) * 36 + 18, y: shelfTop + Math.floor(i / per) * 28 });
    });
  }

  const home = new Map<string, string>();
  for (const mem of [...homeW.keys()].sort()) {
    const h = homeW.get(mem)!;
    if (columns.has(h.id)) home.set(mem, h.id);
  }
  return { pos, home, captions, shelf: new Set(shelfIds) };
}

/** Where a structural connector sits: the centroid of its members, nudged off any node. */
export function layoutConnectors(ids: string[], edges: Edge[], pos: Map<string, Pt>): Map<string, Pt> {
  const want = new Set(ids);
  const members = new Map<string, string[]>();
  for (const e of edges) {
    if (e.type !== 'connector') continue;
    const h = want.has(e.a) ? e.a : want.has(e.b) ? e.b : '';
    if (!h) continue;
    const m = h === e.a ? e.b : e.a;
    if (!pos.has(m)) continue;
    if (!members.has(h)) members.set(h, []);
    members.get(h)!.push(m);
  }
  let maxY = TOP;
  for (const p of pos.values()) maxY = Math.max(maxY, p.y);
  const taken: Pt[] = [...pos.values()];
  const free = (pt: Pt): Pt => {
    let p = pt;
    for (let i = 0; i < 8; i++) {
      const hit = taken.some((q) => Math.hypot(q.x - p.x, q.y - p.y) < 48);
      if (!hit) break;
      p = { x: p.x + 36, y: p.y + 24 };
    }
    taken.push(p);
    return p;
  };
  const out = new Map<string, Pt>();
  let orphan = 0;
  for (const id of [...ids].sort()) {
    const ms = members.get(id) ?? [];
    if (!ms.length) {
      out.set(id, free({ x: orphan * COL + COL / 2, y: maxY + ROW }));
      orphan++;
      continue;
    }
    let sx = 0;
    let sy = 0;
    for (const m of ms) {
      const p = pos.get(m)!;
      sx += p.x;
      sy += p.y;
    }
    out.set(id, free({ x: sx / ms.length, y: sy / ms.length }));
  }
  return out;
}

/** Edges worth drawing on the flat map. A large brain keeps the lit, home and path edges. */
export function edgesForMap(
  edges: Edge[],
  home: Map<string, string>,
  lit: Set<string>,
  mode: 'all' | 'ctx' | 'none',
  path: Set<string>,
): Edge[] {
  const homeEdge = (e: Edge) => home.get(e.a) === e.b || home.get(e.b) === e.a;
  const bothLit = (e: Edge) => lit.has(e.a) && lit.has(e.b);
  const onPath = (e: Edge) => path.has(e.a < e.b ? e.a + '|' + e.b : e.b + '|' + e.a);
  if (mode === 'none') return edges.filter(onPath);
  const keep = (e: Edge) => onPath(e) || bothLit(e) || homeEdge(e) || e.type === 'connector' || e.type === 'new';
  if (mode === 'ctx') return edges.filter((e) => onPath(e) || bothLit(e) || (e.type === 'connector' && (lit.has(e.a) || lit.has(e.b))));
  if (edges.length <= 400) return edges;
  return edges.filter((e) => keep(e) || e.w >= 0.75);
}
