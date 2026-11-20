// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useMemo, useState } from 'react';
import { useStore, useStoreApi } from '../../state/StoreProvider';
import { connectorsVisible } from '../../state/store';
import { VIEW_DESC } from '../../model/layout';
import { Brain3D } from './Brain3D';
import { Legend } from './Legend';
import { ViewTabs } from './ViewTabs';
import { FocusChips } from './FocusChips';
import { ContextControls } from './ContextControls';
import { StatCards } from './StatCards';
import { ConceptBars, DecayScatter, DepthChart } from './Charts';
import { DropZone } from './DropZone';
import { EmbeddingControls } from './EmbeddingControls';
import { AddMemoryForm } from './AddMemoryForm';
import { NewKindForm } from './NewKindForm';
import { labelColor, sourceColor } from './sceneShared';
import { AccessLegend } from '../access/AccessUi';
import { Empty, ErrorBox, Loading, SectionH } from '../common/Async';
import { Icon } from '../common/Icon';

/** Graph 3D workspace tab: controls on the left, the brain canvas on the right. */
export function GraphTab({ hideSide }: { hideSide?: boolean }) {
  const store = useStoreApi();
  const s = useStore((x) => x);
  const [dragOver, setDragOver] = useState(false);
  const g = s.index;
  const serverPositions = s.view === 'embed' ? s.layouts.embed?.positions : s.view === 'tree' ? s.layouts.tree?.positions : undefined;
  const highlight = useMemo(() => [...s.highlight, ...s.agentHighlight], [s.highlight, s.agentHighlight]);
  const sources = useMemo(
    () => [...new Set((s.brain?.nodes ?? []).filter((n) => n.external).map((n) => n.source ?? n.id.split(':')[0]))].map((name) => ({ name, color: sourceColor(name) })),
    [s.brain],
  );
  const accessLabels = (() => {
    const m = new Map<string, number>();
    for (const n of s.brain?.nodes ?? []) for (const l of n.labels ?? []) m.set(l, (m.get(l) ?? 0) + 1);
    return [...m.entries()].map(([label, nodes]) => ({ label, nodes, color: labelColor(label) }));
  })();
  if (!s.brain || !g) return s.error ? <div className="tab-scroll"><ErrorBox error={s.error} onRetry={() => store.init()} /></div> : <Loading text="Loading brain…" />;
  const st = s.brain;
  const showConn = connectorsVisible(s);
  return (
    <div className="graph-split">
      {!hideSide && (
        <aside className="graph-side" aria-label="Context controls">
          <SectionH>Current context</SectionH>
          <div className="goal"><Icon name="goal" size={14} /> {st.goal || <span className="muted">no goal</span>}</div>
          <SectionH>Focus (activation seeds)</SectionH>
          <FocusChips g={g} focus={st.focus} onSelect={(id) => store.select(id, true)} onRemove={(id) => store.removeFocus(id)} onGoal={() => store.focusGoal()} onClear={() => store.setFocus([])} />
          <ContextControls
            params={st.params}
            onParams={(p) => store.patchContext(p, 200)}
            edgesMode={s.edgesMode}
            colorMode={s.colorMode}
            autoRotate={s.autoRotate}
            showConnectors={showConn}
            onDisplay={(p) => store.setDisplay(p)}
          />
          <SectionH>Knowledge and files → brain</SectionH>
          <DropZone onFiles={(f) => store.ingest(f)} busy={s.ingest.loading} focus={s.ingestFocus} onFocusChange={(v) => store.setDisplay({ ingestFocus: v })} />
          <SectionH>Embedding</SectionH>
          <EmbeddingControls embedding={st.embedding} onChange={(p) => store.patchEmbedding(p)} />
          <SectionH>Context statistics</SectionH>
          <StatCards stats={st.stats} params={st.params} />
          <SectionH>Reach by depth</SectionH>
          <DepthChart nodes={st.nodes} depth={st.params.depth} />
          <SectionH>Weight × age (decay)</SectionH>
          <DecayScatter nodes={st.nodes} kindColor={(k) => g.kind(k).color} />
          <SectionH>Concepts in context</SectionH>
          <ConceptBars g={g} onSelect={(id) => store.select(id, true)} />
          <hr style={{ border: 0, borderTop: '1px solid var(--line)', margin: '12px 0' }} />
          <button className="btn primary" style={{ width: '100%', justifyContent: 'center' }} onClick={() => store.discover()}>
            <Icon name="discover" /> Discover new links in context
          </button>
          <details className="fold">
            <summary><Icon name="add" size={14} /> Add memory</summary>
            <AddMemoryForm g={g} onAdd={(k, t, c, f) => store.remember(k, t, c, f)} />
          </details>
          <details className="fold">
            <summary><Icon name="add" size={14} /> New kind</summary>
            <NewKindForm onAdd={(k) => store.addKind(k)} />
          </details>
        </aside>
      )}
      <div
        className="pane"
        style={{ flex: 1, position: 'relative' }}
        onDragEnter={(e) => [...e.dataTransfer.types].includes('Files') && setDragOver(true)}
        onDragOver={(e) => [...e.dataTransfer.types].includes('Files') && e.preventDefault()}
        onDragLeave={(e) => e.currentTarget === e.target && setDragOver(false)}
        onDrop={(e) => {
          e.preventDefault();
          setDragOver(false);
          if (e.dataTransfer.files.length) store.ingest([...e.dataTransfer.files]);
        }}
      >
        <div className="toolbar">
          <ViewTabs view={s.view} onChange={(v) => store.setView(v)} />
          <span className="sep" />
          {s.layouts.loading && <span className="spinner" title="loading layout" />}
          {s.layouts.error && (s.view === 'embed' || s.view === 'tree') && <span className="bad small">layout: {s.layouts.error}</span>}
          {s.whoami?.admin && !s.viewAs && (
            <label className="check small" title="Colour restricted areas by access label">
              <input type="checkbox" checked={s.accessOverlay} onChange={(e) => store.set({ accessOverlay: e.target.checked })} /> <Icon name="lock" size={14} /> Access overlay
            </label>
          )}
          <span className="spacer" />
          <span className="xs muted">
            {st.nodes.length} nodes · {st.edges.length} edges{st.nodes.length > 5000 ? ' · point-cloud mode' : ''}
          </span>
        </div>
        {st.nodes.length === 0 ? (
          <Empty>The brain is empty. Drop files here, or add a memory.</Empty>
        ) : (
          <div style={{ position: 'relative', flex: 1, display: 'flex' }}>
            <Brain3D
              nodes={st.nodes}
              edges={st.edges}
              connectors={st.connectors}
              showConnectors={showConn}
              kindColor={(k) => g.kind(k).color}
              kindLabel={(k) => g.kind(k).label}
              conceptIds={g.conceptIds}
              view={s.view}
              serverPositions={serverPositions}
              treeEdges={s.view === 'tree' ? s.layouts.tree?.treeEdges : undefined}
              colorMode={s.accessOverlay ? 'access' : s.colorMode}
              edgesMode={s.edgesMode}
              showSuperseded={st.params.showSuperseded}
              selected={s.selected}
              focus={st.focus}
              highlight={highlight}
              pathEdges={s.pathEdges}
              pulses={s.pulses}
              flyTo={s.flyTo}
              autoRotate={s.autoRotate}
              theme={s.theme}
              embeddingUse={st.embedding.use}
              depth={st.params.depth}
              pickMode={!!s.pickMode}
              onSelect={(id) => store.select(id)}
              onActivate={(id) => {
                store.setFocus([id]);
                store.select(id, true);
              }}
            />
            <div className="canvas-overlay view-desc">{VIEW_DESC[s.view]}</div>
            <div className="canvas-overlay hud">
              {s.view === 'map'
                ? 'drag: pan · wheel: zoom · click: inspect · double-click: focus · arrows/±: keyboard · Esc: cancel'
                : 'drag: rotate · right/shift-drag: pan · wheel: zoom · click: inspect · double-click: focus · arrows/±: keyboard · Esc: cancel'}
            </div>
            <Legend kinds={[...g.kinds.values()].filter((k) => st.kinds.some((x) => x.name === k.name) || k.name === 'concept')} showConnectors={showConn && !!st.connectors?.length} sources={sources} />
            {s.accessOverlay && <AccessLegend labels={accessLabels} />}
            {s.pickMode && (
              <div className="banner" role="status">
                {s.pickMode.label} — click a node <kbd>Esc</kbd>
                <button className="btn sm" onClick={() => store.cancel()}>
                  Cancel
                </button>
              </div>
            )}
            {dragOver && (
              <div className="drop-ov">
                Drop — files go into the brain
                <br />
                chunks → embedding → auto-links
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
