// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useMemo } from 'react';
import type { SearchResult, SearchRow } from '../../api/types';
import { depthColor, type GraphIndex } from '../../model/graph';
import { EMPTY_NARROW, GROUP_LABELS, groupRows, isNarrowed, narrowResult, narrowTerms, rowSource, type GroupBy, type ResultNarrow } from '../../model/searchView';
import { ResultRow } from './ResultRow';
import { Icon } from '../common/Icon';

export interface SearchResultsProps {
  g: GraphIndex;
  query: string;
  result: SearchResult;
  onGo: (id: string) => void;
  onFocus: (ids: string[]) => void;
  elapsedMs?: number;
  /** Client-side narrowing of what came back (free text + facets). */
  narrow?: ResultNarrow;
  group?: GroupBy;
}

/** Words of the query worth highlighting (skip 1-letter noise). */
const queryTerms = (q: string) => q.toLowerCase().split(/[^\p{L}\p{N}]+/u).filter((w) => w.length > 1);

function Groups({ g, rows, by, terms, onGo, right }: { g: GraphIndex; rows: SearchRow[]; by: GroupBy; terms: string[]; onGo: (id: string) => void; right: (r: SearchRow) => React.ReactNode }) {
  const groups = useMemo(() => groupRows(g, rows, by), [g, rows, by]);
  return (
    <div className="result-list">
      {groups.map((grp) => (
        <div key={grp.key} className="result-group">
          {by !== 'none' && (
            <div className="result-group-h">
              {by === 'kind' && <i className="dot" style={{ background: g.kind(grp.key).color }} />}
              <span className="lbl">{grp.key}</span>
              <span className="cnt mono">{grp.rows.length}</span>
            </div>
          )}
          {grp.rows.map((r) => (
            <ResultRow key={r.id} g={g} id={r.id} terms={terms} onGo={onGo} source={rowSource(g, r)} right={right(r)} />
          ))}
        </div>
      ))}
      {!rows.length && <span className="muted small">nothing left after narrowing</span>}
    </div>
  );
}

/** Search results rendered per mode (hybrid bars, vector lift, beam tree, graph direct + via). */
export function SearchResults({ g, query, result: full, onGo, onFocus, elapsedMs, narrow = EMPTY_NARROW, group = 'none' }: SearchResultsProps) {
  const result = useMemo(() => narrowResult(g, full, narrow), [g, full, narrow]);
  const terms = useMemo(() => [...queryTerms(query), ...narrowTerms(narrow)], [query, narrow]);
  const mode = result.mode;
  const seed = (ids: string[]) =>
    ids.length > 0 && (
      <button className="btn primary sm" style={{ marginTop: 6 }} onClick={() => onFocus(ids)}>
        <Icon name="focus" /> Light as focus
      </button>
    );
  const hidden = full.rows.length - result.rows.length + (full.via?.length ?? 0) - (result.via?.length ?? 0);
  const meta = (
    <>
      {elapsedMs !== undefined && <span className="muted xs">{elapsedMs} ms</span>}
      {isNarrowed(narrow) && hidden > 0 && <span className="muted xs">· {hidden} hidden</span>}
      {group !== 'none' && <span className="muted xs">· {GROUP_LABELS[group]}</span>}
    </>
  );

  if (mode === 'vector')
    return (
      <div className="tool">
        <h4>
          <Icon name="ruler" /> Vector kNN: “{query}” {meta}
        </h4>
        <div className="xs muted">Query cosine to the raw → enriched vector. ▲n: graph enrichment lifted the memory n places because its neighbours match the query.</div>
        <Groups
          g={g}
          rows={result.rows}
          by={group}
          terms={terms}
          onGo={onGo}
          right={(r) => (
            <>
              {(r.raw ?? 0).toFixed(2)}→{(r.enriched ?? r.score).toFixed(2)} {r.lift && r.lift > 0 ? <b className="ok">▲{r.lift}</b> : null}
            </>
          )}
        />
        {seed(result.rows.slice(0, 4).map((r) => r.id))}
      </div>
    );

  if (mode === 'hybrid') {
    const L = result.lambda ?? 0.5;
    return (
      <div className="tool">
        <h4>
          <Icon name="hybrid" /> Hybrid (GraphRAG): “{query}” {meta}
        </h4>
        <div className="xs muted">
          1) Vectors pick up to 4 seeds (≥ 50% of the best). 2) Activation spreads along synapses and concepts. 3) Score = λ·cos + (1−λ)·act, λ={L.toFixed(2)}. <span style={{ color: 'var(--accent)' }}>■</span> vector{' '}
          <span style={{ color: '#ffa94d' }}>■</span> graph
        </div>
        <Groups
          g={g}
          rows={result.rows}
          by={group}
          terms={terms}
          onGo={onGo}
          right={(r) => (
            <>
              <span className="mbar" style={{ width: (r.vector ?? 0) * L * 60, background: 'var(--accent)' }} />
              <span className="mbar" style={{ width: (r.graph ?? 0) * (1 - L) * 60, background: '#ffa94d' }} /> {r.score.toFixed(2)}
            </>
          )}
        />
        {seed(result.rows.slice(0, 5).map((r) => r.id))}
      </div>
    );
  }

  if (mode === 'tree') {
    const kids = new Map<string | null, SearchRow[]>();
    for (const r of result.rows) {
      const p = r.parent ?? null;
      if (!kids.has(p)) kids.set(p, []);
      kids.get(p)!.push(r);
    }
    const roots = result.roots?.length ? result.rows.filter((r) => result.roots!.includes(r.id)) : (kids.get(null) ?? []);
    const Node = ({ r }: { r: SearchRow }) => {
      const ch = (kids.get(r.id) ?? []).sort((a, b) => b.score - a.score);
      return (
        <li>
          <ResultRow g={g} id={r.id} terms={terms} onGo={onGo} source={rowSource(g, r)} right={<span style={{ color: depthColor(r.depth) }}>d{r.depth ?? 0} · {r.score.toFixed(2)}</span>} />
          {ch.length > 0 && (
            <ul>
              {ch.map((c) => (
                <Node key={c.id} r={c} />
              ))}
            </ul>
          )}
        </li>
      );
    };
    return (
      <div className="tool stree">
        <h4>
          <Icon name="tree" /> Search tree (beam): “{query}” {meta}
        </h4>
        <div className="xs muted">Start: the 2 nearest vectors. Each node expands its 3 best neighbours, each level keeps 6. Score = 0.55·cos(query, vector) + 0.45·(parent score × synapse weight). Branches glow in 3D.</div>
        <ul style={{ border: 0, padding: 0, marginTop: 6 }}>
          {roots.map((r) => (
            <Node key={r.id} r={r} />
          ))}
        </ul>
        {!result.rows.length && <span className="muted small">nothing left after narrowing</span>}
        {seed(roots.map((r) => r.id))}
      </div>
    );
  }

  // graph
  return (
    <div className="tool">
      <h4>
        <Icon name="search" /> “{query}” {meta}
      </h4>
      <div className="xs muted">Direct hits (shared word stems × weight):</div>
      <Groups g={g} rows={result.rows} by={group} terms={terms} onGo={onGo} right={(r) => r.score.toFixed(2)} />
      {seed(result.rows.slice(0, 4).map((r) => r.id))}
      <div className="xs muted" style={{ marginTop: 8 }}>
        Knowledge found <b>through links</b> (no query words, reached by activation from the hits):
      </div>
      <Groups g={g} rows={result.via ?? []} by={group} terms={terms} onGo={onGo} right={(r) => `d${r.depth} · ${r.score.toFixed(2)}`} />
    </div>
  );
}
