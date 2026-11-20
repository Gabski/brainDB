// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { Meta, StoryObj } from '@storybook/react-vite';
import { EntitiesTable, EntityText } from '../components/entities/EntityText';
import { fixtureEntitySamples, fixturePatternTest } from '../fixtures';
import { Frame, log } from './helpers';

const meta: Meta = { title: 'brainDB/Entities' };
export default meta;
type S = StoryObj;
const many = fixtureEntitySamples[0];

export const HighlightedText: S = {
  name: 'Highlighted text (many entity types)',
  render: () => (
    <Frame width={640}>
      {fixtureEntitySamples.map((s) => (
        <p key={s.id} style={{ lineHeight: 2 }}>
          <span className="mono xs muted">{s.id} </span>
          <EntityText text={s.text} entities={s.entities} />
        </p>
      ))}
    </Frame>
  ),
};
export const MaskedPii: S = {
  render: () => (
    <Frame width={640}>
      <p style={{ lineHeight: 2 }}>
        <EntityText text={many.text} entities={many.entities.filter((e) => e.pii)} />
      </p>
      <EntitiesTable entities={many.entities.filter((e) => e.pii)} />
    </Frame>
  ),
};
export const Table: S = { render: () => <Frame width={420}><EntitiesTable entities={many.entities} onValue={(t, v) => log('value', t, v)} /></Frame> };
export const TesterResult: S = { render: () => <Frame width={640}><EntityText text={fixturePatternTest.text} entities={fixturePatternTest.entities} /></Frame> };
export const None: S = { render: () => <Frame width={420}><EntitiesTable entities={[]} /></Frame> };
