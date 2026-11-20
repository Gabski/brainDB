// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { Meta, StoryObj } from '@storybook/react-vite';
import { KindBadge } from '../components/common/KindBadge';
import { NodeRow } from '../components/common/NodeRow';
import { Empty, ErrorBox, Loading, SectionH } from '../components/common/Async';
import { Slider } from '../components/common/Slider';
import { CodeView } from '../components/common/CodeView';
import { ConfirmDialog, Modal } from '../components/common/Modal';
import { Toast } from '../components/common/Toast';
import { fixtureBrainXml, fixtureConfigYaml, fixtureContextMd, fixtureContextXml, fixtureFill } from '../fixtures';
import { Frame, g, noop } from './helpers';
import { useState } from 'react';

const meta: Meta = { title: 'brainDB/Common' };
export default meta;
type S = StoryObj;

export const KindBadges: S = {
  render: () => (
    <Frame>
      <div className="row">
        {[...g.kinds.values()].map((k) => (
          <KindBadge key={k.name} label={k.label} color={k.color} />
        ))}
      </div>
    </Frame>
  ),
};
export const NodeRows: S = {
  render: () => (
    <Frame>
      <SectionH count={5}>lit, faded, concept, superseded, external</SectionH>
      <div className="list">
        {['d121', 'f201', 'A:idp', 'd120', 'lab:x204'].map((id) => (
          <NodeRow key={id} g={g} id={id} showId right={g.nodes.get(id)?.activation?.toFixed(2) ?? 'faded'} onGo={noop} />
        ))}
        <NodeRow g={g} id="zz999" right="missing" />
      </div>
    </Frame>
  ),
};
export const AsyncStates: S = {
  render: () => (
    <Frame>
      <Loading />
      <ErrorBox error="op#2 link: no such item x999" onRetry={noop} />
      <Empty>Nothing here yet.</Empty>
    </Frame>
  ),
};
export const Sliders: S = {
  render: () => {
    const [v, setV] = useState(0.16);
    return (
      <Frame>
        <Slider label="Lit threshold (act ≥)" value={v} min={0.02} max={0.6} step={0.01} onChange={setV} />
        <Slider label="Context depth" value={2} min={0} max={6} step={1} digits={0} onChange={noop} />
      </Frame>
    );
  },
};
export const CodeXml: S = { render: () => <Frame width={700}><CodeView text={fixtureContextXml} lang="xml" /></Frame> };
export const CodeMarkdown: S = { render: () => <Frame width={700}><CodeView text={fixtureContextMd} lang="md" /></Frame> };
export const CodeJson: S = { render: () => <Frame width={700}><CodeView text={fixtureFill.output} lang="json" /></Frame> };
export const CodeYaml: S = { render: () => <Frame width={700}><CodeView text={fixtureConfigYaml} lang="yaml" /></Frame> };
export const CodeBrainXml: S = { render: () => <Frame width={900}><CodeView text={fixtureBrainXml.slice(0, 3000)} lang="xml" /></Frame> };
export const ModalDialog: S = { render: () => <Modal title="A dialog" onClose={noop}>Body text. Esc closes.</Modal> };
export const Confirm: S = { render: () => <ConfirmDialog title="Forget d120?" text="This removes the memory." danger confirmLabel="Forget" onConfirm={noop} onCancel={noop} /> };
export const ToastOk: S = { render: () => <Toast text="Linked d121 ↔ f103 (w 0.60)" ok onClose={noop} /> };
export const ToastError: S = { render: () => <Toast text="exec: op#2 link: no such item x999" ok={false} onClose={noop} /> };
