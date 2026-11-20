// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { Meta, StoryObj } from '@storybook/react-vite';
import { PurposeEditor } from '../components/purpose/PurposeEditor';
import { CheckResult, ThresholdBand } from '../components/purpose/CheckResult';
import { ReviewQueue } from '../components/purpose/ReviewQueue';
import { ImportReportView } from '../components/db/ImportReportView';
import { SettingsTab } from '../components/db/DbTabs';
import { Workspace } from '../app/Workspace';
import { createMockClient } from '../api/mockClient';
import { fixtureImportReport, fixturePurpose, fixturePurposeChecks, fixtureQuarantine, fixtureState } from '../fixtures';
import { Frame, MockStore, TabFrame, log } from './helpers';

const meta: Meta = { title: 'brainDB/Purpose' };
export default meta;
type S = StoryObj;
const mc = createMockClient({ log, delay: 40 });
const check = (q: { text: string; kind?: string; section?: string }) => mc.checkPurpose(q);
const ed = (text: string, key?: keyof typeof fixturePurposeChecks) => (
  <TabFrame>
    <PurposeEditor
      value={fixturePurpose}
      onSave={(p) => log('save', p)}
      check={check}
      kinds={fixtureState.kinds.map((k) => k.name)}
      sections={fixtureState.sections}
      initialCheckText={text}
      initialCheck={key ? fixturePurposeChecks[key] : undefined}
    />
  </TabFrame>
);

export const Editor: S = { render: () => ed('') };
export const EditorCheckAccept: S = { name: 'Editor · check → accept', render: () => ed('Guest accounts may bypass conditional access during the pilot wave', 'accept') };
export const EditorCheckReview: S = { name: 'Editor · check → review', render: () => ed('Team offsite moved to Thursday', 'review') };
export const EditorCheckReject: S = { name: 'Editor · check → reject', render: () => ed('Lunch menu for Friday', 'reject') };
export const EditorForbiddenEntity: S = { name: 'Editor · forbidden entity (card)', render: () => ed('Client paid with 4111 1111 1111 1111', 'forbidden') };
export const CheckAccept: S = { render: () => <Frame><CheckResult r={fixturePurposeChecks.accept} accept={0.35} review={0.2} /></Frame> };
export const CheckReview: S = { render: () => <Frame><CheckResult r={fixturePurposeChecks.review} accept={0.35} review={0.2} /></Frame> };
export const CheckReject: S = { render: () => <Frame><CheckResult r={fixturePurposeChecks.reject} accept={0.35} review={0.2} /></Frame> };
export const Band: S = { render: () => <Frame width={420}><ThresholdBand accept={0.35} review={0.2} score={0.27} /></Frame> };
export const Queue: S = { name: 'Review queue', render: () => <TabFrame><ReviewQueue items={fixtureQuarantine} sections={fixtureState.sections} onAdmit={(...a) => log('admit', ...a)} onReject={(...a) => log('reject', ...a)} onGo={log} /></TabFrame> };
export const QueueEmpty: S = { name: 'Review queue (empty)', render: () => <TabFrame><ReviewQueue items={[]} sections={[]} onAdmit={log} onReject={log} /></TabFrame> };
export const ImportWithDecisions: S = { name: 'Import report with decisions', render: () => <Frame width={1100}><ImportReportView report={fixtureImportReport} /></Frame> };
export const SettingsPurpose: S = { name: 'Settings → Purpose (connected)', render: () => <MockStore><TabFrame><SettingsTab initialSection="purpose" /></TabFrame></MockStore> };
export const SettingsNoPurpose: S = { name: 'Settings → Purpose (none declared)', render: () => <MockStore mock={{ noPurpose: true }}><TabFrame><SettingsTab initialSection="purpose" /></TabFrame></MockStore> };
export const WorkspaceReview: S = { name: 'Workspace · review queue + status count', render: () => <MockStore initial={{ activeTab: 'review' }}><Workspace /></MockStore> };
