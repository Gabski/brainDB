// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { Meta, StoryObj } from '@storybook/react-vite';
import { QueryTab, SnippetLibrary } from '../components/query/QueryTab';
import { ChangeList, PlanTree, StatementResults } from '../components/query/StatementResults';
import { fixtureExplainError, fixtureQuery, fixtureRunError, fixtureRunExplain, fixtureRunMulti } from '../fixtures';
import { initialUiState, type UiState } from '../state/store';
import { Frame, MockStore, TabFrame, log } from './helpers';

const meta: Meta = { title: 'brainDB/Query' };
export default meta;
type S = StoryObj;
const baseBdql = initialUiState().bdql;
const withBdql = (p: Partial<UiState['bdql']>): Partial<UiState> => ({ bdql: { ...baseBdql, ...p } });

const Tab = ({ initial, ac, mock }: { initial?: Partial<UiState>; ac?: string; mock?: object }) => (
  <MockStore initial={initial} mock={mock}>
    <TabFrame>
      <QueryTab initialCompletion={ac ? { prefix: ac } : undefined} />
    </TabFrame>
  </MockStore>
);

export const EditorDefault: S = { render: () => <Tab /> };
export const AutocompleteOpen: S = { render: () => <Tab initial={withBdql({ script: 'SELECT id, text, fields.\n  FROM memory WHERE kind = record;' })} ac="fields." /> };
export const LintError: S = {
  render: () => <Tab initial={withBdql({ script: 'SELECT id FROM memory;\n\nSELEKT id FROM memory;\n\nSELECT id FROM memory WHERE kind = tsk;', lint: fixtureExplainError.errors })} />,
};
export const MultiResultRun: S = {
  render: () => <Tab initial={withBdql({ script: fixtureRunMulti.results.map((r) => r.statement).join('\n\n'), result: fixtureRunMulti, ms: 9 })} />,
};
export const MultiResultTextTab: S = {
  render: () => <Tab initial={withBdql({ script: fixtureRunMulti.results.map((r) => r.statement).join('\n\n'), result: fixtureRunMulti, active: 1 })} />,
};
export const MultiResultChangesTab: S = {
  render: () => <Tab initial={withBdql({ script: fixtureRunMulti.results.map((r) => r.statement).join('\n\n'), result: fixtureRunMulti, active: 2 })} />,
};
export const Explain: S = { render: () => <Tab initial={withBdql({ script: fixtureRunExplain.results[0].statement, result: fixtureRunExplain })} /> };
export const RunError: S = { render: () => <Tab initial={withBdql({ script: 'BOOST d121;\nSELEKT * FROM memory;', result: fixtureRunError, lint: [fixtureRunError.error!] })} /> };
export const Running: S = { render: () => <Tab initial={withBdql({ loading: true })} /> };
export const CompactMode: S = {
  render: () => <Tab initial={{ bdql: { ...baseBdql, mode: 'compact' }, query: { ...initialUiState().query, result: fixtureQuery, ms: 3 } }} />,
};
export const CompactModeError: S = {
  render: () => <Tab initial={{ bdql: { ...baseBdql, mode: 'compact' }, query: { ...initialUiState().query, text: '{"from": "memory", ', error: 'Invalid JSON: Unexpected end of JSON input' } }} />,
};
export const Results: S = { render: () => <TabFrame><StatementResults result={fixtureRunMulti} active={0} onActive={log} onGo={log} /></TabFrame> };
export const Plan: S = { render: () => <Frame width={700}><PlanTree plan={fixtureRunExplain.results[0].plan!} /></Frame> };
export const Changes: S = { render: () => <TabFrame height={300}><ChangeList r={fixtureRunMulti.results[2]} onGo={log} /></TabFrame> };
export const Snippets: S = { render: () => <Frame width={760}><SnippetLibrary onPick={(c) => log('pick', c)} /></Frame> };
