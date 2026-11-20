// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { Meta, StoryObj } from '@storybook/react-vite';
import { SourceForm, SourceList, SourcesTab } from '../components/sources/SourcesTab';
import { fixtureSources } from '../fixtures';
import { Frame, MockStore, TabFrame, log } from './helpers';

const meta: Meta = { title: 'brainDB/Sources' };
export default meta;
type S = StoryObj;

export const List: S = { render: () => <Frame width={1100}><SourceList sources={fixtureSources} onSync={log} onRemove={log} onToggle={(s) => log('toggle', s.name)} /></Frame> };
export const ListEmpty: S = { render: () => <Frame width={700}><SourceList sources={[]} onSync={log} onRemove={log} /></Frame> };
export const Form: S = { render: () => <Frame width={900}><SourceForm onSubmit={(s) => log('add', s)} /></Frame> };
export const FormInvalidName: S = { render: () => <Frame width={900}><SourceForm onSubmit={log} initial={{ name: 'A', uri: 'http://lab:7791' }} /></Frame> };
export const TabConnected: S = { render: () => <MockStore><TabFrame><SourcesTab /></TabFrame></MockStore> };
export const TabError: S = { render: () => <MockStore mock={{ fail: 'sources' }} initial={{ sources: { loading: false } }}><TabFrame><SourcesTab /></TabFrame></MockStore> };
