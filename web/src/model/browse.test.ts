// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from 'vitest';
import {
  browseFacets,
  browseFilter,
  browseSort,
  DEFAULT_BROWSE,
  entityFacets,
  entityKey,
  entityNodes,
  haystack,
  isDefaultBrowse,
  matchRanges,
  parseQuery,
  snippet,
  type BrowseQuery,
} from './browse';
import { entityRow, node } from './testutil';

const q = (over: Partial<BrowseQuery> = {}): BrowseQuery => ({ ...DEFAULT_BROWSE, ...over });

const NODES = [
  node('f201', { kind: 'fact', section: 'facts', text: 'Entra login rate limit is 30 per minute', weight: 0.9 }),
  node('d121', { kind: 'decision', section: 'decisions', text: 'We approve app changes in the platform board', weight: 0.7, pinned: true }),
  node('t132', { kind: 'task', section: 'tasks', text: 'Raise the login limit with support', weight: 0.4, activation: 0.8 }),
  node('n154', { kind: 'note', section: 'notes', text: 'Old note about login', weight: 0.2, supersededBy: 'n155' }),
  node('lab:x204', { kind: 'fact', section: 'facts', text: 'External fact from the lab', weight: 0.6, source: 'lab', external: true }),
  node('A:idp', { kind: 'concept', section: 'concepts', text: 'identity provider', weight: 0.8, activation: 0.5 }),
];

describe('parseQuery', () => {
  it('splits plain terms and lower-cases them', () => {
    expect(parseQuery('Login Limit')).toEqual({ terms: ['login', 'limit'], kinds: [], sections: [], sources: [] });
  });
  it('reads the kind:/section:/source: prefixes', () => {
    const p = parseQuery('kind:Fact section:facts source:lab rate');
    expect(p).toEqual({ terms: ['rate'], kinds: ['fact'], sections: ['facts'], sources: ['lab'] });
  });
  it('keeps a quoted phrase in one piece', () => {
    expect(parseQuery('"rate limit" login').terms).toEqual(['rate limit', 'login']);
  });
  it('treats a prefix with no value as plain text', () => {
    expect(parseQuery('kind:').terms).toEqual(['kind:']);
  });
  it('returns nothing for an empty query', () => {
    expect(parseQuery('   ')).toEqual({ terms: [], kinds: [], sections: [], sources: [] });
  });
});

describe('haystack', () => {
  it('covers id, text, kind, section, source and field values', () => {
    const h = haystack(node('r193', { kind: 'record', section: 'records', text: 'Deal', source: 'crm', fields: { owner: 'Ada' } }));
    expect(h).toContain('r193');
    expect(h).toContain('deal');
    expect(h).toContain('record');
    expect(h).toContain('crm');
    expect(h).toContain('ada');
  });
});

describe('browseFilter', () => {
  const ids = (res: ReturnType<typeof browseFilter>) => res.map((n) => n.id);

  it('hides superseded by default and shows them on request', () => {
    expect(ids(browseFilter(NODES, q(), null))).not.toContain('n154');
    expect(ids(browseFilter(NODES, q({ superseded: true }), null))).toContain('n154');
  });
  it('drops concepts when they are switched off', () => {
    expect(ids(browseFilter(NODES, q({ concepts: false }), null))).not.toContain('A:idp');
  });
  it('drops external stubs when they are switched off', () => {
    expect(ids(browseFilter(NODES, q({ external: false }), null))).not.toContain('lab:x204');
  });
  it('keeps only pinned memories', () => {
    expect(ids(browseFilter(NODES, q({ pinned: true }), null))).toEqual(['d121']);
  });
  it('ANDs every free-text term', () => {
    expect(ids(browseFilter(NODES, q({ q: 'login limit' }), null))).toEqual(['f201', 't132']);
    expect(ids(browseFilter(NODES, q({ q: 'login nonsense' }), null))).toEqual([]);
  });
  it('merges the chip facets with the query prefixes', () => {
    expect(ids(browseFilter(NODES, q({ q: 'kind:task', kinds: ['fact'] }), null))).toEqual([]);
    expect(ids(browseFilter(NODES, q({ kinds: ['fact'] }), null))).toEqual(['f201', 'lab:x204']);
  });
  it('treats a node without a source as local', () => {
    expect(ids(browseFilter(NODES, q({ sources: ['local'] }), null))).not.toContain('lab:x204');
    expect(ids(browseFilter(NODES, q({ sources: ['lab'] }), null))).toEqual(['lab:x204']);
  });
  it('keeps only the nodes carrying the picked entity', () => {
    const ents = new Set(['t132']);
    expect(ids(browseFilter(NODES, q(), ents))).toEqual(['t132']);
  });
  it('ignores the dropped facet group (what the counts need)', () => {
    expect(ids(browseFilter(NODES, q({ kinds: ['fact'] }), null, 'kinds'))).toContain('d121');
  });
});

describe('browseSort', () => {
  it('sorts by weight, descending by default', () => {
    const r = browseSort(browseFilter(NODES, q(), null), 'weight', true);
    expect(r.map((n) => n.id)).toEqual(['f201', 'A:idp', 'd121', 'lab:x204', 't132']);
  });
  it('puts missing numbers last in both directions', () => {
    const lit = ['t132', 'A:idp']; // the only nodes with an activation
    for (const desc of [true, false]) {
      const ids = browseSort(browseFilter(NODES, q(), null), 'activation', desc).map((n) => n.id);
      expect(ids.slice(0, 2).sort()).toEqual([...lit].sort());
      expect(ids.slice(2)).not.toContain('t132');
    }
    expect(browseSort(browseFilter(NODES, q(), null), 'activation', false).slice(0, 2).map((n) => n.id)).toEqual(['A:idp', 't132']);
  });
  it('breaks ties by id, so the order is stable', () => {
    const a = node('a1', { weight: 0.5 });
    const b = node('b1', { weight: 0.5 });
    expect(browseSort([b, a], 'weight', false).map((n) => n.id)).toEqual(['a1', 'b1']);
  });
  it('sorts text case-insensitively', () => {
    const r = browseSort([node('x', { text: 'beta' }), node('y', { text: 'Alpha' })], 'text', false);
    expect(r.map((n) => n.id)).toEqual(['y', 'x']);
  });
  it('does not mutate the input', () => {
    const input = [node('b'), node('a')];
    browseSort(input, 'id', false);
    expect(input.map((n) => n.id)).toEqual(['b', 'a']);
  });
});

describe('browseFacets', () => {
  it('counts each group with the other groups applied', () => {
    const query = q({ kinds: ['fact'] });
    const f = browseFacets(NODES, query, null, null);
    // kinds are counted as if no kind were picked …
    expect(f.kinds.find((x) => x.value === 'decision')?.count).toBe(1);
    // … while sections already respect the picked kind
    expect(f.sections.find((x) => x.value === 'decisions')).toBeUndefined();
    expect(f.total).toBe(2);
  });
  it('counts sources with local as the fallback', () => {
    const f = browseFacets(NODES, q(), null, null);
    expect(f.sources.find((x) => x.value === 'local')?.count).toBe(4);
    expect(f.sources.find((x) => x.value === 'lab')?.count).toBe(1);
  });
});

describe('entities', () => {
  const rows = [entityRow('f201', 'number_unit', '30/min'), entityRow('t132', 'number_unit', '30/min'), entityRow('d121', 'org', 'platform board')];

  it('keys a type/value pair', () => {
    expect(entityKey('org', 'acme')).toBe('org:acme');
  });
  it('maps a picked entity to its nodes', () => {
    expect([...(entityNodes(rows, 'number_unit:30/min') ?? [])]).toEqual(['f201', 't132']);
  });
  it('returns null when nothing is picked or nothing is loaded', () => {
    expect(entityNodes(rows, null)).toBeNull();
    expect(entityNodes(null, 'org:platform board')).toBeNull();
  });
  it('counts distinct nodes per entity, restricted to the given rows', () => {
    const f = entityFacets(NODES.filter((n) => n.id !== 't132'), rows);
    expect(f.find((x) => x.value === 'number_unit:30/min')?.count).toBe(1);
  });
  it('counts nothing when the entity index is empty', () => {
    expect(entityFacets(NODES, [])).toEqual([]);
  });
});

describe('matchRanges', () => {
  it('finds every occurrence, case-insensitively', () => {
    expect(matchRanges('Login login', ['login'])).toEqual([
      [0, 5],
      [6, 11],
    ]);
  });
  it('merges overlapping terms into one range', () => {
    expect(matchRanges('abcdef', ['abc', 'bcd'])).toEqual([[0, 4]]);
  });
  it('returns nothing without terms or matches', () => {
    expect(matchRanges('abc', [])).toEqual([]);
    expect(matchRanges('abc', ['z'])).toEqual([]);
  });
});

describe('snippet', () => {
  it('returns short text untouched', () => {
    expect(snippet('short', ['s'])).toEqual({ text: 'short', offset: 0 });
  });
  it('cuts from the start when the match is near it', () => {
    const long = 'login ' + 'x'.repeat(400);
    const s = snippet(long, ['login'], 100);
    expect(s.text.startsWith('login')).toBe(true);
    expect(s.text.endsWith('…')).toBe(true);
    expect(s.offset).toBe(0);
  });
  it('windows around a late match so the hit stays visible', () => {
    const long = 'x'.repeat(400) + ' login ' + 'y'.repeat(400);
    const s = snippet(long, ['login'], 120);
    expect(s.text).toContain('login');
    expect(s.text.startsWith('…')).toBe(true);
  });
});

describe('isDefaultBrowse', () => {
  it('is true only for the untouched query', () => {
    expect(isDefaultBrowse(DEFAULT_BROWSE)).toBe(true);
    expect(isDefaultBrowse(q({ pinned: true }))).toBe(false);
    expect(isDefaultBrowse(q({ q: 'x' }))).toBe(false);
    expect(isDefaultBrowse(q({ entity: 'org:x' }))).toBe(false);
  });
});
