// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { Meta, StoryObj } from '@storybook/react-vite';
import { SearchBar } from '../components/search/SearchBar';
import { SearchResults } from '../components/search/SearchResults';
import { IngestReport } from '../components/search/IngestReport';
import { SuggestionList } from '../components/search/SuggestionList';
import { ConnectorSuggestions } from '../components/search/ConnectorSuggestions';
import { SearchTab } from '../components/search/SearchTab';
import { NarrowBar, RecentBar } from '../components/search/NarrowBar';
import { EMPTY_NARROW } from '../model/searchView';
import { fixtureConnectorSuggestions, fixtureDiscover, fixtureIngest, fixtureSearch, fixtureSources } from '../fixtures';
import { Frame, MockStore, TabFrame, g, log } from './helpers';
import type { BrainStore } from '../state/store';

const meta: Meta = { title: 'brainDB/Search' };
export default meta;
type S = StoryObj;
const W = 520;

export const Bar: S = { render: () => <Frame width={W}><SearchBar query="entra limits" mode="hybrid" sources={fixtureSources} onSearch={(...a) => log('search', ...a)} onMode={log} onSources={log} /></Frame> };
export const Hybrid: S = { render: () => <Frame width={W}><SearchResults g={g} query="entra login limits" result={fixtureSearch.hybrid} onGo={log} onFocus={log} elapsedMs={4} /></Frame> };
export const Vector: S = { render: () => <Frame width={W}><SearchResults g={g} query="entra login limits" result={fixtureSearch.vector} onGo={log} onFocus={log} /></Frame> };
export const Tree: S = { render: () => <Frame width={W}><SearchResults g={g} query="entra login limits" result={fixtureSearch.tree} onGo={log} onFocus={log} /></Frame> };
export const Graph: S = { render: () => <Frame width={W}><SearchResults g={g} query="rate limit login" result={fixtureSearch.graph} onGo={log} onFocus={log} /></Frame> };
export const Federated: S = { render: () => <Frame width={W}><SearchResults g={g} query="token endpoint" result={{ ...fixtureSearch.hybrid, rows: [{ id: 'lab:x204', score: 0.61, vector: 0.7, graph: 0.5, source: 'lab' }, ...fixtureSearch.hybrid.rows] }} onGo={log} onFocus={log} /></Frame> };
export const Grouped: S = {
  name: 'Hybrid grouped by kind',
  render: () => <Frame width={W}><SearchResults g={g} query="entra login limits" result={fixtureSearch.hybrid} onGo={log} onFocus={log} elapsedMs={6} group="kind" /></Frame>,
};
export const Narrowed: S = {
  name: 'Hybrid narrowed (text)',
  render: () => <Frame width={W}><SearchResults g={g} query="entra login limits" result={fixtureSearch.hybrid} onGo={log} onFocus={log} elapsedMs={6} narrow={{ ...EMPTY_NARROW, text: 'login' }} /></Frame>,
};
export const Narrow: S = {
  name: 'Narrow bar',
  render: () => (
    <Frame width={W}>
      <NarrowBar g={g} result={fixtureSearch.hybrid} narrow={{ ...EMPTY_NARROW, kinds: ['fact'] }} group="none" onNarrow={log} onToggle={(a, b) => log('toggle', a, b)} onGroup={log} onClear={() => log('clear')} />
    </Frame>
  ),
};
export const Recent: S = {
  name: 'Recent searches',
  render: () => (
    <Frame width={W}>
      <RecentBar
        items={[
          { query: 'entra limits', mode: 'hybrid', rows: 8, ms: 5 },
          { query: 'who approves app changes', mode: 'tree', rows: 12, ms: 9 },
          { query: 'token endpoint', mode: 'vector', rows: 6, ms: 3 },
        ]}
        onPick={(q, m) => log('pick', q, m)}
        onClear={() => log('clear')}
      />
    </Frame>
  ),
};
export const Ingest: S = { render: () => <Frame width={W}><IngestReport g={g} result={fixtureIngest} onGo={log} /></Frame> };
export const DiscoverInner: S = { render: () => <Frame width={W}><SuggestionList g={g} title="Missing synapses inside the context" list={fixtureDiscover.discover!.inner} onGo={log} onPreview={log} onAdd={(...a) => log('add', ...a)} /></Frame> };
export const DiscoverBridges: S = { render: () => <Frame width={W}><SuggestionList g={g} title="Bridges: context ↔ faded memory" list={fixtureDiscover.discover!.bridge} onGo={log} onAdd={(...a) => log('add', ...a)} /></Frame> };
export const NoSuggestions: S = { render: () => <Frame width={W}><SuggestionList g={g} title="Proposed new links" list={[]} onGo={log} /></Frame> };
export const SuggestedConnectors: S = { render: () => <Frame width={W}><ConnectorSuggestions g={g} list={fixtureConnectorSuggestions.connectors!} onGo={log} onCreate={(q) => log('bdql', q)} /></Frame> };
export const SearchTabEmpty: S = { render: () => <MockStore><TabFrame><SearchTab /></TabFrame></MockStore> };
export const SearchTabWithResults: S = {
  render: () => (
    <MockStore
      onStore={(s: BrainStore) =>
        setTimeout(() => {
          s.runSearch('entra login limits', 'hybrid');
          s.discover();
        }, 0)
      }
    >
      <TabFrame>
        <SearchTab />
      </TabFrame>
    </MockStore>
  ),
};
export const SearchTabError: S = { render: () => <MockStore mock={{ fail: 'search' }} onStore={(s: BrainStore) => setTimeout(() => s.runSearch('entra'), 0)}><TabFrame><SearchTab /></TabFrame></MockStore> };
