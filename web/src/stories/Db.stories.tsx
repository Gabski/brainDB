// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { Meta, StoryObj } from '@storybook/react-vite';
import { ImportWizard, type ImportWizardProps } from '../components/db/ImportWizard';
import { ImportReportView } from '../components/db/ImportReportView';
import { ConfigEditor } from '../components/db/ConfigEditor';
import { DbSettings } from '../components/db/settings/DbSettings';
import { FilePicker } from '../components/db/FilePicker';
import { ImportTab, SettingsTab } from '../components/db/DbTabs';
import { createMockClient } from '../api/mockClient';
import { fixtureConfigErrors, fixtureConfigErrorsYaml, fixtureConfigResult, fixtureConfigYaml, fixtureImportPartial, fixtureImportReport, fixtureState } from '../fixtures';
import type { EmbedProvider, ImportFile } from '../api/types';
import { Frame, MockStore, TabFrame, log } from './helpers';
import { useEffect, useState } from 'react';

const meta: Meta = { title: 'brainDB/Database' };
export default meta;
type S = StoryObj;

const mc = createMockClient({ log, delay: 300 });
const files: ImportFile[] = ['raw/notes/kickoff.md', 'raw/notes/renewal-risks.md', 'raw/crm.sql', 'raw/tickets.jsonl', 'raw/products.json', 'raw/pricing.csv'].map((p) => ({ path: p, file: new File(['x'.repeat(1200)], p.split('/').pop()!) }));
const base: ImportWizardProps = {
  mode: 'create',
  dryRun: (f, c, n) => mc.importFiles(f, { db: n, config: c, dryRun: true }),
  validate: (y, n) => mc.putConfig(y, { db: n, dryRun: true }),
  run: (f, c, n) => mc.importFiles(f, { db: n, config: c }),
  onDone: (n) => log('done', n),
  onCancel: () => log('cancel'),
  examples: [
    { name: 'sales-company', description: 'Tessaline sales memory' },
    { name: 'research-lab', description: 'Electrolyte lab memory' },
    { name: 'import-demo', description: 'A raw pile turned into a database' },
  ],
  onInstallExample: async (example, db) => {
    log('install', example, db);
  },
};
const W = ({ children }: { children: React.ReactNode }) => <Frame width={980}>{children}</Frame>;

export const Step1Name: S = { name: 'Wizard 1 · name', render: () => <W><ImportWizard {...base} /></W> };
export const Step2Files: S = { name: 'Wizard 2 · files and folders', render: () => <W><ImportWizard {...base} initialStep="files" initialName="crm" initialFiles={files} /></W> };
export const Step3Preview: S = { name: 'Wizard 3 · dry-run preview', render: () => <W><ImportWizard {...base} initialStep="preview" initialName="crm" initialFiles={files} initialPreview={fixtureImportReport} /></W> };
export const Step4Config: S = { name: 'Wizard 4 · config (valid)', render: () => <W><ImportWizard {...base} initialStep="config" initialName="crm" initialFiles={files} initialPreview={fixtureImportReport} initialConfig={fixtureConfigYaml} initialConfigResult={fixtureConfigResult} /></W> };
export const Step4ConfigErrors: S = { name: 'Wizard 4 · config with errors', render: () => <W><ImportWizard {...base} initialStep="config" initialName="crm" initialFiles={files} initialPreview={fixtureImportReport} initialConfig={fixtureConfigErrorsYaml} initialConfigResult={fixtureConfigErrors} /></W> };
export const Step5Progress: S = {
  name: 'Wizard 5 · create with progress',
  render: () => <W><ImportWizard {...base} initialStep="run" initialName="crm" progress={{ type: 'import.progress', ts: 0, by: 'ui', tool: 'import', summary: '', ids: [], db: 'crm', done: 3, total: 8, file: 'raw/crm.sql' }} run={() => new Promise(() => {})} /></W>,
};
export const Step5Done: S = { name: 'Wizard 5 · done', render: () => <W><ImportWizard {...base} initialStep="run" initialName="crm" initialResult={{ ...fixtureImportReport, dryRun: false }} /></W> };
export const ImportIntoExisting: S = { name: 'Import flow (existing db)', render: () => <W><ImportWizard {...base} mode="import" dbName="sso" /></W> };
export const ReportSuccess: S = { render: () => <W><ImportReportView report={fixtureImportReport} onUseMapping={log} /></W> };
export const ReportPartialErrors: S = { render: () => <W><ImportReportView report={fixtureImportPartial} /></W> };
export const Files: S = {
  render: () => {
    const [f, setF] = useState(files);
    return <W><FilePicker files={f} onFiles={setF} examples={['import-demo']} onPickExample={log} /></W>;
  },
};
export const ConfigValid: S = {
  render: () => {
    const [y, setY] = useState(fixtureConfigYaml);
    return <W><ConfigEditor value={y} onChange={setY} validate={(v) => mc.putConfig(v, { dryRun: true })} initialResult={fixtureConfigResult} /></W>;
  },
};
export const ConfigWithErrors: S = {
  render: () => {
    const [y, setY] = useState(fixtureConfigErrorsYaml);
    return <W><ConfigEditor value={y} onChange={setY} validate={(v) => mc.putConfig(v, { dryRun: true })} initialResult={fixtureConfigErrors} /></W>;
  },
};
export const SettingsTabConnected: S = { render: () => <MockStore><TabFrame><SettingsTab /></TabFrame></MockStore> };
export const ImportTabConnected: S = { render: () => <MockStore><TabFrame><ImportTab /></TabFrame></MockStore> };

/** The settings forms edit the YAML text itself, so a story only needs the file and a validator. */
function SettingsStory({ initial, result }: { initial: string; result: typeof fixtureConfigResult }) {
  const [draft, setDraft] = useState(initial);
  const [saved, setSaved] = useState(initial);
  const [providers, setProviders] = useState<EmbedProvider[]>([]);
  useEffect(() => {
    mc.embeddingProviders().then(setProviders);
  }, []);
  return (
    <TabFrame>
      <DbSettings
        db="sso"
        saved={saved}
        draft={draft}
        onDraft={setDraft}
        validate={(y) => mc.putConfig(y, { dryRun: true })}
        initialResult={result}
        onSave={() => {
          setSaved(draft);
          log('save', draft);
        }}
        onReload={() => setDraft(saved)}
        kinds={fixtureState.kinds}
        providers={providers}
        storage={{ backend: 'file', format: 'xml', nodes: 158, edges: 498, db: 'sso' }}
        onMigrate={(to) => log('migrate', to)}
      />
    </TabFrame>
  );
}

export const SettingsForms: S = { name: 'Settings · forms', render: () => <SettingsStory initial={fixtureConfigYaml} result={fixtureConfigResult} /> };
export const SettingsFormsInvalid: S = { name: 'Settings · forms with a broken file', render: () => <SettingsStory initial={fixtureConfigErrorsYaml} result={fixtureConfigErrors} /> };
export const SettingsFormsEmpty: S = {
  name: 'Settings · an almost empty config',
  render: () => <SettingsStory initial={'name: scratch\n'} result={{ ...fixtureConfigResult, errors: [] }} />,
};

/** A file that configures API and MCP tokens: the Access & MCP section shows them as a table of variable names. */
const TOKENS_YAML = `name: sso
description: SSO rollout knowledge base
server:
  addr: 0.0.0.0:7791
  tokenEnv: BRAINDB_TOKEN
  tokens:
    - {name: bot, tokenEnv: BRAINDB_TOKEN_BOT, scope: read}
    - {name: ci, tokenEnv: BRAINDB_TOKEN_CI, scope: write, dbs: [sso]}
mcp:
  actor: agent
  tools: [view, run, query]
`;

export const SettingsTokens: S = {
  name: 'Settings · server and MCP tokens',
  render: () => <SettingsStory initial={TOKENS_YAML} result={{ ...fixtureConfigResult, errors: [] }} />,
};
