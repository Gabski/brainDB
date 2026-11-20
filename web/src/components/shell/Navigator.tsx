// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useMemo, useState, type ReactNode } from 'react';
import type { EntityRow, Source, Template } from '../../api/types';
import { isConceptNode, type GraphIndex } from '../../model/graph';
import type { HistoryEntry, SavedQuery } from '../../state/store';
import { short } from '../../model/util';
import { entityHue } from '../entities/EntityText';
import { Icon } from '../common/Icon';

export interface NavigatorProps {
  g: GraphIndex | null;
  selected: string | null;
  sources?: Source[];
  templates?: Template[];
  saved: SavedQuery[];
  history: HistoryEntry[];
  entities?: EntityRow[] | null;
  onSelect: (id: string) => void;
  onKind?: (kind: string) => void;
  onSource?: (name: string) => void;
  onTemplate?: (t: Template) => void;
  onQuery?: (text: string) => void;
  onDeleteSaved?: (name: string) => void;
  onLoadEntities?: () => void;
  defaultOpen?: string[];
}

function Item({ depth, label, count, icon, sel, onClick, open, onToggle, children, title }: { depth: number; label: ReactNode; count?: number | string; icon?: ReactNode; sel?: boolean; onClick?: () => void; open?: boolean; onToggle?: () => void; children?: ReactNode; title?: string }) {
  const hasKids = onToggle !== undefined;
  return (
    <li role="treeitem" aria-expanded={hasKids ? open : undefined} aria-selected={sel}>
      <button
        className={'tree-item' + (sel ? ' sel' : '')}
        style={{ paddingLeft: 6 + depth * 12 }}
        title={title}
        onClick={() => {
          if (hasKids) onToggle!();
          onClick?.();
        }}
        onKeyDown={(e) => {
          if (!hasKids) return;
          if (e.key === 'ArrowRight' && !open) onToggle!();
          if (e.key === 'ArrowLeft' && open) onToggle!();
        }}
      >
        <span className="caret">{hasKids ? (open ? '▾' : '▸') : ''}</span>
        {icon && <span className="ico">{icon}</span>}
        <span className="lbl">{label}</span>
        {count !== undefined && <span className="cnt">{count}</span>}
      </button>
      {hasKids && open && <ul role="group">{children}</ul>}
    </li>
  );
}

const MAX = 200;

/** Left Navigator tree: Brain → sections → items, kinds, concepts, connectors, entities, sources, templates, saved queries, history. */
export function Navigator(p: NavigatorProps) {
  const [open, setOpen] = useState<Set<string>>(new Set(p.defaultOpen ?? ['brain', 'sources', 'saved']));
  const t = (k: string) => () => setOpen((s) => {
    const n = new Set(s);
    if (n.has(k)) n.delete(k);
    else n.add(k);
    return n;
  });
  const o = (k: string) => open.has(k);
  const g = p.g;
  const bySection = useMemo(() => {
    const m = new Map<string, string[]>();
    for (const n of g?.state.nodes ?? []) {
      if (isConceptNode(n) || n.external) continue;
      if (!m.has(n.section)) m.set(n.section, []);
      m.get(n.section)!.push(n.id);
    }
    return m;
  }, [g]);
  const byKind = useMemo(() => {
    const m = new Map<string, number>();
    for (const n of g?.state.nodes ?? []) m.set(n.kind, (m.get(n.kind) ?? 0) + 1);
    return m;
  }, [g]);
  const connByType = useMemo(() => {
    const m = new Map<string, { id: string; label: string; degree: number }[]>();
    for (const c of g?.state.connectors ?? []) {
      if (!m.has(c.type)) m.set(c.type, []);
      m.get(c.type)!.push(c);
    }
    return m;
  }, [g]);
  const ents = useMemo(() => {
    const m = new Map<string, Map<string, Set<string>>>();
    for (const r of p.entities ?? []) {
      if (!m.has(r.type)) m.set(r.type, new Map());
      const v = m.get(r.type)!;
      if (!v.has(r.value)) v.set(r.value, new Set());
      v.get(r.value)!.add(r.node);
    }
    return m;
  }, [p.entities]);
  const ext = (g?.state.nodes ?? []).filter((n) => n.external);
  const dot = (id: string) => <i className="dot" style={{ background: g?.color(id) }} />;

  return (
    <nav className="navigator" aria-label="Navigator">
      <ul className="tree" role="tree">
        <Item depth={0} label={g?.state.name ?? 'Brain'} icon={<Icon name="brain" size={14} />} count={g ? g.state.stats.memories : undefined} open={o('brain')} onToggle={t('brain')}>
          {[...bySection.entries()].map(([sec, ids]) => (
            <Item key={sec} depth={1} label={sec} icon="§" count={ids.length} open={o('sec:' + sec)} onToggle={t('sec:' + sec)}>
              {ids.slice(0, MAX).map((id) => (
                <Item key={id} depth={2} icon={dot(id)} label={<><span className="mono xs muted">{id}</span> {short(g!.nodes.get(id)!.text, 40)}</>} sel={p.selected === id} onClick={() => p.onSelect(id)} title={g!.nodes.get(id)!.text} />
              ))}
              {ids.length > MAX && <li className="muted xs" style={{ paddingLeft: 40 }}>… {ids.length - MAX} more (use Query)</li>}
            </Item>
          ))}
        </Item>
        <Item depth={0} label="Kinds" count={g?.kinds.size} open={o('kinds')} onToggle={t('kinds')}>
          {[...(g?.kinds.values() ?? [])].map((k) => (
            <Item key={k.name} depth={1} icon={<i className="dot" style={{ background: k.color }} />} label={k.label} count={byKind.get(k.name) ?? 0} onClick={() => p.onKind?.(k.name)} />
          ))}
        </Item>
        <Item depth={0} label="Concepts" count={g?.conceptIds.length} open={o('concepts')} onToggle={t('concepts')}>
          {(g?.conceptIds ?? []).map((id) => (
            <Item key={id} depth={1} icon="◆" label={g!.nodes.get(id)!.text} count={g!.nodes.get(id)!.activation?.toFixed(2)} sel={p.selected === id} onClick={() => p.onSelect(id)} />
          ))}
        </Item>
        <Item depth={0} label="Connectors" count={g?.state.connectors?.length ?? 0} open={o('conn')} onToggle={t('conn')}>
          {[...connByType.entries()].map(([type, list]) => (
            <Item key={type} depth={1} icon="⧉" label={type} count={list.length} open={o('ct:' + type)} onToggle={t('ct:' + type)}>
              {list.map((c) => (
                <Item key={c.id} depth={2} label={c.label} count={c.degree} sel={p.selected === c.id} onClick={() => p.onSelect(c.id)} title={c.id} />
              ))}
            </Item>
          ))}
        </Item>
        <Item
          depth={0}
          label="Entities"
          count={p.entities ? ents.size : undefined}
          open={o('ents')}
          onToggle={() => {
            if (!o('ents') && !p.entities) p.onLoadEntities?.();
            t('ents')();
          }}
        >
          {p.entities === null && <li className="muted xs" style={{ paddingLeft: 24 }}>loading…</li>}
          {[...ents.entries()].map(([type, values]) => (
            <Item key={type} depth={1} icon={<span className="dot" style={{ background: `hsl(${entityHue(type)} 60% 55%)` }} />} label={type} count={values.size} open={o('et:' + type)} onToggle={t('et:' + type)}>
              {[...values.entries()]
                .sort((a, b) => b[1].size - a[1].size)
                .slice(0, 20)
                .map(([value, nodes]) => (
                  <Item key={value} depth={2} label={<span className="mono xs">{value}</span>} count={nodes.size} open={o(`ev:${type}:${value}`)} onToggle={t(`ev:${type}:${value}`)}>
                    {[...nodes].map((id) => (
                      <Item key={id} depth={3} icon={dot(id)} label={<><span className="mono xs muted">{id}</span> {short(g?.nodes.get(id)?.text ?? '', 30)}</>} sel={p.selected === id} onClick={() => p.onSelect(id)} />
                    ))}
                  </Item>
                ))}
            </Item>
          ))}
        </Item>
        <Item depth={0} label="Sources" count={p.sources?.length ?? 0} open={o('sources')} onToggle={t('sources')}>
          {(p.sources ?? []).map((s) => (
            <Item key={s.name} depth={1} icon={<span className={'dot ' + s.status} />} label={<span className="mono">{s.name}</span>} count={s.status === 'ok' ? `${s.latencyMs}ms` : s.status} onClick={() => p.onSource?.(s.name)} title={s.uri}>
            </Item>
          ))}
          {ext.length > 0 && (
            <Item depth={1} label="external stubs" count={ext.length} open={o('ext')} onToggle={t('ext')}>
              {ext.map((n) => (
                <Item key={n.id} depth={2} icon={<Icon name="source" size={14} />} label={<><span className="mono xs muted">{n.id}</span> {short(n.text, 30)}</>} sel={p.selected === n.id} onClick={() => p.onSelect(n.id)} />
              ))}
            </Item>
          )}
        </Item>
        <Item depth={0} label="Templates" count={p.templates?.length ?? 0} open={o('templates')} onToggle={t('templates')}>
          {(p.templates ?? []).map((tp) => (
            <Item key={tp.path} depth={1} icon={<Icon name="file" size={14} />} label={tp.name} count={tp.format} onClick={() => p.onTemplate?.(tp)} title={tp.path} />
          ))}
        </Item>
        <Item depth={0} label="Saved queries" count={p.saved.length} open={o('saved')} onToggle={t('saved')}>
          {p.saved.map((q) => (
            <li key={q.name} className="row nowrap" style={{ paddingRight: 4 }}>
              <button className="tree-item" style={{ paddingLeft: 30 }} onClick={() => p.onQuery?.(q.text)} title={q.text}>
                <span className="ico"><Icon name="star" size={14} /></span>
                <span className="lbl">{q.name}</span>
              </button>
              {p.onDeleteSaved && (
                <button className="btn ghost sm icon" aria-label={'Delete ' + q.name} onClick={() => p.onDeleteSaved!(q.name)}>
                  ×
                </button>
              )}
            </li>
          ))}
          {!p.saved.length && <li className="muted xs" style={{ paddingLeft: 30 }}>none yet — use Save in the Query tab</li>}
        </Item>
        <Item depth={0} label="History" count={p.history.length} open={o('history')} onToggle={t('history')}>
          {p.history.slice(0, 30).map((h) => (
            <Item key={h.ts} depth={1} icon={<Icon name={h.ok ? 'check' : 'close'} size={14} />} label={<span className="mono xs">{short(h.text.replace(/\s+/g, ' '), 44)}</span>} count={h.ms !== undefined ? h.ms + 'ms' : undefined} onClick={() => p.onQuery?.(h.text)} title={h.text} />
          ))}
        </Item>
      </ul>
    </nav>
  );
}
