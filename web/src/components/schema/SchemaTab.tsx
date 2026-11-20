// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useCallback, useEffect, useMemo, useState } from 'react';
import { useStore, useStoreApi } from '../../state/StoreProvider';
import type { Pattern, ServerStats } from '../../api/types';
import { KindBadge } from '../common/KindBadge';
import { NewKindForm } from '../graph/NewKindForm';
import { StoragePanel } from './StoragePanel';
import { PatternsPanel } from './PatternsPanel';
import { isConceptNode, EDGE_GLYPH } from '../../model/graph';
import { Loading } from '../common/Async';
import { downloadText } from '../../model/util';

/** Schema: kinds, sections, concepts, edge types, connectors (with counts), custom-kind editor, storage, patterns. */
export function SchemaTab() {
  const store = useStoreApi();
  const brain = useStore((s) => s.brain);
  const g = useStore((s) => s.index);
  const [stats, setStats] = useState<{ data?: ServerStats; loading: boolean; error?: string }>({ loading: true });
  const [patterns, setPatterns] = useState<{ data?: Pattern[]; loading: boolean; error?: string }>({ loading: true });
  const [convertNote, setConvertNote] = useState<string | null>(null);
  const loadStats = useCallback(async () => {
    setStats((s) => ({ ...s, loading: true }));
    try {
      setStats({ data: await store.api.stats(), loading: false });
    } catch (e) {
      setStats({ loading: false, error: e instanceof Error ? e.message : String(e) });
    }
  }, [store]);
  const loadPatterns = useCallback(async () => {
    try {
      setPatterns({ data: await store.api.patterns(), loading: false });
    } catch (e) {
      setPatterns({ loading: false, error: e instanceof Error ? e.message : String(e) });
    }
  }, [store]);
  useEffect(() => {
    loadStats();
    loadPatterns();
  }, [loadStats, loadPatterns]);
  const test = useCallback((t: string) => store.api.testPatterns(t), [store]);

  const counts = useMemo(() => {
    const kinds = new Map<string, number>(),
      sections = new Map<string, number>(),
      edges = new Map<string, number>(),
      conceptDeg = new Map<string, number>(),
      ctypes = new Map<string, number>();
    for (const n of brain?.nodes ?? []) {
      kinds.set(n.kind, (kinds.get(n.kind) ?? 0) + 1);
      if (!isConceptNode(n)) sections.set(n.section, (sections.get(n.section) ?? 0) + 1);
    }
    for (const e of brain?.edges ?? []) {
      edges.set(e.type, (edges.get(e.type) ?? 0) + 1);
      for (const x of [e.a, e.b]) if (x.startsWith('A:')) conceptDeg.set(x, (conceptDeg.get(x) ?? 0) + 1);
    }
    for (const c of brain?.connectors ?? []) ctypes.set(c.type, (ctypes.get(c.type) ?? 0) + 1);
    return { kinds, sections, edges, conceptDeg, ctypes };
  }, [brain]);
  if (!brain || !g) return <Loading />;
  const kinds = [...g.kinds.values()].filter((k) => brain.kinds.some((x) => x.name === k.name) || counts.kinds.has(k.name));
  return (
    <div className="tab-scroll">
      <div className="cards">
        <div className="card">
          <div className="card-h">
            Kinds <span className="muted mono">{kinds.length}</span>
          </div>
          <div className="card-b flush">
            <table className="tbl">
              <thead>
                <tr>
                  <th>kind</th>
                  <th>prefix</th>
                  <th>section</th>
                  <th className="num">base</th>
                  <th className="num">half-life</th>
                  <th className="num">nodes</th>
                </tr>
              </thead>
              <tbody>
                {kinds.map((k) => (
                  <tr key={k.name}>
                    <td>
                      <KindBadge label={k.label} color={k.color} /> {!k.builtin && <span className="xs muted">custom</span>}
                    </td>
                    <td className="mono">{k.prefix}</td>
                    <td>{k.section}</td>
                    <td className="num">{k.base.toFixed(2)}</td>
                    <td className="num">{k.halfLife ?? '∞'}</td>
                    <td className="num">{counts.kinds.get(k.name) ?? 0}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
        <div className="card">
          <div className="card-h">Custom kind editor</div>
          <div className="card-b">
            <NewKindForm onAdd={(k) => store.addKind(k)} />
          </div>
        </div>
        <div className="card">
          <div className="card-h">
            Sections <span className="muted mono">{brain.sections.length}</span>
          </div>
          <div className="card-b flush">
            <table className="tbl">
              <thead>
                <tr>
                  <th>section</th>
                  <th className="num">items</th>
                </tr>
              </thead>
              <tbody>
                {[...new Set([...brain.sections, ...counts.sections.keys()])].map((s) => (
                  <tr key={s}>
                    <td>{s}</td>
                    <td className="num">{counts.sections.get(s) ?? 0}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
        <div className="card">
          <div className="card-h">
            Edge types <span className="muted mono">{brain.edges.length}</span>
          </div>
          <div className="card-b flush">
            <table className="tbl">
              <thead>
                <tr>
                  <th>type</th>
                  <th className="num">edges</th>
                </tr>
              </thead>
              <tbody>
                {[...counts.edges.entries()]
                  .sort((a, b) => b[1] - a[1])
                  .map(([t, n]) => (
                    <tr key={t}>
                      <td>
                        <span className="mono">{EDGE_GLYPH[t] ?? '·'}</span> {t}
                      </td>
                      <td className="num">{n}</td>
                    </tr>
                  ))}
              </tbody>
            </table>
          </div>
        </div>
        <div className="card">
          <div className="card-h">
            Concepts <span className="muted mono">{g.conceptIds.length}</span>
          </div>
          <div className="card-b flush">
            <table className="tbl">
              <thead>
                <tr>
                  <th>id</th>
                  <th>label</th>
                  <th className="num">degree</th>
                  <th className="num">act</th>
                </tr>
              </thead>
              <tbody>
                {g.conceptIds.map((id) => {
                  const n = g.nodes.get(id)!;
                  return (
                    <tr key={id} onClick={() => store.select(id, true)} style={{ cursor: 'pointer' }}>
                      <td className="mono">{id}</td>
                      <td>◆ {n.text}</td>
                      <td className="num">{counts.conceptDeg.get(id) ?? 0}</td>
                      <td className="num">{n.activation?.toFixed(2) ?? '–'}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
        <div className="card">
          <div className="card-h">
            Connectors ⧉ <span className="muted mono">{brain.connectors?.length ?? 0}</span>
            <span className="spacer" />
            <span className="xs muted">
              routes used {brain.stats.routesUsed ?? 0} · edges {brain.stats.connectorEdges ?? counts.edges.get('connector') ?? 0}
            </span>
          </div>
          <div className="card-b flush">
            <table className="tbl">
              <thead>
                <tr>
                  <th>type</th>
                  <th className="num">count</th>
                  <th>examples</th>
                </tr>
              </thead>
              <tbody>
                {['anchor', 'bridge', 'hub', 'alias', 'time'].map((t) => (
                  <tr key={t}>
                    <td>{t}</td>
                    <td className="num">{counts.ctypes.get(t) ?? 0}</td>
                    <td className="ellipsis" style={{ maxWidth: 240 }}>
                      {(brain.connectors ?? [])
                        .filter((c) => c.type === t)
                        .slice(0, 3)
                        .map((c) => (
                          <button key={c.id} className="link" style={{ marginRight: 8 }} onClick={() => store.select(c.id)}>
                            {c.label}
                          </button>
                        ))}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
        <div className="card">
          <div className="card-h">Storage & performance</div>
          <div className="card-b">
            <StoragePanel
              stats={stats.data}
              loading={stats.loading}
              error={stats.error}
              onRefresh={loadStats}
              convertNote={convertNote}
              onSnapshot={async () => {
                try {
                  await store.api.snapshot();
                  store.toastMsg('Snapshot written');
                  loadStats();
                } catch (e) {
                  store.toastMsg('snapshot: ' + (e instanceof Error ? e.message : String(e)), false);
                }
              }}
              onMigrate={async (to) => {
                try {
                  await store.api.migrate(to);
                  store.toastMsg('Migrated to ' + to);
                  loadStats();
                } catch (e) {
                  store.toastMsg('migrate: ' + (e instanceof Error ? e.message : String(e)) + ' — or use the CLI: braindb migrate --to ' + to, false);
                }
              }}
              onConvert={async (to) => {
                try {
                  const r = await store.api.convert(to);
                  if (r.unsupported) setConvertNote(`This server cannot convert over HTTP — requires CLI: braindb convert --to ${to}`);
                  else {
                    setConvertNote(null);
                    store.toastMsg('Converted to ' + to);
                    loadStats();
                  }
                } catch (e) {
                  store.toastMsg('convert: ' + (e instanceof Error ? e.message : String(e)), false);
                }
              }}
              onExportXml={async () => {
                try {
                  downloadText('brain.xml', await store.api.getBrainXml(), 'application/xml');
                } catch (e) {
                  store.toastMsg('export: ' + (e instanceof Error ? e.message : String(e)), false);
                }
              }}
            />
          </div>
        </div>
        <div className="card">
          <div className="card-h">Patterns (entities)</div>
          <div className="card-b">
            <PatternsPanel
              patterns={patterns.data}
              loading={patterns.loading}
              error={patterns.error}
              test={test}
              onToggle={async (name, enabled) => {
                try {
                  setPatterns({ data: await store.api.setPatternEnabled(name, enabled), loading: false });
                } catch (e) {
                  store.toastMsg('pattern: ' + (e instanceof Error ? e.message : String(e)), false);
                }
              }}
              onAdd={async (p) => {
                await store.runBdql(`PATTERN ADD ${p.name} '${p.regex.replace(/'/g, "''")}' TYPE ${p.type} PII ${p.pii};`, 'add pattern');
                loadPatterns();
              }}
              onRemove={async (name) => {
                await store.runBdql(`PATTERN REMOVE ${name};`, 'remove pattern');
                loadPatterns();
              }}
            />
          </div>
        </div>
      </div>
    </div>
  );
}
