// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { Meta, StoryObj } from '@storybook/react-vite';
import { AccessLegend, LabelChips, UserMenu, ViewAsBanner, ViewAsSelector } from '../components/access/AccessUi';
import { AccessPanels, ExplainAccessTool, GrantEditor } from '../components/access/AccessTab';
import { Workspace } from '../app/Workspace';
import { Brain3D } from '../components/graph/Brain3D';
import { fixtureAccess, fixtureExplainError, fixtureState, fixtureWhoamiAdmin, fixtureWhoamiKim } from '../fixtures';
import { buildIndex } from '../model/graph';
import { Frame, MockStore, log, noop } from './helpers';

const meta: Meta = { title: 'brainDB/Access' };
export default meta;
type S = StoryObj;
const users = fixtureAccess.users.rows.map((r) => String(r.name));

export const WhoamiMenuAdmin: S = { name: 'Whoami menu (admin)', render: () => <div style={{ padding: 10, height: 360, display: 'flex', justifyContent: 'flex-end', background: 'var(--panel)' }}><UserMenu who={fixtureWhoamiAdmin} onSwitchToken={() => log('switch token')} defaultOpen /></div> };
/** What a server with no access control actually sends: the lists come back as JSON null. */
export const WhoamiMenuNullLists: S = {
  name: 'Whoami menu (server sent null lists)',
  render: () => (
    <div style={{ padding: 10, height: 360, display: 'flex', justifyContent: 'flex-end', background: 'var(--panel)' }}>
      <UserMenu who={{ ...fixtureWhoamiAdmin, roles: null, labels: null, dbs: null, defaultWriteLabels: undefined }} onSwitchToken={() => log('switch token')} defaultOpen />
    </div>
  ),
};
export const WhoamiMenuUser: S = { name: 'Whoami menu (kim)', render: () => <div style={{ padding: 10, height: 360, display: 'flex', justifyContent: 'flex-end', background: 'var(--panel)' }}><UserMenu who={fixtureWhoamiKim} onSwitchToken={() => log('switch token')} defaultOpen /></div> };
export const Labels: S = { render: () => <Frame><LabelChips labels={['finance', 'ops', 'security', 'ip-sensitive', 'legal']} /></Frame> };
export const ViewAs: S = {
  name: 'View as… selector + banner',
  render: () => (
    <div style={{ background: 'var(--bg)', height: 200 }}>
      <div className="topbar" style={{ height: 38 }}>
        <span className="spacer" />
        <ViewAsSelector users={users} value="kim" onChange={log} />
      </div>
      <ViewAsBanner user="kim" onExit={() => log('exit')} />
    </div>
  ),
};
export const Panels: S = {
  name: 'Access tab panels',
  render: () => (
    <div className="tab-scroll" style={{ background: 'var(--bg)', minHeight: '100vh' }}>
      <AccessPanels data={fixtureAccess} onRun={(b) => log('run', b)} validate={async () => []} explain={async () => fixtureAccess.explain} onViewAs={(u) => log('view as', u)} />
    </div>
  ),
};
export const GrantEditorValid: S = { render: () => <Frame width={640}><GrantEditor labels={['finance', 'ops']} users={users} roles={['analyst', 'sales-rep']} validate={async () => []} onApply={(b) => log('apply', b)} /></Frame> };
export const GrantEditorInvalid: S = { render: () => <Frame width={640}><GrantEditor labels={['finance']} users={users} roles={['analyst']} validate={async () => fixtureExplainError.errors.slice(0, 1)} onApply={noop} /></Frame> };
export const ExplainAccess: S = { render: () => <Frame width={640}><ExplainAccessTool users={users} run={async () => fixtureAccess.explain} initialNode="n154" initialUser="kim" initialText={fixtureAccess.explain} /></Frame> };

const gi = buildIndex(fixtureState);
const counts = new Map<string, number>();
fixtureState.nodes.forEach((n) => n.labels?.forEach((l) => counts.set(l, (counts.get(l) ?? 0) + 1)));
export const Overlay: S = {
  name: 'Graph 3D access overlay',
  render: (_a, ctx) => (
    <div style={{ height: '100vh', display: 'flex', position: 'relative' }}>
      <Brain3D
        nodes={fixtureState.nodes}
        edges={fixtureState.edges}
        kindColor={(k) => gi.kind(k).color}
        kindLabel={(k) => gi.kind(k).label}
        conceptIds={gi.conceptIds}
        view="brain"
        colorMode="access"
        edgesMode="all"
        showSuperseded={false}
        selected={null}
        focus={fixtureState.focus}
        highlight={[]}
        pathEdges={[]}
        pulses={{ token: 0, items: [] }}
        flyTo={null}
        autoRotate
        theme={(ctx.globals.theme as 'dark') ?? 'dark'}
        embeddingUse="enriched"
        depth={2}
        pickMode={false}
        onSelect={log}
        onActivate={log}
      />
      <AccessLegend labels={[...counts.entries()].map(([label, nodes]) => ({ label, nodes }))} />
    </div>
  ),
};
export const AccessTabFull: S = { name: 'Workspace · Access tab (admin)', render: () => <MockStore initial={{ activeTab: 'access' }}><Workspace /></MockStore> };
export const OverlayInWorkspace: S = { name: 'Workspace · access overlay', render: () => <MockStore initial={{ activeTab: 'graph', accessOverlay: true }}><Workspace /></MockStore> };
export const ViewingAsKim: S = { name: 'Workspace · viewing as kim', render: () => <MockStore onStore={(s) => setTimeout(() => s.setViewAs('kim'), 0)}><Workspace /></MockStore> };
export const NonAdmin: S = { name: 'Workspace · signed in as kim (no Access tab)', render: () => <MockStore mock={{ user: 'kim' }}><Workspace /></MockStore> };
export const WriteDenied: S = {
  name: 'Write denied',
  render: () => (
    <MockStore
      mock={{ user: 'kim', writeDenied: true }}
      onStore={(s) =>
        setTimeout(() => {
          s.select('d125');
          setTimeout(() => s.boost('d125'), 300);
        }, 200)
      }
    >
      <Workspace />
    </MockStore>
  ),
};
