// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useCallback, useEffect, useState } from 'react';
import { useApi } from '../../api/ApiProvider';
import { useStore, useStoreApi } from '../../state/StoreProvider';
import type { ContextFormat } from '../../api/types';
import { ContextViewer } from './ContextViewer';
import { FocusChips } from '../graph/FocusChips';
import { ContextControls } from '../graph/ContextControls';
import { StatCards } from '../graph/StatCards';
import { CodeView } from '../common/CodeView';
import { ErrorBox, Loading, SectionH } from '../common/Async';
import { NodeRow } from '../common/NodeRow';
import { RouteChips } from '../inspector/PropertiesTable';
import { isConceptNode, isLit } from '../../model/graph';
import { Icon } from '../common/Icon';

/** Context tab: shared focus/params + the rendered slice; or an ad-hoc context query that does not change the focus. */
export function ContextTab() {
  const api = useApi();
  const store = useStoreApi();
  const brain = useStore((s) => s.brain);
  const g = useStore((s) => s.index);
  const revision = useStore((s) => s.revision);
  const purpose = useStore((s) => s.purpose.data);
  const [mode, setMode] = useState<'shared' | 'adhoc'>('shared');
  const [format, setFormat] = useState<'xml' | 'md'>('xml');
  const [text, setText] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [goal, setGoal] = useState(brain?.goal ?? '');
  useEffect(() => setGoal(brain?.goal ?? ''), [brain?.goal]);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setText(await api.contextText(format));
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, [api, format]);
  useEffect(() => {
    if (mode === 'shared') load();
  }, [load, revision, mode]);

  if (!brain || !g) return <Loading />;
  const lit = brain.nodes.filter((n) => isLit(n) && !isConceptNode(n)).sort((a, b) => (b.activation ?? 0) - (a.activation ?? 0));
  return (
    <div className="split-h">
      <aside className="graph-side" style={{ width: 300 }}>
        <div className="seg" style={{ marginBottom: 10 }}>
          <button aria-pressed={mode === 'shared'} onClick={() => setMode('shared')}>
            Shared context
          </button>
          <button aria-pressed={mode === 'adhoc'} onClick={() => setMode('adhoc')}>
            Ad-hoc query
          </button>
        </div>
        {mode === 'shared' ? (
          <>
            <SectionH>Goal</SectionH>
            <form
              className="row nowrap"
              onSubmit={(e) => {
                e.preventDefault();
                store.setGoal(goal);
              }}
            >
              <input className="input grow" aria-label="Goal" value={goal} onChange={(e) => setGoal(e.target.value)} />
              <button className="btn" disabled={goal === brain.goal}>
                Set
              </button>
            </form>
            <SectionH>Focus</SectionH>
            <FocusChips g={g} focus={brain.focus} onSelect={(id) => store.select(id, true)} onRemove={(id) => store.removeFocus(id)} onGoal={() => store.focusGoal()} onClear={() => store.setFocus([])} />
            <ContextControls params={brain.params} onParams={(p) => store.patchContext(p, 200)} />
            <SectionH>Statistics</SectionH>
            <StatCards stats={brain.stats} params={brain.params} />
            <SectionH count={lit.length}>Lit items</SectionH>
            <div className="list">
              {lit.map((n) => (
                <div key={n.id}>
                  <NodeRow g={g} id={n.id} showId onGo={(id) => store.select(id, true)} right={`d${n.depth} · ${n.activation?.toFixed(2)}`} />
                  {n.route?.length ? (
                    <div style={{ padding: '0 0 4px 22px' }}>
                      <RouteChips g={g} route={n.route} onGo={(id) => store.select(id)} />
                    </div>
                  ) : null}
                </div>
              ))}
            </div>
          </>
        ) : (
          <AdhocForm />
        )}
      </aside>
      <div className="pane" style={{ flex: 1 }}>
      {purpose && (
        <div className="purpose-head" title={purpose.description}>
          <Icon name="goal" size={14} />
          <b>{purpose.name}</b>
          <span className="muted ellipsis grow">{purpose.description}</span>
          <span className="xs muted mono">admission {purpose.admission.mode}</span>
        </div>
      )}
      {mode === 'shared' ? <ContextViewer format={format} onFormat={setFormat} text={text} loading={loading} error={error} onRefresh={load} /> : <AdhocResult />}
      </div>
    </div>
  );
}

// ad-hoc context query (A3) keeps its own small state in module scope via a hook pair
let adhocListeners: (() => void)[] = [];
let adhoc: { query: string; depth: number; format: ContextFormat; out: string | null; outFormat: string; loading: boolean; error: string | null } = {
  query: 'guest access policy',
  depth: 2,
  format: 'md',
  out: null,
  outFormat: 'md',
  loading: false,
  error: null,
};
const setAdhoc = (p: Partial<typeof adhoc>) => {
  adhoc = { ...adhoc, ...p };
  adhocListeners.forEach((l) => l());
};
function useAdhoc() {
  const [, force] = useState(0);
  useEffect(() => {
    const l = () => force((x) => x + 1);
    adhocListeners.push(l);
    return () => {
      adhocListeners = adhocListeners.filter((x) => x !== l);
    };
  }, []);
  return adhoc;
}

function AdhocForm() {
  const api = useApi();
  const a = useAdhoc();
  const run = async () => {
    setAdhoc({ loading: true, error: null });
    try {
      const r = await api.contextQuery({ query: a.query, depth: a.depth, format: a.format });
      setAdhoc({ out: r.output, outFormat: r.format, loading: false });
    } catch (e) {
      setAdhoc({ error: e instanceof Error ? e.message : String(e), loading: false });
    }
  };
  return (
    <form
      className="col"
      onSubmit={(e) => {
        e.preventDefault();
        run();
      }}
    >
      <div className="xs muted">An ad-hoc lit slice from a question. It does not change the shared focus.</div>
      <label className="field">
        <span className="lbl-row">query text</span>
        <textarea className="input" rows={3} value={a.query} onChange={(e) => setAdhoc({ query: e.target.value })} />
      </label>
      <label className="field">
        <span className="lbl-row">
          depth <b>{a.depth}</b>
        </span>
        <input type="range" min={0} max={6} step={1} value={a.depth} onChange={(e) => setAdhoc({ depth: +e.target.value })} />
      </label>
      <label className="row small">
        format
        <select className="input" value={a.format} onChange={(e) => setAdhoc({ format: e.target.value as ContextFormat })}>
          {['md', 'xml', 'json', 'yaml'].map((f) => (
            <option key={f}>{f}</option>
          ))}
        </select>
      </label>
      <button className="btn primary" type="submit" disabled={a.loading}>
        <Icon name="play" /> Build slice
      </button>
    </form>
  );
}

function AdhocResult() {
  const a = useAdhoc();
  return (
    <div className="tab-scroll">
      {a.loading ? <Loading /> : a.error ? <ErrorBox error={a.error} /> : a.out !== null ? <CodeView text={a.out} lang={a.outFormat === 'md' ? 'md' : a.outFormat === 'xml' ? 'xml' : a.outFormat === 'json' ? 'json' : 'yaml'} /> : <div className="empty">Enter a question and build the slice.</div>}
    </div>
  );
}
