// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { Meta, StoryObj } from '@storybook/react-vite';
import { EvidenceTable } from '../components/template/EvidenceTable';
import { TemplateTab } from '../components/template/TemplateTab';
import { fixtureFill, fixtureFillMissing } from '../fixtures';
import { Frame, MockStore, TabFrame, log } from './helpers';

const meta: Meta = { title: 'brainDB/Template' };
export default meta;
type S = StoryObj;

export const Evidence: S = { render: () => <Frame width={900}><EvidenceTable values={fixtureFill.values} missing={[]} onSource={log} /></Frame> };
export const EvidenceMissing: S = { render: () => <Frame width={900}><EvidenceTable values={fixtureFillMissing.values} missing={fixtureFillMissing.missing} onSource={log} /></Frame> };
export const TabEmpty: S = { render: () => <MockStore><TabFrame><TemplateTab /></TabFrame></MockStore> };
export const TabFilled: S = { render: () => <MockStore><TabFrame><TemplateTab initialResult={fixtureFill} /></TabFrame></MockStore> };
export const TabMissingSlots: S = { render: () => <MockStore mock={{ fillMissing: true }}><TabFrame><TemplateTab initialResult={fixtureFillMissing} /></TabFrame></MockStore> };
export const TabError: S = { render: () => <MockStore mock={{ fail: 'fill' }}><TabFrame><TemplateTab initialError="fill: template parse error at line 3: unterminated string" /></TabFrame></MockStore> };
