// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useState } from 'react';
import type { Meta, StoryObj } from '@storybook/react-vite';
import { Browse } from '../components/browse/BrowseTab';
import { DEFAULT_BROWSE, type BrowseQuery } from '../model/browse';
import { fixtureEntityRows } from '../fixtures';
import { g, log, TabFrame } from './helpers';

const meta: Meta = { title: 'brainDB/Browse' };
export default meta;
type S = StoryObj;

/** Local-state wrapper so the filters are live in Storybook. */
function Live({ init, entities = fixtureEntityRows }: { init?: Partial<BrowseQuery>; entities?: typeof fixtureEntityRows | null | undefined }) {
  const [q, setQ] = useState<BrowseQuery>({ ...DEFAULT_BROWSE, ...init });
  const [sel, setSel] = useState<string | null>(null);
  const [ents, setEnts] = useState(entities);
  return (
    <TabFrame>
      <Browse
        g={g}
        query={q}
        onQuery={(p) => setQ((x) => ({ ...x, ...p }))}
        entities={ents}
        onLoadEntities={() => (log('loadEntities'), setEnts(fixtureEntityRows))}
        selected={sel}
        onSelect={(id) => (log('select', id), setSel(id))}
        onFocus={(ids) => log('focus', ids)}
        onToast={(t, ok) => log('toast', t, ok)}
      />
    </TabFrame>
  );
}

export const Default: S = { render: () => <Live /> };
export const Filtered: S = { name: 'Free text + facet', render: () => <Live init={{ q: 'kind:fact access', sort: 'weight' }} /> };
export const PinnedOnly: S = { render: () => <Live init={{ pinned: true }} /> };
export const SortedByActivation: S = { render: () => <Live init={{ sort: 'activation', desc: true }} /> };
export const WithSuperseded: S = { render: () => <Live init={{ superseded: true, sort: 'id', desc: false }} /> };
export const EntitiesNotLoaded: S = { name: 'Entities facet (lazy)', render: () => <Live entities={undefined} /> };
export const NoMatch: S = { render: () => <Live init={{ q: 'zzzz-nothing-matches' }} /> };
