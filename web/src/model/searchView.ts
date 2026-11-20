// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

// Search tab: pure narrowing / grouping of a SearchResult, client-side. The server ranks,
// this only hides and buckets what came back.
import type { SearchResult, SearchRow } from '../api/types';
import type { GraphIndex } from './graph';
import { haystack } from './browse';

export type GroupBy = 'none' | 'kind' | 'section' | 'source';

export const GROUP_LABELS: Record<GroupBy, string> = {
  none: 'no grouping',
  kind: 'by kind',
  section: 'by section',
  source: 'by source',
};

export interface ResultNarrow {
  /** Free text applied to the rows that came back (id, text, kind, section, source, fields). */
  text: string;
  kinds: string[];
  sections: string[];
  sources: string[];
}

export const EMPTY_NARROW: ResultNarrow = { text: '', kinds: [], sections: [], sources: [] };

export const isNarrowed = (n: ResultNarrow) => !!n.text || !!n.kinds.length || !!n.sections.length || !!n.sources.length;

/** The row's source: the federated source name, or 'local'. */
export const rowSource = (g: GraphIndex, r: SearchRow) => r.source ?? g.nodes.get(r.id)?.source ?? 'local';

const terms = (text: string) => text.toLowerCase().split(/\s+/).filter(Boolean);

/** Free-text terms of a narrow, for highlighting. */
export const narrowTerms = (n: ResultNarrow) => terms(n.text);

function keep(g: GraphIndex, r: SearchRow, n: ResultNarrow, drop?: keyof ResultNarrow): boolean {
  const node = g.nodes.get(r.id);
  if (drop !== 'kinds' && n.kinds.length && !n.kinds.includes(node?.kind ?? '?')) return false;
  if (drop !== 'sections' && n.sections.length && !n.sections.includes(node?.section ?? '?')) return false;
  if (drop !== 'sources' && n.sources.length && !n.sources.includes(rowSource(g, r))) return false;
  if (drop !== 'text' && n.text) {
    const h = node ? haystack(node) : r.id.toLowerCase();
    if (!terms(n.text).every((t) => h.includes(t))) return false;
  }
  return true;
}

/**
 * Narrows `rows`. In tree mode a kept row whose parent was dropped is re-parented to the nearest
 * kept ancestor, so the hierarchy never loses a branch silently.
 */
export function narrowRows(g: GraphIndex, rows: SearchRow[], n: ResultNarrow, tree = false): SearchRow[] {
  const kept = rows.filter((r) => keep(g, r, n));
  if (!tree) return kept;
  const ok = new Set(kept.map((r) => r.id));
  const by = new Map(rows.map((r) => [r.id, r]));
  return kept.map((r) => {
    let p = r.parent ?? null;
    const seen = new Set<string>();
    while (p && !ok.has(p) && !seen.has(p)) {
      seen.add(p);
      p = by.get(p)?.parent ?? null;
    }
    return p === (r.parent ?? null) ? r : { ...r, parent: p };
  });
}

/** Narrows every list of a result (rows + via) in one go. */
export function narrowResult(g: GraphIndex, result: SearchResult, n: ResultNarrow): SearchResult {
  if (!isNarrowed(n)) return result;
  const rows = narrowRows(g, result.rows, n, result.mode === 'tree');
  const ids = new Set(rows.map((r) => r.id));
  return {
    ...result,
    rows,
    via: result.via ? narrowRows(g, result.via, n) : result.via,
    roots: result.roots?.filter((id) => ids.has(id)),
  };
}

export interface ResultFacet {
  value: string;
  count: number;
}

const tally = (rows: SearchRow[], pick: (r: SearchRow) => string | undefined): ResultFacet[] => {
  const m = new Map<string, number>();
  for (const r of rows) {
    const v = pick(r);
    if (v === undefined) continue;
    m.set(v, (m.get(v) ?? 0) + 1);
  }
  return [...m.entries()].map(([value, count]) => ({ value, count })).sort((a, b) => b.count - a.count || (a.value < b.value ? -1 : 1));
};

export interface ResultFacets {
  kinds: ResultFacet[];
  sections: ResultFacet[];
  sources: ResultFacet[];
}

/** Facet counts over everything the search returned (rows + via), each group counted with the others applied. */
export function resultFacets(g: GraphIndex, result: SearchResult, n: ResultNarrow): ResultFacets {
  const all = [...result.rows, ...(result.via ?? [])];
  const of = (drop: keyof ResultNarrow) => all.filter((r) => keep(g, r, n, drop));
  return {
    kinds: tally(of('kinds'), (r) => g.nodes.get(r.id)?.kind),
    sections: tally(of('sections'), (r) => g.nodes.get(r.id)?.section),
    sources: tally(of('sources'), (r) => rowSource(g, r)),
  };
}

export interface RowGroup {
  key: string;
  rows: SearchRow[];
}

/** Buckets rows for the chosen grouping, keeping the server's order inside each bucket. */
export function groupRows(g: GraphIndex, rows: SearchRow[], by: GroupBy): RowGroup[] {
  if (by === 'none') return [{ key: '', rows }];
  const m = new Map<string, SearchRow[]>();
  for (const r of rows) {
    const node = g.nodes.get(r.id);
    const k = by === 'kind' ? (node?.kind ?? '?') : by === 'section' ? (node?.section ?? '?') : rowSource(g, r);
    if (!m.has(k)) m.set(k, []);
    m.get(k)!.push(r);
  }
  return [...m.entries()]
    .map(([key, list]) => ({ key, rows: list }))
    .sort((a, b) => b.rows.length - a.rows.length || (a.key < b.key ? -1 : 1));
}
