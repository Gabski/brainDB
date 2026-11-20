// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { BrainNode, Edge, Kind, State } from '../api/types';

export const DEPTH_COLORS = ['#fff3bf', '#ffa94d', '#f06595', '#9775fa', '#4dabf7', '#38d9a9', '#69db7c'];
export const depthColor = (d: number | undefined) => DEPTH_COLORS[Math.min(Math.max(d ?? 0, 0), 6)];

/** Fallback kinds (used when a State lacks one, e.g. an older server). */
export const DEFAULT_KINDS: Kind[] = [
  ['fact', '#4ea8ff', 0.6, 30, 'f', 'facts'],
  ['decision', '#ffa94d', 0.7, 60, 'd', 'decisions'],
  ['task', '#69db7c', 0.55, 14, 't', 'tasks'],
  ['question', '#da77f2', 0.5, 7, 'q', 'questions'],
  ['note', '#adb5bd', 0.4, 2, 'n', 'notes'],
  ['summary', '#38d9a9', 0.6, 30, 's', 'procedures'],
  ['file', '#f783ac', 0.5, 14, 'x', 'files'],
  ['idea', '#ffe066', 0.5, 7, 'i', 'ideas'],
  ['chunk', '#94d82d', 0.45, 21, 'c', 'files'],
  ['record', '#91a7ff', 0.5, 30, 'r', 'records'],
  ['concept', '#e7f5ff', 0.8, null, 'A:', 'concepts'],
].map(([name, color, base, halfLife, prefix, section]) => ({
  name: name as string,
  label: name as string,
  color: color as string,
  base: base as number,
  halfLife: halfLife as number | null,
  prefix: prefix as string,
  section: section as string,
  builtin: true,
}));

export const isConceptNode = (n: Pick<BrainNode, 'id' | 'kind'>) => n.kind === 'concept' || n.id.startsWith('A:');
export const isLit = (n: BrainNode | undefined) => !!n && n.activation !== undefined && n.activation !== null;
export const edgeKey = (a: string, b: string) => (a < b ? a + '|' + b : b + '|' + a);
export const otherEnd = (e: Edge, u: string) => (e.a === u ? e.b : e.a);

/** Read-only index over a State snapshot: lookups the UI needs for rendering. */
export interface GraphIndex {
  state: State;
  nodes: Map<string, BrainNode>;
  adj: Map<string, Edge[]>;
  kinds: Map<string, Kind>;
  conceptIds: string[];
  focus: Set<string>;
  kind(name: string): Kind;
  color(id: string): string;
  label(id: string): string;
}

export function buildIndex(state: State): GraphIndex {
  const nodes = new Map(state.nodes.map((n) => [n.id, n]));
  const adj = new Map<string, Edge[]>();
  for (const n of state.nodes) adj.set(n.id, []);
  for (const e of state.edges) {
    if (!adj.has(e.a) || !adj.has(e.b)) continue;
    adj.get(e.a)!.push(e);
    adj.get(e.b)!.push(e);
  }
  const kinds = new Map(DEFAULT_KINDS.map((k) => [k.name, k]));
  for (const k of state.kinds) kinds.set(k.name, k);
  const fallback: Kind = { ...DEFAULT_KINDS[4], name: '?', label: '?' };
  const kind = (name: string) => kinds.get(name) ?? fallback;
  return {
    state,
    nodes,
    adj,
    kinds,
    conceptIds: state.nodes.filter(isConceptNode).map((n) => n.id),
    focus: new Set(state.focus),
    kind,
    color: (id) => {
      const n = nodes.get(id);
      return n ? kind(n.kind).color : '#555';
    },
    label: (id) => {
      const n = nodes.get(id);
      if (!n) return id;
      return isConceptNode(n) ? '◆ ' + n.text : n.text;
    },
  };
}

export const EDGE_GLYPH: Record<string, string> = {
  concept: '◆',
  supersede: '⇄',
  new: '+',
  similar: '~',
  part: '▸',
  seq: '→',
  assoc: '·',
};
