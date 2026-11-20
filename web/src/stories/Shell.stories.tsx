// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { Meta, StoryObj } from '@storybook/react-vite';
import { TopBar } from '../components/shell/TopBar';
import { ConnectionSelector } from '../components/shell/ConnectionSelector';
import { Navigator } from '../components/shell/Navigator';
import { TabBar } from '../components/shell/TabBar';
import { StatusBar } from '../components/shell/StatusBar';
import { CommandPalette } from '../components/shell/CommandPalette';
import { TokenPrompt } from '../components/shell/TokenPrompt';
import { AboutDialog } from '../components/shell/AboutDialog';
import { fixtureDbs, fixtureEntityRows, fixtureSources, fixtureState, fixtureTemplates } from '../fixtures';
import { initialUiState } from '../state/store';
import { g, log, noop } from './helpers';

const meta: Meta = { title: 'brainDB/Shell' };
export default meta;
type S = StoryObj;

const conn = (open = false) => <ConnectionSelector current="sso" dbs={fixtureDbs} sources={fixtureSources} live="live" onSwitchDb={log} onNewDb={() => log('new db')} onAddSource={() => log('add source')} defaultOpen={open} />;
/** Server started with --brain: it hosts one database and cannot create another. */
const connSingle = () => <ConnectionSelector current="brain" dbs={[]} sources={[]} live="live" onSwitchDb={log} onNewDb={() => log('new db')} canManageDbs={false} defaultOpen />;
export const Top: S = { render: () => <div style={{ height: 300 }}><div className="topbar" style={{ height: 38 }}><TopBar connection={conn()} theme="dark" onTheme={log} onPalette={() => log('palette')} onAbout={() => log('about')} /></div></div> };
export const ConnectionMenuOpen: S = { render: () => <div style={{ padding: 10, height: 400, background: 'var(--panel)' }}>{conn(true)}</div> };
export const ConnectionMenuSingleBrain: S = {
  name: 'Connection menu (server started with --brain)',
  render: () => <div style={{ padding: 10, height: 300, background: 'var(--panel)' }}>{connSingle()}</div>,
};
export const NavigatorTree: S = {
  render: () => (
    <div className="navigator" style={{ width: 260, height: '100vh' }}>
      <Navigator
        g={g}
        selected="d121"
        sources={fixtureSources}
        templates={fixtureTemplates}
        saved={[{ name: 'big rollout waves', text: 'SELECT * FROM memory WHERE kind = record;' }]}
        history={[{ text: 'SELECT id FROM memory LIMIT 5;', format: 'md', ts: 1, ms: 3, rows: 5, ok: true }, { text: 'SELEKT', format: 'md', ts: 2, ok: false }]}
        entities={fixtureEntityRows}
        onSelect={log}
        onQuery={log}
        defaultOpen={['brain', 'sec:decisions', 'conn', 'ct:anchor', 'ents', 'et:person', 'sources', 'templates', 'saved', 'history']}
      />
    </div>
  ),
};
export const Tabs: S = { render: () => <TabBar tabs={initialUiState().tabs} active="query" onActivate={log} onClose={log} onAdd={noop} /> };
export const Status: S = { render: () => <div style={{ position: 'fixed', bottom: 0, left: 0, right: 0, height: 24, display: 'flex' }}><StatusBar db="sso" connection="sso" live="live" brain={fixtureState} lastOp={{ name: 'run', ms: 9, ok: true }} storage="xml" format="bin" /></div> };
export const StatusOffline: S = { render: () => <div style={{ position: 'fixed', bottom: 0, left: 0, right: 0, height: 24, display: 'flex' }}><StatusBar connection="sso" live="offline" brain={null} lastOp={{ name: 'state', ms: 2001, ok: false, error: 'Failed to fetch' }} /></div> };
export const Palette: S = {
  render: () => (
    <CommandPalette
      onClose={noop}
      commands={[
        { id: '1', group: 'Query', label: 'Run query (BDQL)', hint: 'Ctrl+Shift+Enter', run: noop },
        { id: '2', group: 'Open tab', label: 'Schema', run: noop },
        { id: '3', group: 'View', label: 'Graph view: Depth sphere', run: noop },
        ...fixtureState.nodes.slice(0, 20).map((n) => ({ id: n.id, group: 'Go to', label: n.text, hint: n.id, run: noop })),
      ]}
    />
  ),
};
export const PaletteFiltered: S = { render: () => <CommandPalette onClose={noop} initial="entra" commands={fixtureState.nodes.map((n) => ({ id: n.id, group: 'Go to', label: n.text, hint: n.id, run: noop }))} /> };
export const Token: S = { name: 'Token prompt', render: () => <TokenPrompt onSubmit={(t) => log('token', t)} /> };
export const TokenRejected: S = { render: () => <TokenPrompt onSubmit={log} error="401: invalid token" /> };
export const About: S = { render: () => <AboutDialog version="0.9.0-dev" onClose={noop} /> };
