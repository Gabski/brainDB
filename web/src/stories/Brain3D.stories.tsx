// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { Meta, StoryObj } from '@storybook/react-vite';
import { Brain3D, type Brain3DProps } from '../components/graph/Brain3D';
import { buildIndex } from '../model/graph';
import type { State } from '../api/types';
import { fixtureLayoutEmbed, fixtureLayoutTree, fixtureState, fixtureStateUnlit, makeBigState } from '../fixtures';
import type { ViewName } from '../model/layout';
import { log } from './helpers';

const meta: Meta = { title: 'brainDB/Graph/Brain3D', parameters: { layout: 'fullscreen' } };
export default meta;
type S = StoryObj;

function props(st: State, view: ViewName, over: Partial<Brain3DProps> = {}): Brain3DProps {
  const gi = buildIndex(st);
  return {
    nodes: st.nodes,
    edges: st.edges,
    connectors: st.connectors,
    showConnectors: view !== 'brain',
    kindColor: (k) => gi.kind(k).color,
    kindLabel: (k) => gi.kind(k).label,
    conceptIds: gi.conceptIds,
    view,
    serverPositions: view === 'embed' ? fixtureLayoutEmbed.positions : view === 'tree' ? fixtureLayoutTree.positions : undefined,
    treeEdges: view === 'tree' ? fixtureLayoutTree.treeEdges : undefined,
    colorMode: 'kind',
    edgesMode: 'all',
    showSuperseded: false,
    selected: null,
    focus: st.focus,
    highlight: [],
    pathEdges: [],
    pulses: { token: 0, items: [] },
    flyTo: null,
    autoRotate: true,
    theme: (document.documentElement.dataset.theme as 'dark' | 'light') ?? 'dark',
    embeddingUse: 'enriched',
    depth: st.params.depth,
    pickMode: false,
    onSelect: (id) => log('select', id),
    onActivate: (id) => log('activate', id),
    ...over,
  };
}
const View = (p: Brain3DProps) => (
  <div style={{ height: '100vh', display: 'flex' }}>
    <Brain3D {...p} />
  </div>
);
const themed = (st: State, view: ViewName, over?: Partial<Brain3DProps>) => ({
  render: (_: unknown, ctx: { globals: Record<string, unknown> }) => <View {...props(st, view, { theme: (ctx.globals.theme as 'dark' | 'light') ?? 'dark', ...over })} />,
});

export const BrainView: S = themed(fixtureState, 'brain');
export const LayersView: S = themed(fixtureState, 'layers');
export const DepthSphereView: S = themed(fixtureState, 'sphere');
export const DecayView: S = themed(fixtureState, 'time');
export const VectorsView: S = themed(fixtureState, 'embed');
export const TreeView: S = themed(fixtureState, 'tree');
export const Unlit: S = themed(fixtureStateUnlit, 'brain');
export const ColourByDepth: S = themed(fixtureState, 'layers', { colorMode: 'depth' });
export const ConnectorsShown: S = themed(fixtureState, 'layers', { showConnectors: true, selected: 'H:entra' });
export const PathWithPulses: S = themed(fixtureState, 'brain', {
  selected: 'd121',
  pathEdges: [
    ['d121', 'A:idp'],
    ['A:idp', 'A:auth'],
    ['A:auth', 't132'],
  ],
  pulses: {
    token: 1,
    period: 2.2,
    items: [
      { from: 'd121', to: 'A:idp', delay: 0, dur: 0.45, color: '#ffd43b', loop: true, size: 7 },
      { from: 'A:idp', to: 'A:auth', delay: 0.45, dur: 0.45, color: '#ffd43b', loop: true, size: 7 },
      { from: 'A:auth', to: 't132', delay: 0.9, dur: 0.45, color: '#ffd43b', loop: true, size: 7 },
    ],
  },
});
export const AgentHighlight: S = themed(fixtureState, 'brain', { highlight: ['x180', 'c181', 'c182', 'c183', 'k175'] });
export const EdgesContextOnly: S = themed(fixtureState, 'brain', { edgesMode: 'ctx' });
const big = makeBigState(8000);
export const BigGraphPointCloud: S = { name: 'Big graph (8000 nodes, point cloud)', ...themed(big, 'brain', { autoRotate: false }) };
