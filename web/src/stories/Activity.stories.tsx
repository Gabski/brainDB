// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { Meta, StoryObj } from '@storybook/react-vite';
import { AgentFeed } from '../components/activity/AgentFeed';
import { fixtureJournal } from '../fixtures';
import { TabFrame, g, log } from './helpers';

const meta: Meta = { title: 'brainDB/Activity' };
export default meta;
type S = StoryObj;
const feed = [...fixtureJournal].reverse();

export const Live: S = { render: () => <TabFrame><AgentFeed feed={feed} live="live" g={g} onGo={log} /></TabFrame> };
export const WithImportProgress: S = {
  render: () => <TabFrame><AgentFeed feed={[{ type: 'import.progress', ts: Date.now(), by: 'ui', tool: 'import', summary: 'raw/crm.sql', ids: [], db: 'crm', done: 4, total: 8, file: 'raw/crm.sql' }, ...feed]} live="live" g={g} onGo={log} /></TabFrame>,
};
export const Offline: S = { render: () => <TabFrame><AgentFeed feed={feed} live="offline" g={g} onGo={log} /></TabFrame> };
export const Empty: S = { render: () => <TabFrame><AgentFeed feed={[]} live="connecting" /></TabFrame> };
