// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useState } from 'react';
import { useStore, useStoreApi } from '../../state/StoreProvider';
import { isConceptNode } from '../../model/graph';
import { KindBadge } from '../common/KindBadge';
import { Empty, ErrorBox, Loading } from '../common/Async';
import { Section } from '../common/Collapsible';
import { CodeView } from '../common/CodeView';
import { FieldsTable, PropertiesTable, RouteChips } from './PropertiesTable';
import { LinksList, NearestList, VectorBars } from './Lists';
import { ActionBar, type NodeAction } from './ActionBar';
import { ToolOutput } from './ToolOutput';
import { ConnectorInspector } from './ConnectorInspector';
import { RemotePicker } from './RemotePicker';
import { EntitiesTable, EntityText } from '../entities/EntityText';
import { ConfirmDialog } from '../common/Modal';
import { sourceColor } from '../graph/sceneShared';
import { LabelChips } from '../access/AccessUi';
import { Icon } from '../common/Icon';
import { copyText } from '../../model/util';

/** Right-hand Inspector for the selected node or connector. */
export function Inspector() {
  const store = useStoreApi();
  const s = useStore((x) => x);
  const [picker, setPicker] = useState(false);
  const [confirmForget, setConfirmForget] = useState(false);
  const g = s.index;
  const id = s.selected;
  const go = (x: string) => store.select(x, true);

  if (!g || !s.brain) return <div className="insp-body">{s.loading ? <Loading /> : <Empty>No brain loaded.</Empty>}</div>;

  const conn = id ? s.brain.connectors?.find((c) => c.id === id) : undefined;
  if (conn) {
    const members = s.brain.edges.filter((e) => e.type === 'connector' && (e.a === id || e.b === id)).map((e) => (e.a === id ? e.b : e.a));
    return <ConnectorInspector g={g} connector={conn} members={members} onGo={go} onRun={(q, l) => store.runBdql(q, l)} />;
  }

  const n = id ? g.nodes.get(id) : undefined;
  if (!n)
    return (
      <div className="insp-body">
        <Empty>Select a node in the Graph, a grid row, or the Navigator.</Empty>
        <p className="small muted">
          The whole brain is memory, but only a <b>slice</b> is lit: activation starts at the focus and spreads along synapses up to the chosen depth. Each hop weakens the signal: link weight × damping × memory weight
          (decay). It dies below the threshold.
        </p>
        <p className="small muted">
          Memories connect to <b>concepts</b> (◆) with a weight. Through concepts the context reaches facts without a direct association. <b>Superseded</b> versions weigh ×0.3 and barely conduct.
        </p>
        <p className="small muted">
          <b>Connectors</b> (⧉) are structural junctions (anchors, bridges, hubs, aliases, time buckets): they route activation but carry no knowledge.
        </p>
      </div>
    );

  const kind = g.kind(n.kind);
  const item = s.details.data?.item?.node.id === n.id ? s.details.data.item : undefined;
  const entities = item?.entities ?? item?.node.entities ?? n.entities;
  const links = s.details.data?.item?.node.id === n.id ? s.details.data.links : undefined;
  const near = s.details.data?.item?.node.id === n.id ? s.details.data.near : undefined;
  const fields = item?.node.fields ?? n.fields;
  const inFocus = s.brain.focus.includes(n.id);

  const act = (a: NodeAction) => {
    switch (a) {
      case 'focus':
        store.setFocus([n.id]);
        store.select(n.id, true);
        break;
      case 'toggleFocus':
        if (inFocus) store.removeFocus(n.id);
        else store.addFocus(n.id);
        break;
      case 'test':
        store.testLinks(n.id);
        break;
      case 'suggest':
        store.suggest(n.id);
        break;
      case 'path':
      case 'link':
      case 'supersede':
        store.startPick(a, n.id);
        store.openTab('graph');
        break;
      case 'linkExternal':
        setPicker(true);
        break;
      case 'root':
        store.setTreeRoot(n.id);
        break;
      case 'near':
        store.findSimilar(n.id);
        break;
      case 'boost':
        store.boost(n.id);
        break;
      case 'weaken':
        store.weaken(n.id);
        break;
      case 'pin':
        store.togglePin(n.id);
        break;
      case 'forget':
        setConfirmForget(true);
        break;
    }
  };

  return (
    <div>
      <div className="insp-head">
        <div className="row nowrap insp-nav">
          <button className="btn ghost sm icon" onClick={() => store.navBack()} disabled={!s.nav.back.length} aria-label="Back" title={s.nav.back.length ? 'Back to ' + s.nav.back[s.nav.back.length - 1] + ' (Alt+Left)' : 'Back'}>
            ←
          </button>
          <button className="btn ghost sm icon" onClick={() => store.navForward()} disabled={!s.nav.fwd.length} aria-label="Forward" title={s.nav.fwd.length ? 'Forward to ' + s.nav.fwd[s.nav.fwd.length - 1] + ' (Alt+Right)' : 'Forward'}>
            →
          </button>
          <span className="spacer" />
          <button className="btn ghost sm icon" onClick={() => copyText(n.id).then((ok) => store.toastMsg(ok ? 'Copied ' + n.id : 'Copy failed', ok))} aria-label="Copy id" title="Copy id">
            <Icon name="copy" size={14} />
          </button>
        </div>
        <div className="row">
          {n.external ? (
            <span className="kind-badge" style={{ background: sourceColor(n.source ?? n.id.split(':')[0]) }}>
              <Icon name="source" size={14} /> {n.source} · {kind.label}
            </span>
          ) : (
            <KindBadge label={kind.label} color={kind.color} />
          )}
          <b className="mono">{n.id}</b>
          <span className="muted xs">§ {n.section}</span>
          {n.pinned && <Icon name="pin" size={14} label="pinned" />}
          {inFocus && (
            <span className="chip on" style={{ height: 18 }}>
              focus
            </span>
          )}
        </div>
        <div className="itext">
          {isConceptNode(n) ? '◆ ' : ''}
          <EntityText text={n.text} entities={entities} />
        </div>
        {n.route?.length ? (
          <div style={{ marginTop: 6 }}>
            <RouteChips g={g} route={n.route} onGo={go} />
          </div>
        ) : null}
        {n.labels?.length ? (
          <div style={{ marginTop: 6 }}>
            <LabelChips labels={n.labels} />
          </div>
        ) : null}
      </div>
      <div className="insp-body">
        <ActionBar node={n} inFocus={inFocus} onAction={act} busy={s.toolLoading} />
        <div className="field" style={{ marginTop: -4 }}>
          <label className="lbl-row xs" htmlFor="link-w">
            weight for "Link to…" <b>{s.linkWeight.toFixed(2)}</b>
          </label>
          <input id="link-w" type="range" min={0.1} max={1} step={0.05} value={s.linkWeight} onChange={(e) => store.setDisplay({ linkWeight: +e.target.value })} />
        </div>
        {s.toolLoading && <Loading text="Running…" />}
        {s.tool && (
          <ToolOutput
            g={g}
            tool={s.tool}
            depth={s.brain.params.depth}
            threshold={s.brain.params.threshold}
            onGo={go}
            onReplay={(x) => store.testLinks(x)}
            onEdgeAdjust={(a, b, d) => store.adjustEdge(a, b, d)}
            onEdgeRemove={(a, b) => store.removeEdge(a, b)}
            onShowPath={(p) => store.highlightPath(p)}
            onFocusBoth={(ids) => store.setFocus(ids)}
            onPreview={(u, v) => store.previewLink(u, v)}
            onAddLink={(u, v, w) => store.addLink(u, v, w)}
          />
        )}
        <Section id="insp.props" title="Properties">
          <PropertiesTable g={g} node={n} kind={kind} item={item} onGo={go} />
        </Section>
        {fields && Object.keys(fields).length > 0 && (
          <Section id="insp.fields" title="Fields" count={Object.keys(fields).length}>
            <FieldsTable fields={fields} />
          </Section>
        )}
        {entities && (
          <Section id="insp.entities" title="Entities" count={entities.length}>
            <EntitiesTable entities={entities} />
          </Section>
        )}
        <Section id="insp.links" title="Links" count={links?.length}>
          {s.details.loading && !links ? <Loading /> : s.details.error ? <ErrorBox error={s.details.error} /> : <LinksList g={g} links={links ?? []} onGo={go} />}
        </Section>
        <Section id="insp.vector" title={'Vector · ' + s.brain.embedding.model} defaultOpen={false}>
          {s.vector.loading ? <Loading text="computing…" /> : s.vector.error ? <ErrorBox error={s.vector.error} /> : s.vector.data ? <VectorBars vector={s.vector.data} model={s.brain.embedding.model} alpha={s.brain.embedding.alpha} hops={s.brain.embedding.hops} concept={isConceptNode(n)} /> : null}
        </Section>
        <Section id="insp.near" title={`Nearest semantically (${s.brain.embedding.use})`}>
          {near ? <NearestList g={g} near={near} onGo={go} use={s.brain.embedding.use} onLink={(x, cos) => store.addLink(n.id, x, cos, 'similar')} /> : s.details.loading ? <Loading /> : null}
        </Section>
        {item?.xml && (
          <Section id="insp.xml" title="Item XML" defaultOpen={false}>
            <CodeView text={item.xml} lang="xml" maxHeight={240} />
          </Section>
        )}
      </div>
      {picker && (
        <RemotePicker
          from={n.id}
          sources={s.sources.data ?? []}
          initialQuery={n.text.slice(0, 60)}
          search={(src, q) => store.api.remoteSearch(src, q)}
          onPick={(rid, w) => {
            setPicker(false);
            store.linkExternal(n.id, rid, w);
          }}
          onClose={() => setPicker(false)}
        />
      )}
      {confirmForget && (
        <ConfirmDialog
          title={`Forget ${n.id}?`}
          text={
            <>
              This removes <b className="mono">{n.id}</b>
              {n.kind === 'file' ? ' and its chunks' : ''} from the brain. It is journaled, but there is no undo in the UI.
            </>
          }
          confirmLabel="Forget"
          danger
          onCancel={() => setConfirmForget(false)}
          onConfirm={() => {
            setConfirmForget(false);
            store.forget(n.id);
          }}
        />
      )}
    </div>
  );
}
