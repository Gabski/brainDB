// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from 'vitest';
import type { SearchResult, SearchRow } from '../api/types';
import { EMPTY_NARROW, groupRows, isNarrowed, narrowResult, narrowRows, narrowTerms, resultFacets, rowSource, type ResultNarrow } from './searchView';
import { index, node } from './testutil';

const g = index([
  node('f201', { kind: 'fact', section: 'facts', text: 'Entra login rate limit is 30 per minute' }),
  node('d121', { kind: 'decision', section: 'decisions', text: 'We approve app changes in the platform board' }),
  node('t132', { kind: 'task', section: 'tasks', text: 'Raise the login limit with support' }),
  node('n154', { kind: 'note', section: 'notes', text: 'A note about something else' }),
  node('lab:x204', { kind: 'fact', section: 'facts', text: 'External fact', source: 'lab', external: true }),
]);

const row = (id: string, over: Partial<SearchRow> = {}): SearchRow => ({ id, score: 0.5, ...over });
const n = (over: Partial<ResultNarrow> = {}): ResultNarrow => ({ ...EMPTY_NARROW, ...over });

describe('rowSource', () => {
  it('prefers the row source, then the node, then local', () => {
    expect(rowSource(g, row('f201'))).toBe('local');
    expect(rowSource(g, row('lab:x204'))).toBe('lab');
    expect(rowSource(g, row('f201', { source: 'crm' }))).toBe('crm');
  });
});

describe('narrowRows', () => {
  const rows = [row('f201'), row('d121'), row('t132'), row('lab:x204')];

  it('keeps everything when nothing is narrowed', () => {
    expect(narrowRows(g, rows, n()).length).toBe(4);
  });
  it('ANDs the free-text terms over id, text, kind, section and source', () => {
    expect(narrowRows(g, rows, n({ text: 'login' })).map((r) => r.id)).toEqual(['f201', 't132']);
    expect(narrowRows(g, rows, n({ text: 'login board' })).map((r) => r.id)).toEqual([]);
  });
  it('filters by kind, section and source', () => {
    expect(narrowRows(g, rows, n({ kinds: ['fact'] })).map((r) => r.id)).toEqual(['f201', 'lab:x204']);
    expect(narrowRows(g, rows, n({ sections: ['tasks'] })).map((r) => r.id)).toEqual(['t132']);
    expect(narrowRows(g, rows, n({ sources: ['lab'] })).map((r) => r.id)).toEqual(['lab:x204']);
  });
  it('matches a row whose node is not in the snapshot by id only', () => {
    expect(narrowRows(g, [row('zz9')], n({ text: 'zz9' })).map((r) => r.id)).toEqual(['zz9']);
    expect(narrowRows(g, [row('zz9')], n({ text: 'login' }))).toEqual([]);
  });

  it('re-parents a kept row to the nearest kept ancestor in tree mode', () => {
    const tree = [row('f201', { parent: null }), row('d121', { parent: 'f201' }), row('t132', { parent: 'd121' })];
    // 'd121' (the decision) is narrowed away; its child must hang off f201, not vanish
    const out = narrowRows(g, tree, n({ kinds: ['fact', 'task'] }), true);
    expect(out.map((r) => r.id)).toEqual(['f201', 't132']);
    expect(out.find((r) => r.id === 't132')?.parent).toBe('f201');
  });
  it('makes a row a root when no ancestor survives', () => {
    const tree = [row('d121', { parent: null }), row('t132', { parent: 'd121' })];
    const out = narrowRows(g, tree, n({ kinds: ['task'] }), true);
    expect(out).toEqual([{ ...row('t132', { parent: null }) }]);
  });
  it('does not loop on a cyclic parent chain', () => {
    const tree = [row('d121', { parent: 't132' }), row('t132', { parent: 'd121' })];
    const out = narrowRows(g, tree, n({ kinds: ['fact'] }), true);
    expect(out).toEqual([]);
  });
});

describe('narrowResult', () => {
  const result: SearchResult = { mode: 'graph', rows: [row('f201'), row('d121')], via: [row('t132'), row('n154')], roots: ['f201', 'd121'] };

  it('returns the same object when nothing is narrowed', () => {
    expect(narrowResult(g, result, n())).toBe(result);
  });
  it('narrows rows and via, and drops roots that went away', () => {
    const out = narrowResult(g, result, n({ text: 'login' }));
    expect(out.rows.map((r) => r.id)).toEqual(['f201']);
    expect(out.via?.map((r) => r.id)).toEqual(['t132']);
    expect(out.roots).toEqual(['f201']);
  });
});

describe('resultFacets', () => {
  const result: SearchResult = { mode: 'hybrid', rows: [row('f201'), row('d121'), row('lab:x204')], via: [row('t132')] };

  it('counts over rows and via together', () => {
    const f = resultFacets(g, result, n());
    expect(f.kinds.find((x) => x.value === 'fact')?.count).toBe(2);
    expect(f.sources.map((x) => x.value).sort()).toEqual(['lab', 'local']);
  });
  it('counts a group as if that group were not picked', () => {
    const f = resultFacets(g, result, n({ kinds: ['fact'] }));
    expect(f.kinds.find((x) => x.value === 'decision')?.count).toBe(1);
    expect(f.sections.find((x) => x.value === 'decisions')).toBeUndefined();
  });
  it('sorts facets by count, then by value', () => {
    expect(resultFacets(g, result, n()).kinds[0].value).toBe('fact');
  });
});

describe('groupRows', () => {
  const rows = [row('f201'), row('d121'), row('lab:x204'), row('t132')];

  it('returns one anonymous bucket without grouping', () => {
    expect(groupRows(g, rows, 'none')).toEqual([{ key: '', rows }]);
  });
  it('buckets by kind, biggest bucket first, server order inside', () => {
    const got = groupRows(g, rows, 'kind');
    expect(got[0]).toEqual({ key: 'fact', rows: [row('f201'), row('lab:x204')] });
    expect(got.map((x) => x.key)).toEqual(['fact', 'decision', 'task']);
  });
  it('buckets by source with local as the fallback', () => {
    expect(groupRows(g, rows, 'source').map((x) => x.key)).toEqual(['local', 'lab']);
  });
  it('buckets an unknown node under ?', () => {
    expect(groupRows(g, [row('zz9')], 'kind')[0].key).toBe('?');
  });
});

describe('narrow helpers', () => {
  it('reports whether anything narrows', () => {
    expect(isNarrowed(EMPTY_NARROW)).toBe(false);
    expect(isNarrowed(n({ text: 'x' }))).toBe(true);
    expect(isNarrowed(n({ sources: ['lab'] }))).toBe(true);
  });
  it('splits the narrow text into terms for highlighting', () => {
    expect(narrowTerms(n({ text: ' Login  LIMIT ' }))).toEqual(['login', 'limit']);
    expect(narrowTerms(EMPTY_NARROW)).toEqual([]);
  });
});
