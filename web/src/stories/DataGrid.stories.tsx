// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { Meta, StoryObj } from '@storybook/react-vite';
import { DataGrid } from '../components/common/DataGrid';
import { fixtureQuery, makeBigQuery } from '../fixtures';
import { TabFrame, log } from './helpers';

const meta: Meta<typeof DataGrid> = { title: 'brainDB/Data/DataGrid', component: DataGrid };
export default meta;
type S = StoryObj<typeof DataGrid>;

const big = makeBigQuery(20000);

export const QueryResult: S = { render: () => <TabFrame><DataGrid columns={fixtureQuery.columns} rows={fixtureQuery.rows} total={fixtureQuery.total} elapsedMs={3} onRowActivate={(r) => log('activate', r.id)} /></TabFrame> };
export const Empty: S = { render: () => <TabFrame><DataGrid columns={['id', 'text']} rows={[]} elapsedMs={1} /></TabFrame> };
export const Large20kRows: S = {
  name: 'Large (20k rows, virtualised)',
  render: () => <TabFrame><DataGrid columns={big.columns} rows={big.rows} total={big.total} elapsedMs={41} onRowActivate={(r) => log('activate', r.id)} exportName="deals" /></TabFrame>,
};
export const NullsAndMissing: S = {
  render: () => (
    <TabFrame height={300}>
      <DataGrid columns={['path', 'value', 'score']} rows={[{ path: 'project.name', value: 'SSO rollout', score: 0.62 }, { path: 'budget', value: null, score: 0.07 }]} cellClass={(r) => (r.value === null ? 'missing' : undefined)} />
    </TabFrame>
  ),
};
