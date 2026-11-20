// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { Meta, StoryObj } from '@storybook/react-vite';
import { ContextViewer } from '../components/context/ContextViewer';
import { ContextTab } from '../components/context/ContextTab';
import { fixtureContextMd, fixtureContextXml } from '../fixtures';
import { MockStore, TabFrame, log } from './helpers';

const meta: Meta = { title: 'brainDB/Context' };
export default meta;
type S = StoryObj;

export const ViewerXml: S = { render: () => <TabFrame><ContextViewer format="xml" onFormat={log} text={fixtureContextXml} onRefresh={() => log('refresh')} /></TabFrame> };
export const ViewerMarkdown: S = { render: () => <TabFrame><ContextViewer format="md" onFormat={log} text={fixtureContextMd} /></TabFrame> };
export const ViewerLoading: S = { render: () => <TabFrame><ContextViewer format="xml" onFormat={log} text={null} loading /></TabFrame> };
export const ViewerError: S = { render: () => <TabFrame><ContextViewer format="xml" onFormat={log} text={null} error="502 Bad Gateway" onRefresh={() => log('retry')} /></TabFrame> };
export const ContextTabConnected: S = { render: () => <MockStore><TabFrame><ContextTab /></TabFrame></MockStore> };
