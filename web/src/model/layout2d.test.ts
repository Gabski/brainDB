// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from 'vitest';
import type { Edge } from '../api/types';
import { node } from './testutil';
import { edgesForMap, layoutConnectors, layoutMap } from './layout2d';

const edge = (a: string, b: string, over: Partial<Edge> = {}): Edge => ({ a, b, w: 0.5, type: 'assoc', ...over });

describe('layoutMap', () => {
  it('puts a concept above the memories that link to it, in one column', () => {
    const nodes = [
      node('A:pricing', { kind: 'concept', section: 'concepts', text: 'Pricing' }),
      node('f101', { weight: 0.4 }),
      node('d121', { kind: 'decision', section: 'decisions', weight: 0.9 }),
    ];
    const edges = [edge('f101', 'A:pricing', { w: 0.8, type: 'concept' }), edge('d121', 'A:pricing', { w: 0.9, type: 'concept' })];
    const { pos } = layoutMap(nodes, edges, ['A:pricing']);
    const concept = pos.get('A:pricing')!;
    expect(concept.y).toBeLessThan(pos.get('f101')!.y);
    expect(concept.y).toBeLessThan(pos.get('d121')!.y);
    expect(pos.get('f101')!.x).toBe(concept.x);
    expect(pos.get('d121')!.x).toBe(concept.x);
    expect(pos.get('d121')!.y).toBeLessThan(pos.get('f101')!.y);
  });

  it('sends a memory to its strongest concept', () => {
    const nodes = [
      node('A:a', { kind: 'concept', section: 'concepts' }),
      node('A:b', { kind: 'concept', section: 'concepts' }),
      node('f101'),
    ];
    const edges = [edge('f101', 'A:a', { w: 0.4, type: 'concept' }), edge('f101', 'A:b', { w: 0.9, type: 'concept' })];
    const { pos, home } = layoutMap(nodes, edges, ['A:a', 'A:b']);
    expect(home.get('f101')).toBe('A:b');
    expect(pos.get('f101')!.x).toBe(pos.get('A:b')!.x);
    expect(pos.get('A:a')!.x).toBeLessThan(pos.get('A:b')!.x);
  });

  it('keeps the conceptIds order', () => {
    const nodes = [node('A:a', { kind: 'concept' }), node('A:b', { kind: 'concept' })];
    const { pos } = layoutMap(nodes, [], ['A:b', 'A:a']);
    expect(pos.get('A:b')!.x).toBeLessThan(pos.get('A:a')!.x);
  });

  it('places memories with no concept in section columns to the right', () => {
    const nodes = [
      node('A:pricing', { kind: 'concept', section: 'concepts' }),
      node('f101', { section: 'facts' }),
      node('n150', { kind: 'note', section: 'notes' }),
    ];
    const { pos, captions } = layoutMap(nodes, [], ['A:pricing']);
    expect(pos.get('A:pricing')!.x).toBeLessThan(pos.get('f101')!.x);
    expect(pos.get('f101')!.x).toBeLessThan(pos.get('n150')!.x);
    expect(captions.map((c) => c.text)).toEqual(['facts', 'notes']);
  });

  it('wraps a long column so two nodes never share a point', () => {
    const nodes = [node('A:pricing', { kind: 'concept' }), ...Array.from({ length: 8 }, (_, i) => node('f' + i, { weight: 1 - i / 10 }))];
    const edges = nodes.slice(1).map((n) => edge(n.id, 'A:pricing', { w: 0.6, type: 'concept' }));
    const { pos } = layoutMap(nodes, edges, ['A:pricing']);
    expect(pos.size).toBe(nodes.length);
    const keys = [...pos.values()].map((p) => `${p.x},${p.y}`);
    expect(new Set(keys).size).toBe(nodes.length);
    expect(pos.get('f7')!.x).toBeGreaterThan(pos.get('f0')!.x);
    for (const p of pos.values()) {
      expect(Number.isFinite(p.x)).toBe(true);
      expect(Number.isFinite(p.y)).toBe(true);
    }
  });

  it('packs the lit context and shelves the rest', () => {
    const nodes = [
      node('A:pricing', { kind: 'concept', section: 'concepts' }),
      node('A:other', { kind: 'concept', section: 'concepts' }),
      node('f101'),
      node('n150', { kind: 'note', section: 'notes' }),
    ];
    const edges = [edge('f101', 'A:pricing', { w: 0.8, type: 'concept' }), edge('n150', 'A:other', { w: 0.8, type: 'concept' })];
    const { pos, shelf } = layoutMap(nodes, edges, ['A:pricing', 'A:other'], new Set(['f101']));
    expect(shelf.has('n150')).toBe(true);
    expect(shelf.has('A:other')).toBe(true);
    expect(shelf.has('f101')).toBe(false);
    expect(pos.get('n150')!.y).toBeGreaterThan(pos.get('f101')!.y + 40);
    expect(pos.get('f101')!.x).toBe(pos.get('A:pricing')!.x);
  });

  it('is deterministic', () => {
    const nodes = [node('A:pricing', { kind: 'concept' }), node('f101'), node('d121', { kind: 'decision', section: 'decisions' })];
    const edges = [edge('d121', 'A:pricing', { w: 0.5, type: 'concept' }), edge('f101', 'A:pricing', { w: 0.5, type: 'concept' })];
    const a = layoutMap(nodes, edges, ['A:pricing']);
    const b = layoutMap([...nodes].reverse(), [...edges].reverse(), ['A:pricing']);
    expect([...a.pos.entries()]).toEqual([...b.pos.entries()]);
    expect([...a.home.entries()]).toEqual([...b.home.entries()]);
  });
});

describe('layoutConnectors', () => {
  it('parks a connector on the centroid of its members', () => {
    const nodes = [node('f101'), node('d121', { kind: 'decision', section: 'decisions' })];
    const { pos } = layoutMap(nodes, [], []);
    const edges = [edge('H:q4', 'f101', { type: 'connector', w: 0.8 }), edge('d121', 'H:q4', { type: 'connector', w: 0.8 })];
    const c = layoutConnectors(['H:q4'], edges, pos);
    const p = c.get('H:q4')!;
    const midX = (pos.get('f101')!.x + pos.get('d121')!.x) / 2;
    expect(p.x).toBeCloseTo(midX, 5);
  });
});

describe('edgesForMap', () => {
  const edges = [
    edge('f101', 'd121', { w: 0.4 }),
    edge('f101', 'A:pricing', { w: 0.8, type: 'concept' }),
    edge('n150', 'q140', { w: 0.2 }),
  ];
  const home = new Map([['f101', 'A:pricing']]);
  const path = new Set(['n150|q140']);

  it('keeps only the path when edges are hidden', () => {
    expect(edgesForMap(edges, home, new Set(), 'none', path).map((e) => e.a + e.b)).toEqual(['n150q140']);
  });

  it('keeps lit pairs in context mode and drops the faded edge', () => {
    const got = edgesForMap(edges, home, new Set(['f101', 'd121']), 'ctx', new Set());
    expect(got.map((e) => e.a)).toEqual(['f101']);
  });

  it('drops weak faded edges once the graph is large', () => {
    const many = [...edges, ...Array.from({ length: 400 }, (_, i) => edge('a' + i, 'b' + i, { w: 0.2 }))];
    const got = edgesForMap(many, home, new Set(['f101', 'd121']), 'all', path);
    const ids = got.map((e) => e.a + '|' + e.b);
    expect(ids).toContain('f101|d121');
    expect(ids).toContain('f101|A:pricing');
    expect(ids).toContain('n150|q140');
    expect(ids).not.toContain('a0|b0');
  });
});
