// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

// Client-side layouts for the Brain / Layers / Depth sphere / Decay views (layoutPos, sphereCtx, inBrain, LOBES).
// Vectors and Tree positions come from GET /api/layout.
import type { BrainNode, Vec3 } from '../api/types';
import { isConceptNode } from './graph';
import { gauss, hash, mulberry32 } from './util';

export type ViewName = 'brain' | 'layers' | 'sphere' | 'time' | 'embed' | 'tree' | 'map';

export const VIEWS: { id: ViewName; label: string }[] = [
  { id: 'brain', label: 'Brain' },
  { id: 'layers', label: 'Layers' },
  { id: 'sphere', label: 'Depth sphere' },
  { id: 'time', label: 'Decay' },
  { id: 'embed', label: 'Vectors' },
  { id: 'tree', label: 'Tree' },
  { id: 'map', label: '2D' },
];

export const SECTIONS = ['facts', 'decisions', 'tasks', 'questions', 'notes', 'procedures', 'ideas', 'files'];

export const LOBES: Record<string, Vec3> = {
  facts: [-38, 18, 38],
  decisions: [38, 22, 40],
  tasks: [-42, -6, -18],
  questions: [42, -12, 4],
  notes: [38, 8, -50],
  procedures: [-30, 30, -52],
  ideas: [30, 38, -10],
  files: [-34, -24, 8],
};

/** Lobe centre of a section; unknown sections get a deterministic hashed lobe. */
export function lobeOf(section: string): Vec3 {
  const l = LOBES[section];
  if (l) return l;
  const r = mulberry32(hash(section));
  return [(r() < 0.5 ? -1 : 1) * (20 + r() * 25), (r() - 0.5) * 50, (r() - 0.5) * 100];
}

/** Clamp a point into the hemisphere shell of side `s` (-1 / +1). */
export function inBrain(p: Vec3, s: number): Vec3 {
  const lx = (Math.abs(p[0]) - 3) / 64,
    ly = p[1] / (p[1] < 0 ? 44 : 58),
    lz = p[2] / 90;
  const r = Math.hypot(lx, ly, lz),
    k = r > 0.84 ? 0.84 / r : 1;
  return [s * (3 + Math.max(0, lx * k) * 64), ly * k * (p[1] < 0 ? 44 : 58), lz * k * 90];
}

export const LEVEL: Record<string, number> = {
  chunk: -15,
  record: -15,
  concept: 80,
  decision: 40,
  summary: 40,
  fact: 0,
  file: 0,
  task: -40,
  question: -40,
  idea: -40,
  note: -80,
};

export const timeX = (age: number) => 100 - 200 * (Math.log1p(Math.min(age, 320)) / Math.log1p(320));

const fib = (i: number, N: number, R: number): Vec3 => {
  const y = 1 - (2 * (i + 0.5)) / N,
    rr = Math.sqrt(Math.max(0, 1 - y * y)),
    ph = i * 2.39996;
  return [Math.cos(ph) * rr * R, y * R, Math.sin(ph) * rr * R];
};

/** Position of one node in a client-side view. `conceptIds` keeps concept order stable. */
export function layoutPos(n: BrainNode, view: ViewName, conceptIds: string[], ctx?: Map<string, Vec3>): Vec3 {
  const r = mulberry32(hash(n.id));
  const concept = isConceptNode(n);
  if (view === 'brain') {
    if (concept) {
      const i = Math.max(0, conceptIds.indexOf(n.id));
      const p = fib(i, Math.max(1, conceptIds.length), 1);
      return [p[0] * 12, p[1] * 22, p[2] * 30];
    }
    const L = lobeOf(n.section),
      s = Math.sign(L[0]) || 1;
    const spread = 1 + (Math.min(n.age, 300) / 300) * 0.6;
    return inBrain([L[0] + gauss(r) * 13 * spread, L[1] + gauss(r) * 12 * spread, L[2] + gauss(r) * 16 * spread], s);
  }
  if (view === 'layers') {
    if (concept) {
      const i = Math.max(0, conceptIds.indexOf(n.id)),
        a = (i / Math.max(1, conceptIds.length)) * Math.PI * 2;
      return [Math.cos(a) * 70, 80, Math.sin(a) * 70];
    }
    const si = SECTIONS.indexOf(n.section),
      a = ((si < 0 ? hash(n.section) % 8 : si) / 8) * Math.PI * 2 + (r() - 0.5) * 0.65,
      rad = 30 + r() * 80;
    return [Math.cos(a) * rad, (LEVEL[n.kind] ?? 0) + (r() - 0.5) * 8, Math.sin(a) * rad];
  }
  if (view === 'time') {
    if (concept) {
      const i = Math.max(0, conceptIds.indexOf(n.id));
      return [120, 50 - i * 5.6, 0];
    }
    const si = SECTIONS.indexOf(n.section);
    return [timeX(n.age) + (r() - 0.5) * 4, -45 + n.weight * 95, ((si < 0 ? 4 : si) - 3.5) * 14 + (r() - 0.5) * 8];
  }
  return ctx?.get(n.id) ?? [0, 0, 0];
}

/** Depth sphere: focus in the centre, one shell per activation depth, faded memory outside. */
export function sphereCtx(nodes: BrainNode[]): Map<string, Vec3> {
  const out = new Map<string, Vec3>();
  const byD = new Map<number, BrainNode[]>();
  const rest: BrainNode[] = [];
  for (const n of nodes) {
    if (n.activation === undefined || n.activation === null) {
      rest.push(n);
      continue;
    }
    const d = n.depth ?? 0;
    if (!byD.has(d)) byD.set(d, []);
    byD.get(d)!.push(n);
  }
  for (const [d, list] of byD) {
    list.sort((a, b) => (b.activation ?? 0) - (a.activation ?? 0));
    list.forEach((n, i) => out.set(n.id, fib(i, list.length, d === 0 ? (list.length > 1 ? 9 : 0) : d * 30)));
  }
  rest.forEach((n, i) => {
    const p = fib(i, rest.length, 1),
      R = 165 + (1 - n.weight) * 45;
    out.set(n.id, [p[0] * R, p[1] * R, p[2] * R]);
  });
  return out;
}

/** All positions for a view. `server` holds positions from /api/layout for embed/tree. */
export function computeLayout(
  nodes: BrainNode[],
  view: ViewName,
  conceptIds: string[],
  server?: Record<string, Vec3>,
): Map<string, Vec3> {
  let ctx: Map<string, Vec3> | undefined;
  if (view === 'sphere') ctx = sphereCtx(nodes);
  else if (view === 'embed' || view === 'tree') {
    ctx = new Map(Object.entries(server ?? {}));
    // nodes missing from the server layout park on an outer ring
    const missing = nodes.filter((n) => !ctx!.has(n.id));
    missing.forEach((n, i) => {
      const a = (i / Math.max(1, missing.length)) * Math.PI * 2;
      ctx!.set(n.id, [Math.cos(a) * 170, -128, Math.sin(a) * 170]);
    });
  }
  const out = new Map<string, Vec3>();
  for (const n of nodes) out.set(n.id, layoutPos(n, view, conceptIds, ctx));
  return out;
}

export const VIEW_DESC: Record<ViewName, string> = {
  brain:
    'Brain: all memory in lobes (one per section), concepts in the core. Only the current context is lit; the rest is faded memory, and superseded items are almost invisible.',
  layers:
    'Layers: knowledge level, top to bottom: concepts → decisions → facts → tasks → notes. Shows how context passes through concepts.',
  sphere:
    'Depth sphere: focus in the centre, each shell is one more activation hop. Memory outside the context sits on the outer sphere.',
  time: 'Decay: X = age (log), Y = weight = base × decay. Old and superseded memories sink to the bottom left.',
  embed:
    'Vectors: 3D PCA of the embeddings (from the server). Enriched vectors pull related knowledge into clusters; switch raw/enriched in the Embedding panel. Teal edges are vector similarity.',
  tree: 'Tree: maximum spanning tree from the root (first focus item or "Tree root"), i.e. the strongest route to every memory. Files expand into chunks.',
  map: '2D: the same scene, flattened. Concepts sit on a top row and each memory under the concept it links to most strongly. Drag pans, the wheel zooms, and the view does not orbit.',
};
