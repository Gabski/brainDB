// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { Meta, StoryObj } from '@storybook/react-vite';
import { Workspace } from '../app/Workspace';
import { fixtureRunMulti } from '../fixtures';
import { initialUiState, type UiState } from '../state/store';
import { MockStore } from './helpers';
import type { MockOptions } from '../api/mockClient';
import type { BrainStore } from '../state/store';

const meta: Meta = { title: 'brainDB/Workspace (full page)', parameters: { layout: 'fullscreen' } };
export default meta;
type S = StoryObj;

const Page = ({ initial, mock, onStore }: { initial?: Partial<UiState>; mock?: MockOptions; onStore?: (s: BrainStore) => void }) => (
  <MockStore initial={initial} mock={mock} onStore={onStore}>
    <Workspace />
  </MockStore>
);
const ui = initialUiState();

export const GraphTab: S = { render: () => <Page initial={{ activeTab: 'graph' }} onStore={(s) => setTimeout(() => s.select('d121'), 0)} /> };
export const QueryTab: S = { render: () => <Page initial={{ activeTab: 'query', bdql: { ...ui.bdql, script: fixtureRunMulti.results.map((r) => r.statement).join('\n\n'), result: fixtureRunMulti, ms: 9 } }} onStore={(s) => setTimeout(() => s.select('r193'), 0)} /> };
export const SchemaTab: S = { render: () => <Page initial={{ activeTab: 'schema' }} /> };
export const SourcesTab: S = { render: () => <Page initial={{ activeTab: 'sources' }} onStore={(s) => setTimeout(() => s.select('lab:x204'), 0)} /> };
export const ContextTab: S = { render: () => <Page initial={{ activeTab: 'context' }} /> };
export const TemplateTab: S = { render: () => <Page initial={{ activeTab: 'template' }} /> };
export const ActivityLive: S = { render: () => <Page initial={{ activeTab: 'activity' }} mock={{ eventEvery: 4000 }} /> };
export const SettingsTab: S = { render: () => <Page initial={{ activeTab: 'settings', tabs: [...ui.tabs, 'import', 'settings'] }} /> };
export const NewDatabaseWizard: S = { render: () => <Page initial={{ wizardOpen: true }} /> };
export const TokenRequired: S = { render: () => <Page initial={{ needToken: true, brain: null }} mock={{ hang: true }} /> };
export const BackendDown: S = { render: () => <Page initial={{ brain: null }} mock={{ fail: true }} /> };
