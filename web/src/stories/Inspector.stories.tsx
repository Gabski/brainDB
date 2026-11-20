// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { Meta, StoryObj } from '@storybook/react-vite';
import { PropertiesTable, FieldsTable, RouteChips } from '../components/inspector/PropertiesTable';
import { LinksList, NearestList, VectorBars } from '../components/inspector/Lists';
import { ActionBar } from '../components/inspector/ActionBar';
import { ToolOutput } from '../components/inspector/ToolOutput';
import { ConnectorInspector } from '../components/inspector/ConnectorInspector';
import { RemotePicker } from '../components/inspector/RemotePicker';
import { Inspector } from '../components/inspector/Inspector';
import { fixtureInspect, fixtureInspectRecord, fixtureRemoteHits, fixtureSources, fixtureState, fixtureVector } from '../fixtures';
import { Frame, MockStore, g, log, noop } from './helpers';
import type { BrainStore } from '../state/store';

const meta: Meta = { title: 'brainDB/Inspector' };
export default meta;
type S = StoryObj;
const d121 = g.nodes.get('d121')!;
const r193 = g.nodes.get('r193')!;

export const Properties: S = { render: () => <Frame><PropertiesTable g={g} node={d121} kind={g.kind('decision')} item={fixtureInspect.item} onGo={log} /></Frame> };
export const PropertiesSuperseded: S = { render: () => <Frame><PropertiesTable g={g} node={g.nodes.get('d120')!} kind={g.kind('decision')} onGo={log} /></Frame> };
export const PropertiesExternal: S = { render: () => <Frame><PropertiesTable g={g} node={g.nodes.get('lab:x204')!} kind={g.kind('fact')} onGo={log} /></Frame> };
export const PropertiesRecordWithRoute: S = { render: () => <Frame><PropertiesTable g={g} node={r193} kind={g.kind('record')} item={fixtureInspectRecord.item} onGo={log} /><br /><FieldsTable fields={r193.fields!} /></Frame> };
export const Routes: S = { render: () => <Frame><RouteChips g={g} route={['H:entra', 'H:sso-rollout']} onGo={log} /></Frame> };
export const Links: S = { render: () => <Frame><LinksList g={g} links={fixtureInspect.links!} onGo={log} /></Frame> };
export const Nearest: S = { render: () => <Frame><NearestList g={g} near={fixtureInspect.near!} onGo={log} onLink={(id, c) => log('link', id, c)} use="enriched" /></Frame> };
export const Vector: S = { render: () => <Frame><VectorBars vector={fixtureVector} model="hash-256" alpha={0.5} hops={2} /></Frame> };
export const Actions: S = { render: () => <Frame><ActionBar node={d121} inFocus onAction={(a) => log('action', a)} /></Frame> };
export const ActionsConcept: S = { render: () => <Frame><ActionBar node={g.nodes.get('A:idp')!} inFocus={false} onAction={(a) => log('action', a)} /></Frame> };
const tp = { g, depth: 2, threshold: 0.16, onGo: log, onReplay: log, onEdgeAdjust: (...a: unknown[]) => log('edge', ...a), onEdgeRemove: (...a: unknown[]) => log('rm', ...a), onShowPath: log, onFocusBoth: log, onPreview: (...a: unknown[]) => log('preview', ...a), onAddLink: (...a: unknown[]) => log('add', ...a) };
export const ToolTestLinks: S = { render: () => <Frame width={380}><ToolOutput {...tp} tool={{ type: 'test', id: 'd121', data: fixtureInspect.test! }} /></Frame> };
export const ToolSuggest: S = { render: () => <Frame width={380}><ToolOutput {...tp} tool={{ type: 'suggest', id: 'd121', data: fixtureInspect.suggest! }} /></Frame> };
export const ToolPaths: S = { render: () => <Frame width={380}><ToolOutput {...tp} tool={{ type: 'paths', from: 'd121', to: 't132', data: fixtureInspect.paths! }} /></Frame> };
export const ToolNoPath: S = { render: () => <Frame width={380}><ToolOutput {...tp} tool={{ type: 'paths', from: 'd121', to: 'f201', data: [] }} /></Frame> };
export const ToolMessage: S = { render: () => <Frame width={380}><ToolOutput {...tp} tool={{ type: 'message', title: 'Memory added', text: 'remember ok\nlinked to 2 concepts', ok: true }} /></Frame> };
const conn = fixtureState.connectors!.find((c) => c.id === 'H:entra')!;
const members = fixtureState.edges.filter((e) => e.type === 'connector' && e.a === conn.id).map((e) => e.b);
export const Connector: S = { render: () => <Frame><ConnectorInspector g={g} connector={conn} members={members} onGo={log} onRun={(q) => log('bdql', q)} /></Frame> };
export const ConnectorHub: S = {
  render: () => {
    const c = fixtureState.connectors!.find((x) => x.type === 'hub')!;
    return <Frame><ConnectorInspector g={g} connector={c} members={fixtureState.edges.filter((e) => e.type === 'connector' && e.a === c.id).map((e) => e.b)} onGo={log} onRun={(q) => log('bdql', q)} /></Frame>;
  },
};
export const LinkExternalPicker: S = { render: () => <RemotePicker from="d121" sources={fixtureSources} search={async () => fixtureRemoteHits} initialQuery="token endpoint" initialHits={fixtureRemoteHits} onPick={(...a) => log('pick', ...a)} onClose={noop} /> };
export const LinkExternalNoSources: S = { render: () => <RemotePicker from="d121" sources={[]} search={async () => []} onPick={noop} onClose={noop} /> };

const sel = (id: string) => (s: BrainStore) => setTimeout(() => s.select(id), 0);
const Connected = ({ id, mock }: { id?: string; mock?: object }) => (
  <MockStore onStore={id ? sel(id) : undefined} mock={mock}>
    <div className="inspector" style={{ width: 340, height: '100vh' }}>
      <Inspector />
    </div>
  </MockStore>
);
export const InspectorEmpty: S = { render: () => <Connected /> };
export const InspectorDecision: S = { render: () => <Connected id="d121" /> };
export const InspectorRecordWithFields: S = { render: () => <Connected id="r193" /> };
export const InspectorEntities: S = { render: () => <Connected id="n154" /> };
export const InspectorConnector: S = { render: () => <Connected id="H:bridge-lab" /> };
export const InspectorExternal: S = { render: () => <Connected id="lab:f310" /> };
/** Two hops of history, so Back / Forward in the header are live. */
export const InspectorHistory: S = {
  name: 'Inspector (back / forward)',
  render: () => (
    <MockStore
      onStore={(s: BrainStore) =>
        setTimeout(() => {
          s.select('d121');
          s.select('t132');
          s.select('n154');
        }, 0)
      }
    >
      <div className="inspector" style={{ width: 340, height: '100vh' }}>
        <Inspector />
      </div>
    </MockStore>
  ),
};
export const InspectorLoading: S = { render: () => <MockStore mock={{ hang: true }} init={false} onStore={sel('d121')}><div className="inspector" style={{ width: 340, height: '100vh' }}><Inspector /></div></MockStore> };
export const InspectorError: S = { render: () => <MockStore mock={{ fail: 'inspect' }} init={false} onStore={sel('d121')}><div className="inspector" style={{ width: 340, height: '100vh' }}><Inspector /></div></MockStore> };
