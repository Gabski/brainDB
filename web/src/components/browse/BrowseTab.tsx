// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useEffect, useMemo, useRef, useState } from 'react';
import type { BrainNode, EntityRow } from '../../api/types';
import { EDGE_GLYPH, isConceptNode, otherEnd, type GraphIndex } from '../../model/graph';
import { browseFacets, browseFilter, browseSort, entityNodes, parseQuery, snippet, SORT_LABELS, isDefaultBrowse, DEFAULT_BROWSE, type BrowseQuery, type Facet, type SortKey } from '../../model/browse';
import { copyText, f2, short } from '../../model/util';
import { useStore, useStoreApi } from '../../state/StoreProvider';
import { HiText } from '../common/HiText';
import { Icon } from '../common/Icon';
import { KindBadge } from '../common/KindBadge';
import { EntityText, entityHue } from '../entities/EntityText';
import { Empty, Loading } from '../common/Async';

const PAGE = 120;

function FacetSection({ title, icon, facets, selected, onToggle, dot, limit = 8 }: { title: string; icon?: React.ReactNode; facets: Facet[]; selected: string[]; onToggle: (v: string) => void; dot?: (v: string) => string | undefined; limit?: number }) {
  const [open, setOpen] = useState(true);
  const [all, setAll] = useState(false);
  const list = all ? facets : facets.slice(0, limit);
  const extra = selected.filter((v) => !list.some((f) => f.value === v)).map((v) => ({ value: v, count: 0 }));
  return (
    <div className="facet">
      <button className="facet-h" aria-expanded={open} onClick={() => setOpen(!open)}>
        <span className="caret">{open ? '▾' : '▸'}</span>
        {icon && <span className="ico">{icon}</span>}
        <span className="lbl">{title}</span>
        <span className="cnt">{facets.length}</span>
      </button>
      {open && (
        <ul className="facet-list">
          {[...list, ...extra].map((f) => {
            const on = selected.includes(f.value);
            const c = dot?.(f.value);
            return (
              <li key={f.value}>
                <button className={'facet-item' + (on ? ' on' : '')} aria-pressed={on} onClick={() => onToggle(f.value)} title={f.value}>
                  {c && <i className="dot" style={{ background: c }} />}
                  <span className="lbl">{f.value}</span>
                  <span className="cnt">{f.count || ''}</span>
                </button>
              </li>
            );
          })}
          {!facets.length && <li className="muted xs" style={{ padding: '2px 8px' }}>none</li>}
          {facets.length > limit && (
            <li>
              <button className="btn ghost sm" style={{ marginLeft: 4 }} onClick={() => setAll(!all)}>
                {all ? 'less' : `${facets.length - limit} more`}
              </button>
            </li>
          )}
        </ul>
      )}
    </div>
  );
}

/** One result row: kind dot, id, matched snippet, sort value. */
function Row({ g, n, terms, sel, cur, onPreview, onOpen }: { g: GraphIndex; n: BrainNode; terms: string[]; sel: boolean; cur: boolean; onPreview: (id: string) => void; onOpen: (id: string) => void }) {
  const s = useMemo(() => snippet(n.text, terms), [n.text, terms]);
  const k = g.kind(n.kind);
  return (
    <li id={'browse-row-' + n.id} role="option" aria-selected={cur}>
      <button
        className={'browse-row' + (cur ? ' cur' : '') + (sel ? ' sel' : '')}
        onClick={() => onPreview(n.id)}
        onDoubleClick={() => onOpen(n.id)}
        title={n.id + ' · ' + n.text}
      >
        <i className={'dot' + (isConceptNode(n) ? ' diamond' : '') + (n.external ? ' ring' : '')} style={{ background: k.color, color: k.color }} />
        <span className="id mono xs muted">{n.id}</span>
        <span className="t">
          <HiText text={s.text} terms={terms} />
        </span>
        <span className="meta">
          {n.pinned && <Icon name="pin" size={14} label="pinned" />}
          {n.supersededBy && <span className="muted xs">⇄</span>}
          {n.source && <span className="muted xs">{short(n.source, 10)}</span>}
          <span className="mono xs">{n.activation !== undefined ? f2(n.activation) : f2(n.weight)}</span>
        </span>
      </button>
    </li>
  );
}

/** Right pane: the whole memory, its fields, entities and neighbours, with wiki-style back. */
function Preview({ g, id, terms, onNavigate, onBack, canBack, onOpen, onFocus, onCopy }: { g: GraphIndex; id: string; terms: string[]; onNavigate: (id: string) => void; onBack: () => void; canBack: boolean; onOpen: (id: string) => void; onFocus: (id: string) => void; onCopy: (id: string) => void }) {
  const n = g.nodes.get(id);
  if (!n) return <Empty>{id} is not in this snapshot.</Empty>;
  const k = g.kind(n.kind);
  const links = (g.adj.get(id) ?? []).slice().sort((a, b) => b.w - a.w);
  const fields = Object.entries(n.fields ?? {});
  return (
    <div className="browse-preview">
      <div className="pane-h">
        {canBack && (
          <button className="btn ghost sm icon" onClick={onBack} aria-label="Back" title="Back">
            ←
          </button>
        )}
        <b className="mono">{n.id}</b>
        <KindBadge label={k.label} color={k.color} title={'kind ' + n.kind} />
        <span className="muted">§ {n.section}</span>
        <span className="spacer" />
        <button className="btn ghost sm icon" onClick={() => onCopy(n.id)} aria-label="Copy id" title="Copy id">
          <Icon name="copy" size={14} />
        </button>
      </div>
      <div className="tab-scroll">
        <p className="browse-text">{n.entities?.length ? <EntityText text={n.text} entities={n.entities} /> : <HiText text={n.text} terms={terms} />}</p>
        <div className="browse-meta">
          <span>weight <b className="mono">{f2(n.weight)}</b></span>
          <span>base <b className="mono">{f2(n.base)}</b></span>
          <span>age <b className="mono">{n.age}d</b></span>
          {n.activation !== undefined && <span>activation <b className="mono">{f2(n.activation)}</b></span>}
          {n.depth !== undefined && <span>depth <b className="mono">d{n.depth}</b></span>}
          {n.pinned && <span className="ok">pinned</span>}
          {n.external && <span className="muted">external · {n.source}</span>}
          {n.supersededBy && (
            <span>
              superseded by{' '}
              <button className="btn ghost sm" onClick={() => onNavigate(n.supersededBy!)}>
                {n.supersededBy}
              </button>
            </span>
          )}
        </div>
        {fields.length > 0 && (
          <table className="browse-fields">
            <tbody>
              {fields.map(([key, v]) => (
                <tr key={key}>
                  <th>{key}</th>
                  <td className="mono">{v}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        {!!n.entities?.length && (
          <div className="chips" style={{ marginTop: 8 }}>
            {n.entities.map((e, i) => (
              <span key={i} className="chip" style={{ borderColor: `hsl(${entityHue(e.type)} 60% 45%)` }} title={e.type}>
                {e.type}: {short(e.value, 24)}
              </span>
            ))}
          </div>
        )}
        <h4 className="section-h">
          Links <span className="count">{links.length}</span>
        </h4>
        <div className="list">
          {links.map((e, i) => {
            const o = otherEnd(e, id);
            const t = g.nodes.get(o);
            return (
              <button key={i} className="li" onClick={() => onNavigate(o)} title={o + (t ? ' · ' + t.text : '')}>
                <i className="dot" style={{ background: g.color(o) }} />
                <span className="t">
                  <span className="muted xs">{EDGE_GLYPH[e.type] ?? '·'} </span>
                  {t ? short(t.text, 54) : o}
                </span>
                <span className="n">{f2(e.w)}</span>
              </button>
            );
          })}
          {!links.length && <span className="muted small">no links</span>}
        </div>
        <div className="row" style={{ marginTop: 10 }}>
          <button className="btn primary sm" onClick={() => onOpen(id)}>
            <Icon name="eye" size={14} /> Open in Inspector
          </button>
          <button className="btn sm" onClick={() => onFocus(id)}>
            <Icon name="focus" size={14} /> Light as focus
          </button>
        </div>
      </div>
    </div>
  );
}

export interface BrowseProps {
  g: GraphIndex;
  query: BrowseQuery;
  onQuery: (p: Partial<BrowseQuery>) => void;
  /** undefined = entities never requested, null = loading. */
  entities?: EntityRow[] | null;
  onLoadEntities?: () => void;
  selected: string | null;
  onSelect: (id: string) => void;
  onFocus?: (ids: string[]) => void;
  onToast?: (text: string, ok?: boolean) => void;
}

/** Faceted browser over the whole snapshot: filters on the left, results in the middle, the memory on the right. */
export function Browse(p: BrowseProps) {
  const { g, query: q } = p;
  const [limit, setLimit] = useState(PAGE);
  const [preview, setPreview] = useState<string | null>(p.selected);
  const [back, setBack] = useState<string[]>([]);
  const listRef = useRef<HTMLUListElement>(null);

  const terms = useMemo(() => parseQuery(q.q).terms, [q.q]);
  const ents = useMemo(() => entityNodes(p.entities, q.entity), [p.entities, q.entity]);
  const rows = useMemo(() => browseSort(browseFilter(g.state.nodes, q, ents), q.sort, q.desc), [g, q, ents]);
  const facets = useMemo(() => browseFacets(g.state.nodes, q, ents, p.entities), [g, q, ents, p.entities]);
  const shown = rows.slice(0, limit);

  useEffect(() => setLimit(PAGE), [q.q, q.kinds, q.sections, q.sources, q.entity, q.pinned, q.concepts, q.superseded, q.external]);
  useEffect(() => {
    if (p.selected) setPreview(p.selected);
  }, [p.selected]);
  // keep the preview on a row that still exists
  useEffect(() => {
    if (preview && !rows.some((r) => r.id === preview) && rows.length) setPreview(null);
  }, [rows]); // eslint-disable-line react-hooks/exhaustive-deps

  const go = (id: string) => {
    if (preview && preview !== id) setBack([...back, preview]);
    setPreview(id);
  };
  const goBack = () => {
    const prev = back[back.length - 1];
    if (prev === undefined) return;
    setBack(back.slice(0, -1));
    setPreview(prev);
  };
  const toggle = (group: 'kinds' | 'sections' | 'sources') => (v: string) => {
    const cur = q[group];
    p.onQuery({ [group]: cur.includes(v) ? cur.filter((x) => x !== v) : [...cur, v] } as Partial<BrowseQuery>);
  };
  const move = (d: 1 | -1) => {
    const i = shown.findIndex((r) => r.id === preview);
    const next = shown[Math.min(Math.max(i + d, 0), shown.length - 1)] ?? shown[0];
    if (!next) return;
    setPreview(next.id);
    listRef.current?.querySelector('#browse-row-' + CSS.escape(next.id))?.scrollIntoView({ block: 'nearest' });
  };

  return (
    <div className="split-v">
      <div className="toolbar">
        <label className="grow row nowrap" style={{ gap: 4 }}>
          <Icon name="filter" size={14} />
          <input
            className="input grow"
            placeholder="Filter memories — words, kind:fact, section:decisions, source:name"
            value={q.q}
            onChange={(e) => p.onQuery({ q: e.target.value })}
            onKeyDown={(e) => {
              if (e.key === 'ArrowDown') (e.preventDefault(), move(1));
              if (e.key === 'Escape' && q.q) (e.preventDefault(), p.onQuery({ q: '' }));
            }}
            aria-label="Filter memories"
          />
        </label>
        <select className="input" style={{ width: 118 }} value={q.sort} onChange={(e) => p.onQuery({ sort: e.target.value as SortKey })} aria-label="Sort by">
          {(Object.keys(SORT_LABELS) as SortKey[]).map((k) => (
            <option key={k} value={k}>
              sort: {SORT_LABELS[k]}
            </option>
          ))}
        </select>
        <button className="btn sm icon" onClick={() => p.onQuery({ desc: !q.desc })} aria-label={q.desc ? 'Descending' : 'Ascending'} title={q.desc ? 'Descending' : 'Ascending'}>
          {q.desc ? '▾' : '▴'}
        </button>
        <span className="sep" />
        <button className={'btn sm' + (q.pinned ? ' on' : '')} aria-pressed={q.pinned} onClick={() => p.onQuery({ pinned: !q.pinned })} title="Only pinned">
          <Icon name="pin" size={14} /> pinned
        </button>
        <button className={'btn sm' + (q.concepts ? ' on' : '')} aria-pressed={q.concepts} onClick={() => p.onQuery({ concepts: !q.concepts })} title="Include concepts">
          ◆ concepts
        </button>
        <button className={'btn sm' + (q.superseded ? ' on' : '')} aria-pressed={q.superseded} onClick={() => p.onQuery({ superseded: !q.superseded })} title="Include superseded">
          ⇄ superseded
        </button>
        <button className={'btn sm' + (q.external ? ' on' : '')} aria-pressed={q.external} onClick={() => p.onQuery({ external: !q.external })} title="Include external stubs">
          <Icon name="source" size={14} /> external
        </button>
        {!isDefaultBrowse(q) && (
          <button className="btn ghost sm" onClick={() => p.onQuery(DEFAULT_BROWSE)}>
            clear
          </button>
        )}
      </div>
      <div className="browse">
        <aside className="browse-facets" aria-label="Filters">
          <FacetSection title="Kinds" facets={facets.kinds} selected={q.kinds} onToggle={toggle('kinds')} dot={(v) => g.kind(v).color} />
          <FacetSection title="Sections" icon="§" facets={facets.sections} selected={q.sections} onToggle={toggle('sections')} />
          <FacetSection title="Sources" icon={<Icon name="source" size={14} />} facets={facets.sources} selected={q.sources} onToggle={toggle('sources')} />
          <div className="facet">
            <button
              className="facet-h"
              onClick={() => {
                if (p.entities === undefined) p.onLoadEntities?.();
                else if (q.entity) p.onQuery({ entity: null });
              }}
            >
              <span className="ico">
                <Icon name="label" size={14} />
              </span>
              <span className="lbl">Entities</span>
              <span className="cnt">{p.entities === undefined ? 'load' : facets.entities.length}</span>
            </button>
            {p.entities === null && <Loading text="entities…" />}
            {p.entities && (
              <ul className="facet-list">
                {facets.entities.slice(0, 12).map((f) => {
                  const type = f.value.slice(0, f.value.indexOf(':'));
                  const on = q.entity === f.value;
                  return (
                    <li key={f.value}>
                      <button className={'facet-item' + (on ? ' on' : '')} aria-pressed={on} onClick={() => p.onQuery({ entity: on ? null : f.value })} title={f.value}>
                        <i className="dot" style={{ background: `hsl(${entityHue(type)} 60% 55%)` }} />
                        <span className="lbl">{f.value}</span>
                        <span className="cnt">{f.count}</span>
                      </button>
                    </li>
                  );
                })}
                {!facets.entities.length && <li className="muted xs" style={{ padding: '2px 8px' }}>none here</li>}
              </ul>
            )}
          </div>
        </aside>
        <div className="pane browse-main">
          <div className="pane-h">
            <b>{rows.length}</b>
            <span className="muted">of {g.state.nodes.length} memories</span>
            {q.entity && (
              <span className="chip on">
                {q.entity}
                <button className="x" aria-label="Clear entity filter" onClick={() => p.onQuery({ entity: null })}>
                  ×
                </button>
              </span>
            )}
            <span className="spacer" />
            <span className="muted xs">↑↓ move · Enter opens</span>
          </div>
          <ul
            className="browse-list"
            role="listbox"
            aria-label="Memories"
            aria-activedescendant={preview ? 'browse-row-' + preview : undefined}
            tabIndex={0}
            ref={listRef}
            onKeyDown={(e) => {
              if (e.key === 'ArrowDown') (e.preventDefault(), move(1));
              else if (e.key === 'ArrowUp') (e.preventDefault(), move(-1));
              else if (e.key === 'Enter' && preview) (e.preventDefault(), p.onSelect(preview));
            }}
          >
            {shown.map((n) => (
              <Row key={n.id} g={g} n={n} terms={terms} sel={p.selected === n.id} cur={preview === n.id} onPreview={go} onOpen={p.onSelect} />
            ))}
            {!rows.length && (
              <li>
                <Empty>Nothing matches. Clear a filter, or widen the flags above.</Empty>
              </li>
            )}
            {rows.length > limit && (
              <li className="browse-more">
                <button className="btn sm" onClick={() => setLimit(limit + PAGE * 2)}>
                  Show {Math.min(PAGE * 2, rows.length - limit)} more ({rows.length - limit} left)
                </button>
              </li>
            )}
          </ul>
        </div>
        {preview ? (
          <Preview
            g={g}
            id={preview}
            terms={terms}
            canBack={back.length > 0}
            onBack={goBack}
            onNavigate={go}
            onOpen={(id) => p.onSelect(id)}
            onFocus={(id) => p.onFocus?.([id])}
            onCopy={(id) => copyText(id).then((ok) => p.onToast?.(ok ? 'Copied ' + id : 'Copy failed', ok))}
          />
        ) : (
          <div className="browse-preview">
            <Empty>Pick a memory to read it here.</Empty>
          </div>
        )}
      </div>
    </div>
  );
}

/** Store-connected Browse tab. */
export function BrowseTab() {
  const store = useStoreApi();
  const g = useStore((s) => s.index);
  const q = useStore((s) => s.browse);
  const entities = useStore((s) => s.entityRows);
  const selected = useStore((s) => s.selected);
  if (!g) return <Loading />;
  return (
    <Browse
      g={g}
      query={q}
      onQuery={(x) => store.setBrowse(x)}
      entities={entities}
      onLoadEntities={() => store.loadEntities()}
      selected={selected}
      onSelect={(id) => store.select(id, true)}
      onFocus={(ids) => store.setFocus(ids)}
      onToast={(t, ok) => store.toastMsg(t, ok)}
    />
  );
}
