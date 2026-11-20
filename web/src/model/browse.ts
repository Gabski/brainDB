// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

// Browse tab: pure filter / sort / facet logic over the State snapshot. No React, no store.
import type { BrainNode, EntityRow } from '../api/types';
import { isConceptNode } from './graph';

export type SortKey = 'weight' | 'activation' | 'age' | 'kind' | 'section' | 'text' | 'id';

export const SORT_LABELS: Record<SortKey, string> = {
  weight: 'weight',
  activation: 'activation',
  age: 'age',
  kind: 'kind',
  section: 'section',
  text: 'text',
  id: 'id',
};

/** One facet group of the Browse tab. */
export type FacetGroup = 'kinds' | 'sections' | 'sources' | 'entity';

export interface BrowseQuery {
  /** Free text; supports the `kind:`, `section:` and `source:` prefixes. */
  q: string;
  kinds: string[];
  sections: string[];
  sources: string[];
  /** "type:value" picked in the Entities facet, or null. */
  entity: string | null;
  /** Keep only pinned memories. */
  pinned: boolean;
  /** Include concept nodes (A:…). */
  concepts: boolean;
  /** Include superseded memories. */
  superseded: boolean;
  /** Include external (federated) stubs. */
  external: boolean;
  sort: SortKey;
  desc: boolean;
}

export const DEFAULT_BROWSE: BrowseQuery = {
  q: '',
  kinds: [],
  sections: [],
  sources: [],
  entity: null,
  pinned: false,
  concepts: true,
  superseded: false,
  external: true,
  sort: 'weight',
  desc: true,
};

export const isDefaultBrowse = (q: BrowseQuery) =>
  !q.q && !q.kinds.length && !q.sections.length && !q.sources.length && !q.entity && !q.pinned && q.concepts && !q.superseded && q.external;

export interface ParsedQuery {
  /** Plain words every row must contain (AND). */
  terms: string[];
  kinds: string[];
  sections: string[];
  sources: string[];
}

const unquote = (s: string) => (s.length > 1 && ((s[0] === '"' && s.endsWith('"')) || (s[0] === "'" && s.endsWith("'"))) ? s.slice(1, -1) : s);

/** Splits the free-text box into plain terms plus `kind:`/`section:`/`source:` filters. */
export function parseQuery(q: string): ParsedQuery {
  const out: ParsedQuery = { terms: [], kinds: [], sections: [], sources: [] };
  // quoted phrases stay in one piece
  for (const raw of q.match(/"[^"]*"|'[^']*'|\S+/g) ?? []) {
    const m = /^(kind|section|source):(.*)$/i.exec(raw);
    if (m && m[2]) {
      const v = unquote(m[2]).toLowerCase();
      if (m[1].toLowerCase() === 'kind') out.kinds.push(v);
      else if (m[1].toLowerCase() === 'section') out.sections.push(v);
      else out.sources.push(v);
      continue;
    }
    const t = unquote(raw).toLowerCase();
    if (t) out.terms.push(t);
  }
  return out;
}

/** Everything a free-text term may match, lower-cased. */
export function haystack(n: BrainNode): string {
  const f = n.fields ? Object.entries(n.fields).map(([k, v]) => k + ' ' + v).join(' ') : '';
  return `${n.id} ${n.text} ${n.kind} ${n.section} ${n.source ?? ''} ${f}`.toLowerCase();
}

export const entityKey = (type: string, value: string) => type + ':' + value;

/** Node ids carrying the picked entity ("type:value"), or null when nothing is picked. */
export function entityNodes(rows: EntityRow[] | null | undefined, picked: string | null): Set<string> | null {
  if (!picked || !rows) return null;
  const out = new Set<string>();
  for (const r of rows) if (entityKey(r.type, r.value) === picked) out.add(r.node);
  return out;
}

/**
 * Applies the query to the snapshot. `drop` skips one facet group, which is what facet
 * counts need: every count is "how many rows would this value add to the current result".
 */
export function browseFilter(nodes: BrainNode[], q: BrowseQuery, ents: Set<string> | null, drop?: FacetGroup): BrainNode[] {
  const p = parseQuery(q.q);
  // Chips and query prefixes are two independent constraints: OR inside a group, AND between them.
  // `drop` only drops the chips, because the prefix is part of the free-text query itself.
  const chips = {
    kinds: new Set(drop === 'kinds' ? [] : q.kinds.map((x) => x.toLowerCase())),
    sections: new Set(drop === 'sections' ? [] : q.sections.map((x) => x.toLowerCase())),
    sources: new Set(drop === 'sources' ? [] : q.sources.map((x) => x.toLowerCase())),
  };
  const typed = { kinds: new Set(p.kinds), sections: new Set(p.sections), sources: new Set(p.sources) };
  const miss = (a: Set<string>, b: Set<string>, v: string) => (a.size && !a.has(v)) || (b.size && !b.has(v));
  const byEntity = drop === 'entity' ? null : ents;
  const out: BrainNode[] = [];
  for (const n of nodes) {
    const concept = isConceptNode(n);
    if (concept && !q.concepts) continue;
    if (!q.superseded && n.supersededBy) continue;
    if (!q.external && n.external) continue;
    if (q.pinned && !n.pinned) continue;
    if (miss(chips.kinds, typed.kinds, n.kind.toLowerCase())) continue;
    if (miss(chips.sections, typed.sections, n.section.toLowerCase())) continue;
    if (miss(chips.sources, typed.sources, (n.source ?? 'local').toLowerCase())) continue;
    if (byEntity && !byEntity.has(n.id)) continue;
    if (p.terms.length) {
      const h = haystack(n);
      if (!p.terms.every((t) => h.includes(t))) continue;
    }
    out.push(n);
  }
  return out;
}

const cmpStr = (a: string, b: string) => (a < b ? -1 : a > b ? 1 : 0);

/** Stable sort by the chosen key; missing numbers sort last in both directions. */
export function browseSort(rows: BrainNode[], sort: SortKey, desc: boolean): BrainNode[] {
  const num = (n: BrainNode): number | undefined => {
    if (sort === 'weight') return n.weight;
    if (sort === 'activation') return n.activation;
    if (sort === 'age') return n.age;
    return undefined;
  };
  const out = [...rows];
  out.sort((a, b) => {
    let c = 0;
    if (sort === 'id') c = cmpStr(a.id, b.id);
    else if (sort === 'text') c = cmpStr(a.text.toLowerCase(), b.text.toLowerCase());
    else if (sort === 'kind') c = cmpStr(a.kind, b.kind) || cmpStr(a.id, b.id);
    else if (sort === 'section') c = cmpStr(a.section, b.section) || cmpStr(a.id, b.id);
    else {
      const x = num(a);
      const y = num(b);
      if (x === undefined && y === undefined) c = cmpStr(a.id, b.id);
      else if (x === undefined) return 1; // undefined always last
      else if (y === undefined) return -1;
      else c = x - y || cmpStr(a.id, b.id);
    }
    return desc ? -c : c;
  });
  return out;
}

export interface Facet {
  value: string;
  count: number;
}

const tally = (nodes: BrainNode[], pick: (n: BrainNode) => string | undefined): Facet[] => {
  const m = new Map<string, number>();
  for (const n of nodes) {
    const v = pick(n);
    if (v === undefined) continue;
    m.set(v, (m.get(v) ?? 0) + 1);
  }
  return [...m.entries()].map(([value, count]) => ({ value, count })).sort((a, b) => b.count - a.count || cmpStr(a.value, b.value));
};

export interface Facets {
  kinds: Facet[];
  sections: Facet[];
  sources: Facet[];
  /** Entity "type:value" counted over the rows that pass every other filter. */
  entities: Facet[];
  /** Rows matching everything but the flag, so the flag toggles show what they would add. */
  total: number;
}

/** Facet counts for the sidebar: each group counted with the other groups applied. */
export function browseFacets(nodes: BrainNode[], q: BrowseQuery, ents: Set<string> | null, rows: EntityRow[] | null | undefined): Facets {
  const pick = entityNodes(rows, q.entity);
  return {
    kinds: tally(browseFilter(nodes, q, pick, 'kinds'), (n) => n.kind),
    sections: tally(browseFilter(nodes, q, pick, 'sections'), (n) => n.section),
    sources: tally(browseFilter(nodes, q, pick, 'sources'), (n) => n.source ?? 'local'),
    entities: entityFacets(browseFilter(nodes, q, pick, 'entity'), rows),
    total: browseFilter(nodes, q, ents).length,
  };
}

/** Entity "type:value" counts restricted to the given rows. */
export function entityFacets(nodes: BrainNode[], rows: EntityRow[] | null | undefined): Facet[] {
  if (!rows?.length) return [];
  const ids = new Set(nodes.map((n) => n.id));
  const m = new Map<string, Set<string>>();
  for (const r of rows) {
    if (!ids.has(r.node)) continue;
    const k = entityKey(r.type, r.value);
    if (!m.has(k)) m.set(k, new Set());
    m.get(k)!.add(r.node);
  }
  return [...m.entries()].map(([value, set]) => ({ value, count: set.size })).sort((a, b) => b.count - a.count || cmpStr(a.value, b.value));
}

/** Non-overlapping [start, end) ranges of `text` matching any term, for highlighting. */
export function matchRanges(text: string, terms: string[]): [number, number][] {
  if (!terms.length) return [];
  const low = text.toLowerCase();
  const hits: [number, number][] = [];
  for (const t of terms) {
    if (!t) continue;
    let i = low.indexOf(t);
    while (i >= 0) {
      hits.push([i, i + t.length]);
      i = low.indexOf(t, i + t.length);
    }
  }
  hits.sort((a, b) => a[0] - b[0] || b[1] - a[1]);
  const out: [number, number][] = [];
  for (const h of hits) {
    const last = out[out.length - 1];
    if (last && h[0] <= last[1]) last[1] = Math.max(last[1], h[1]);
    else out.push([...h]);
  }
  return out;
}

/**
 * A window of `text` around the first match, so a long memory still shows why it matched.
 * Returns the slice plus the offset it starts at (ranges must be shifted by it).
 */
export function snippet(text: string, terms: string[], max = 220): { text: string; offset: number } {
  if (text.length <= max) return { text, offset: 0 };
  const r = matchRanges(text, terms);
  const first = r.length ? r[0][0] : 0;
  if (first < max - 40) return { text: text.slice(0, max) + '…', offset: 0 };
  const start = Math.max(0, first - 60);
  return { text: '…' + text.slice(start, start + max) + (start + max < text.length ? '…' : ''), offset: start - 1 };
}
