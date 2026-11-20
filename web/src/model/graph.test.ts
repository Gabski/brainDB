// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from 'vitest';
import type { Edge, Kind } from '../api/types';
import { buildIndex, DEFAULT_KINDS, depthColor, DEPTH_COLORS, edgeKey, EDGE_GLYPH, isConceptNode, isLit, otherEnd } from './graph';
import { node, state } from './testutil';

const edge = (a: string, b: string, over: Partial<Edge> = {}): Edge => ({ a, b, w: 0.5, type: 'assoc', ...over });

describe('isConceptNode', () => {
  it('accepts the concept kind and the A: prefix', () => {
    expect(isConceptNode({ id: 'A:idp', kind: 'fact' })).toBe(true);
    expect(isConceptNode({ id: 'x1', kind: 'concept' })).toBe(true);
    expect(isConceptNode({ id: 'f201', kind: 'fact' })).toBe(false);
  });
});

describe('isLit', () => {
  it('is true only with an activation', () => {
    expect(isLit(node('a', { activation: 0 }))).toBe(true);
    expect(isLit(node('a'))).toBe(false);
    expect(isLit(undefined)).toBe(false);
  });
});

describe('edgeKey', () => {
  it('is order-independent', () => {
    expect(edgeKey('a', 'b')).toBe(edgeKey('b', 'a'));
    expect(edgeKey('a', 'b')).toBe('a|b');
  });
});

describe('otherEnd', () => {
  it('returns the far end of an edge', () => {
    expect(otherEnd(edge('a', 'b'), 'a')).toBe('b');
    expect(otherEnd(edge('a', 'b'), 'b')).toBe('a');
  });
});

describe('depthColor', () => {
  it('clamps below 0 and above the palette', () => {
    expect(depthColor(0)).toBe(DEPTH_COLORS[0]);
    expect(depthColor(undefined)).toBe(DEPTH_COLORS[0]);
    expect(depthColor(-5)).toBe(DEPTH_COLORS[0]);
    expect(depthColor(99)).toBe(DEPTH_COLORS[6]);
  });
});

describe('buildIndex', () => {
  const nodes = [node('f201'), node('d121', { kind: 'decision' }), node('A:idp', { kind: 'concept', text: 'identity provider' })];
  const edges = [edge('f201', 'd121'), edge('f201', 'A:idp', { type: 'concept' }), edge('f201', 'ghost')];
  const g = buildIndex(state(nodes, edges, { focus: ['f201'] }));

  it('indexes nodes by id', () => {
    expect(g.nodes.get('d121')?.kind).toBe('decision');
    expect(g.nodes.get('nope')).toBeUndefined();
  });
  it('builds adjacency both ways and skips edges to unknown nodes', () => {
    expect(g.adj.get('f201')?.length).toBe(2);
    expect(g.adj.get('d121')?.length).toBe(1);
    expect(g.adj.get('ghost')).toBeUndefined();
  });
  it('collects concept ids', () => {
    expect(g.conceptIds).toEqual(['A:idp']);
  });
  it('exposes the focus as a set', () => {
    expect(g.focus.has('f201')).toBe(true);
    expect(g.focus.has('d121')).toBe(false);
  });
  it('falls back to a placeholder kind for an unknown name', () => {
    expect(g.kind('fact').color).toBe(DEFAULT_KINDS.find((k) => k.name === 'fact')!.color);
    expect(g.kind('nonexistent').name).toBe('?');
  });
  it('lets the state override a built-in kind', () => {
    const custom: Kind = { name: 'fact', label: 'FACT', color: '#123456', base: 0.5, halfLife: 10, prefix: 'f', section: 'facts', builtin: false };
    const g2 = buildIndex(state(nodes, [], { kinds: [custom] }));
    expect(g2.kind('fact').color).toBe('#123456');
    expect(g2.color('f201')).toBe('#123456');
  });
  it('colours an unknown id with the neutral grey', () => {
    expect(g.color('ghost')).toBe('#555');
  });
  it('labels concepts with a diamond and unknown ids with the id itself', () => {
    expect(g.label('A:idp')).toBe('◆ identity provider');
    expect(g.label('f201')).toBe('text of f201');
    expect(g.label('ghost')).toBe('ghost');
  });
});

describe('EDGE_GLYPH', () => {
  it('has a glyph for every edge type the UI renders', () => {
    for (const t of ['concept', 'supersede', 'new', 'similar', 'part', 'seq', 'assoc']) expect(EDGE_GLYPH[t]).toBeTruthy();
  });
});
