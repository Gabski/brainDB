// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { SearchResult } from '../../api/types';
import type { GraphIndex } from '../../model/graph';
import { GROUP_LABELS, isNarrowed, resultFacets, type GroupBy, type ResultNarrow } from '../../model/searchView';
import { Icon } from '../common/Icon';

export interface NarrowBarProps {
  g: GraphIndex;
  result: SearchResult;
  narrow: ResultNarrow;
  group: GroupBy;
  onNarrow: (p: Partial<ResultNarrow>) => void;
  onToggle: (group: 'kinds' | 'sections' | 'sources', value: string) => void;
  onGroup: (g: GroupBy) => void;
  onClear: () => void;
}

/**
 * Second toolbar of the Search tab: narrows what came back without asking the server again.
 * Facet chips carry counts, so it is visible what each one would hide.
 */
export function NarrowBar({ g, result, narrow, group, onNarrow, onToggle, onGroup, onClear }: NarrowBarProps) {
  const f = resultFacets(g, result, narrow);
  const chip = (grp: 'kinds' | 'sections' | 'sources', value: string, count: number, dot?: string) => {
    const on = narrow[grp].includes(value);
    return (
      <button key={grp + value} className={'chip' + (on ? ' on' : '')} aria-pressed={on} onClick={() => onToggle(grp, value)} title={grp.slice(0, -1) + ' ' + value}>
        {dot && <i className="dot" style={{ background: dot }} />}
        {value} <span className="muted mono xs">{count}</span>
      </button>
    );
  };
  return (
    <div className="toolbar narrow-bar">
      <label className="row nowrap" style={{ gap: 4, flex: '1 1 220px', minWidth: 160 }}>
        <Icon name="filter" size={14} />
        <input className="input grow" placeholder="Narrow these results…" value={narrow.text} onChange={(e) => onNarrow({ text: e.target.value })} aria-label="Narrow results" />
      </label>
      <select className="input" style={{ width: 128 }} value={group} onChange={(e) => onGroup(e.target.value as GroupBy)} aria-label="Group results">
        {(Object.keys(GROUP_LABELS) as GroupBy[]).map((k) => (
          <option key={k} value={k}>
            {GROUP_LABELS[k]}
          </option>
        ))}
      </select>
      <span className="sep" />
      <div className="chips grow">
        {f.kinds.slice(0, 6).map((x) => chip('kinds', x.value, x.count, g.kind(x.value).color))}
        {f.sections.length > 1 && f.sections.slice(0, 4).map((x) => chip('sections', x.value, x.count))}
        {f.sources.length > 1 && f.sources.map((x) => chip('sources', x.value, x.count))}
      </div>
      {isNarrowed(narrow) && (
        <button className="btn ghost sm" onClick={onClear}>
          clear
        </button>
      )}
    </div>
  );
}

export interface RecentBarProps {
  items: { query: string; mode: string; rows?: number; ms?: number }[];
  onPick: (query: string, mode: string) => void;
  onClear: () => void;
}

/** Recent searches as one-click chips. */
export function RecentBar({ items, onPick, onClear }: RecentBarProps) {
  if (!items.length) return null;
  return (
    <div className="row nowrap recent-bar">
      <Icon name="history" size={14} />
      <div className="chips grow">
        {items.slice(0, 8).map((r, i) => (
          <button key={i} className="chip" onClick={() => onPick(r.query, r.mode)} title={`${r.mode}${r.rows !== undefined ? ' · ' + r.rows + ' hits' : ''}${r.ms !== undefined ? ' · ' + r.ms + ' ms' : ''}`}>
            {r.query}
          </button>
        ))}
      </div>
      <button className="btn ghost sm" onClick={onClear} aria-label="Clear recent searches">
        ×
      </button>
    </div>
  );
}
