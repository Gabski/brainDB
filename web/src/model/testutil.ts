// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

// Builders for the unit tests: a minimal State / GraphIndex without touching the API or fixtures.
import type { BrainNode, Edge, EntityRow, Kind, State } from '../api/types';
import { buildIndex, DEFAULT_KINDS, type GraphIndex } from './graph';

export function node(id: string, over: Partial<BrainNode> = {}): BrainNode {
  return {
    id,
    kind: 'fact',
    section: 'facts',
    text: 'text of ' + id,
    base: 0.6,
    weight: 0.5,
    age: 3,
    pinned: false,
    ...over,
  };
}

export function state(nodes: BrainNode[], edges: Edge[] = [], over: Partial<State> = {}): State {
  return {
    name: 'test',
    goal: '',
    step: 1,
    params: { depth: 2, threshold: 0.1, hop: 0.6, showSuperseded: false },
    embedding: { model: 'hash-256', dim: 256, alpha: 0.5, hops: 2, use: 'enriched', lambda: 0.5, coherenceRaw: 0, coherenceEnriched: 0 },
    kinds: DEFAULT_KINDS as Kind[],
    sections: ['facts', 'decisions'],
    focus: [],
    goalFocus: [],
    nodes,
    edges,
    stats: { lit: 0, memories: nodes.length, concepts: 0, tokens: 0, reach: 0, avgWeight: 0.5, superseded: 0, newLinks: 0, byDepth: [] },
    status: 'ok',
    ...over,
  };
}

export const index = (nodes: BrainNode[], edges: Edge[] = [], over: Partial<State> = {}): GraphIndex => buildIndex(state(nodes, edges, over));

export const entityRow = (nodeId: string, type: string, value: string): EntityRow => ({ node: nodeId, type, value, raw: value, confidence: 0.9 });
