// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { Meta, StoryObj } from '@storybook/react-vite';
import { StoragePanel } from '../components/schema/StoragePanel';
import { PatternsPanel } from '../components/schema/PatternsPanel';
import { SchemaTab } from '../components/schema/SchemaTab';
import { BrainXmlTab } from '../components/schema/BrainXmlTab';
import { createMockClient } from '../api/mockClient';
import { fixturePatterns, fixtureServerStats, fixtureServerStatsBin, fixtureServerStatsFile, fixtureServerStatsRedis } from '../fixtures';
import { Frame, MockStore, TabFrame, log } from './helpers';

const meta: Meta = { title: 'brainDB/Schema' };
export default meta;
type S = StoryObj;
const sp = { onSnapshot: () => log('snapshot'), onMigrate: (t: string) => log('migrate', t), onConvert: (t: string) => log('convert', t), onExportXml: () => log('export'), onRefresh: () => log('refresh') };

export const StorageXml: S = { name: 'Storage (xml format)', render: () => <Frame width={480}><StoragePanel stats={fixtureServerStats} {...sp} /></Frame> };
export const StorageBin: S = { name: 'Storage (bin format)', render: () => <Frame width={480}><StoragePanel stats={fixtureServerStatsBin} {...sp} /></Frame> };
export const StorageBothRedisError: S = { name: 'Storage (both format, redis error)', render: () => <Frame width={480}><StoragePanel stats={fixtureServerStatsRedis} {...sp} /></Frame> };
export const StorageFileBackend: S = {
  name: 'Storage (plain file backend, no redis stats)',
  render: () => <Frame width={480}><StoragePanel stats={fixtureServerStatsFile} {...sp} /></Frame>,
};
export const StorageConvertUnsupported: S = { render: () => <Frame width={480}><StoragePanel stats={fixtureServerStats} {...sp} convertNote="This server cannot convert over HTTP — requires CLI: braindb convert --to bin" /></Frame> };
export const StorageLoading: S = { render: () => <Frame width={480}><StoragePanel loading {...sp} /></Frame> };
export const StorageError: S = { render: () => <Frame width={480}><StoragePanel error="GET /api/stats: 404 Not Found" {...sp} /></Frame> };
const mc = createMockClient({ log, delay: 30 });
export const PatternTester: S = {
  render: () => (
    <Frame width={640}>
      <PatternsPanel patterns={fixturePatterns} test={(t) => mc.testPatterns(t)} onToggle={(...a) => log('toggle', ...a)} onAdd={(p) => log('add', p)} onRemove={(n) => log('remove', n)} />
    </Frame>
  ),
};
export const SchemaTabFull: S = { render: () => <MockStore><TabFrame><SchemaTab /></TabFrame></MockStore> };
export const SchemaTabFileBackend: S = {
  name: 'Schema tab (plain file backend)',
  render: () => <MockStore mock={{ stats: fixtureServerStatsFile }}><TabFrame><SchemaTab /></TabFrame></MockStore>,
};
export const SchemaTabConvertUnsupported: S = { render: () => <MockStore mock={{ convertUnsupported: true, stats: fixtureServerStatsBin }}><TabFrame><SchemaTab /></TabFrame></MockStore> };
export const BrainXml: S = { render: () => <MockStore><TabFrame><BrainXmlTab /></TabFrame></MockStore> };
