// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { Meta, StoryObj } from '@storybook/react-vite';
import { useState } from 'react';
import { FocusChips } from '../components/graph/FocusChips';
import { ContextControls } from '../components/graph/ContextControls';
import { StatCards } from '../components/graph/StatCards';
import { ConceptBars, DecayScatter, DepthChart } from '../components/graph/Charts';
import { DropZone } from '../components/graph/DropZone';
import { EmbeddingControls } from '../components/graph/EmbeddingControls';
import { AddMemoryForm } from '../components/graph/AddMemoryForm';
import { NewKindForm } from '../components/graph/NewKindForm';
import { Legend } from '../components/graph/Legend';
import { ViewTabs } from '../components/graph/ViewTabs';
import { GraphTab } from '../components/graph/GraphTab';
import { buildIndex } from '../model/graph';
import { fixtureState, fixtureStateEmpty, fixtureStateUnlit } from '../fixtures';
import type { Params } from '../api/types';
import type { ViewName } from '../model/layout';
import { Frame, MockStore, TabFrame, g, log } from './helpers';

const meta: Meta = { title: 'brainDB/Graph/Controls' };
export default meta;
type S = StoryObj;
const st = fixtureState;

export const Focus: S = { render: () => <Frame><FocusChips g={g} focus={st.focus} onSelect={(id) => log('select', id)} onRemove={(id) => log('remove', id)} onGoal={() => log('goal')} onClear={() => log('clear')} /></Frame> };
export const FocusEmpty: S = { render: () => <Frame><FocusChips g={g} focus={[]} onSelect={log} onRemove={log} onGoal={() => log('goal')} /></Frame> };
export const Controls: S = {
  render: () => {
    const [p, setP] = useState<Params>(st.params);
    const [d, setD] = useState({ edgesMode: 'all' as const, colorMode: 'kind' as const, autoRotate: true, showConnectors: true });
    return (
      <Frame>
        <ContextControls params={p} onParams={(x) => setP({ ...p, ...x })} {...d} onDisplay={(x) => setD({ ...d, ...(x as object) })} />
      </Frame>
    );
  },
};
export const Stats: S = { render: () => <Frame><StatCards stats={st.stats} params={st.params} /></Frame> };
export const StatsOverBudget: S = { render: () => <Frame><StatCards stats={{ ...st.stats, tokens: 5230 }} params={st.params} /></Frame> };
export const ChartsLit: S = {
  render: () => (
    <Frame>
      <DepthChart nodes={st.nodes} depth={st.params.depth} />
      <br />
      <DecayScatter nodes={st.nodes} kindColor={(k) => g.kind(k).color} />
      <br />
      <ConceptBars g={g} onSelect={log} />
    </Frame>
  ),
};
export const ChartsUnlit: S = {
  render: () => {
    const gu = buildIndex(fixtureStateUnlit);
    return (
      <Frame>
        <DepthChart nodes={fixtureStateUnlit.nodes} depth={2} />
        <br />
        <DecayScatter nodes={fixtureStateUnlit.nodes} kindColor={(k) => gu.kind(k).color} />
        <br />
        <ConceptBars g={gu} />
      </Frame>
    );
  },
};
export const Drop: S = { render: () => <Frame><DropZone onFiles={(f) => log('files', f.map((x) => x.name))} focus onFocusChange={log} /></Frame> };
export const DropBusy: S = { render: () => <Frame><DropZone onFiles={log} busy /></Frame> };
export const Embedding: S = {
  render: () => {
    const [e, setE] = useState(st.embedding);
    return <Frame><EmbeddingControls embedding={e} onChange={(p) => setE({ ...e, ...p })} /></Frame>;
  },
};
export const AddMemory: S = { render: () => <Frame><AddMemoryForm g={g} onAdd={(...a) => log('remember', ...a)} /></Frame> };
export const NewKind: S = { render: () => <Frame><NewKindForm onAdd={(k) => log('addKind', k)} /></Frame> };
export const LegendBox: S = { render: () => <div style={{ position: 'relative', height: 140, background: 'var(--bg)' }}><Legend kinds={st.kinds} sources={[{ name: 'lab', color: 'hsl(200 70% 62%)' }]} /></div> };
export const Views: S = {
  render: () => {
    const [v, setV] = useState<ViewName>('brain');
    return <Frame width={700}><ViewTabs view={v} onChange={setV} /></Frame>;
  },
};
export const GraphTabFull: S = { name: 'GraphTab (connected)', render: () => <MockStore><TabFrame><GraphTab /></TabFrame></MockStore> };
export const GraphTabMap: S = { name: 'GraphTab (2D map)', render: () => <MockStore initial={{ view: 'map' }}><TabFrame><GraphTab /></TabFrame></MockStore> };
export const GraphTabEmptyBrain: S = { name: 'GraphTab (empty brain)', render: () => <MockStore mock={{ state: fixtureStateEmpty }}><TabFrame><GraphTab /></TabFrame></MockStore> };
export const GraphTabLoading: S = { name: 'GraphTab (loading)', render: () => <MockStore mock={{ hang: true }} initial={{ brain: null }}><TabFrame><GraphTab /></TabFrame></MockStore> };
export const GraphTabError: S = { name: 'GraphTab (backend down)', render: () => <MockStore mock={{ fail: true }} initial={{ brain: null }}><TabFrame><GraphTab /></TabFrame></MockStore> };
