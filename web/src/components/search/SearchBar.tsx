// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useState } from 'react';
import type { SearchMode, Source } from '../../api/types';

export interface SearchBarProps {
  query: string;
  mode: SearchMode;
  sources?: Source[];
  selectedSources?: string[];
  onSearch: (q: string, mode: SearchMode) => void;
  onMode: (m: SearchMode) => void;
  onSources?: (s: string[]) => void;
  /** How many hits the server should return. */
  k?: number;
  onK?: (k: number) => void;
  busy?: boolean;
}

export const K_CHOICES = [8, 15, 30, 50];

export const MODE_LABEL: Record<SearchMode, string> = {
  hybrid: 'Hybrid (vector → graph)',
  vector: 'Vector (kNN)',
  tree: 'Tree (beam over graph)',
  graph: 'Graph (words + activation)',
};

/** Knowledge search box with mode and federated source selectors. */
export function SearchBar({ query, mode, sources, selectedSources = [], onSearch, onMode, onSources, k, onK, busy }: SearchBarProps) {
  const [q, setQ] = useState(query);
  return (
    <form
      className="row nowrap"
      role="search"
      onSubmit={(e) => {
        e.preventDefault();
        onSearch(q, mode);
      }}
    >
      <select className="input" aria-label="Search mode" value={mode} onChange={(e) => onMode(e.target.value as SearchMode)}>
        {(Object.keys(MODE_LABEL) as SearchMode[]).map((m) => (
          <option key={m} value={m}>
            {MODE_LABEL[m]}
          </option>
        ))}
      </select>
      <input className="input grow" aria-label="Search knowledge" placeholder='Search knowledge… e.g. "entra limits", "who approves app changes"' value={q} onChange={(e) => setQ(e.target.value)} />
      {sources && sources.length > 0 && onSources && (
        <select className="input" aria-label="Sources" value={selectedSources.join(',')} onChange={(e) => onSources(e.target.value ? e.target.value.split(',') : [])}>
          <option value="">local</option>
          <option value="*">+ all sources</option>
          {sources.map((s) => (
            <option key={s.name} value={s.name}>
              + {s.name}
            </option>
          ))}
        </select>
      )}
      {onK && (
        <select className="input" style={{ width: 86 }} aria-label="Hits" value={k ?? 8} onChange={(e) => onK(Number(e.target.value))}>
          {K_CHOICES.map((n) => (
            <option key={n} value={n}>
              top {n}
            </option>
          ))}
        </select>
      )}
      <button className="btn primary" type="submit" disabled={busy || !q.trim()}>
        {busy ? <span className="spinner" /> : 'Search'}
      </button>
    </form>
  );
}
