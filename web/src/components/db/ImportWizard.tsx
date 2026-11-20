// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useState } from 'react';
import type { BrainEvent, ConfigResult, ImportFile, ImportReport } from '../../api/types';
import { FilePicker } from './FilePicker';
import { ImportReportView } from './ImportReportView';
import { ConfigEditor } from './ConfigEditor';
import { ErrorBox, Loading } from '../common/Async';

export type WizardStep = 'name' | 'files' | 'preview' | 'config' | 'run';

export interface ImportWizardProps {
  /** create: new database (5 steps); import: into the active database (no name step). */
  mode: 'create' | 'import';
  dbName?: string;
  dryRun: (files: ImportFile[], config: string, name: string) => Promise<ImportReport>;
  validate: (yaml: string, name: string) => Promise<ConfigResult>;
  run: (files: ImportFile[], config: string, name: string, description: string) => Promise<ImportReport>;
  progress?: BrainEvent | null;
  onDone?: (name: string) => void;
  onCancel?: () => void;
  examples?: string[];
  // story/initial state
  initialStep?: WizardStep;
  initialName?: string;
  initialFiles?: ImportFile[];
  initialPreview?: ImportReport;
  initialConfig?: string;
  initialConfigResult?: ConfigResult;
  initialResult?: ImportReport;
}

const baseConfig = (name: string, description: string) => `name: ${name || 'newdb'}\ndescription: ${description || '""'}\ncontext: {depth: 2, threshold: 0.16, hop: 0.85}\nembedding: {model: hash-256, alpha: 0.5, hops: 2, use: enriched, lambda: 0.5}\n`;

/** "New database" / "Import" flow: name → files/folders → dry-run preview → config → create with progress. */
export function ImportWizard(p: ImportWizardProps) {
  const steps: WizardStep[] = p.mode === 'create' ? ['name', 'files', 'preview', 'config', 'run'] : ['files', 'preview', 'config', 'run'];
  const [step, setStep] = useState<WizardStep>(p.initialStep ?? steps[0]);
  const [name, setName] = useState(p.initialName ?? p.dbName ?? '');
  const [desc, setDesc] = useState('');
  const [files, setFiles] = useState<ImportFile[]>(p.initialFiles ?? []);
  const [preview, setPreview] = useState<ImportReport | null>(p.initialPreview ?? null);
  const [config, setConfig] = useState(p.initialConfig ?? '');
  const [result, setResult] = useState<ImportReport | null>(p.initialResult ?? null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const idx = steps.indexOf(step);
  const nameOk = /^[a-z0-9][a-z0-9_-]{0,40}$/.test(name);
  const LABEL: Record<WizardStep, string> = { name: '1 · Name', files: 'Files', preview: 'Preview', config: 'Config', run: p.mode === 'create' ? 'Create' : 'Import' };

  const doPreview = async () => {
    setBusy(true);
    setErr(null);
    try {
      const r = await p.dryRun(files, config, name);
      setPreview(r);
      if (!config) setConfig(baseConfig(name, desc) + r.mapping);
      setStep('preview');
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };
  const doRun = async () => {
    setBusy(true);
    setErr(null);
    setStep('run');
    try {
      setResult(await p.run(files, config, name, desc));
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };
  const pr = p.progress && (!p.progress.db || p.progress.db === name) ? p.progress : null;
  const pct = pr?.total ? Math.round(((pr.done ?? 0) / pr.total) * 100) : result ? 100 : 0;

  return (
    <div className="col">
      <div className="steps" role="list">
        {steps.map((s, i) => (
          <div key={s} role="listitem" className={'st' + (s === step ? ' on' : i < idx ? ' done' : '')} aria-current={s === step ? 'step' : undefined}>
            {p.mode === 'create' ? `${i + 1} · ` : `${i + 1} · `}
            {LABEL[s].replace(/^\d · /, '')}
          </div>
        ))}
      </div>
      {err && <ErrorBox error={err} />}

      {step === 'name' && (
        <form
          className="col"
          onSubmit={(e) => {
            e.preventDefault();
            if (nameOk) setStep('files');
          }}
        >
          <label className="field">
            <span className="lbl-row">database name</span>
            <input className="input mono" autoFocus placeholder="crm" value={name} onChange={(e) => setName(e.target.value.toLowerCase())} />
          </label>
          {name && !nameOk && <div className="xs bad">Use lower-case letters, digits, - and _ (max 41 chars).</div>}
          <label className="field">
            <span className="lbl-row">description</span>
            <input className="input" placeholder="Customer knowledge base" value={desc} onChange={(e) => setDesc(e.target.value)} />
          </label>
          <div className="row" style={{ justifyContent: 'flex-end' }}>
            {p.onCancel && (
              <button type="button" className="btn" onClick={p.onCancel}>
                Cancel
              </button>
            )}
            <button className="btn primary" type="submit" disabled={!nameOk}>
              Next →
            </button>
          </div>
        </form>
      )}

      {step === 'files' && (
        <>
          <FilePicker files={files} onFiles={setFiles} examples={p.examples} />
          <div className="row" style={{ justifyContent: 'flex-end' }}>
            {idx > 0 && (
              <button className="btn" onClick={() => setStep(steps[idx - 1])}>
                ← Back
              </button>
            )}
            <button className="btn primary" disabled={!files.length || busy} onClick={doPreview}>
              {busy ? <span className="spinner" /> : null} Dry-run preview →
            </button>
          </div>
        </>
      )}

      {step === 'preview' && preview && (
        <>
          <ImportReportView report={preview} onUseMapping={(m) => setConfig(baseConfig(name, desc) + m)} />
          <div className="row" style={{ justifyContent: 'flex-end' }}>
            <button className="btn" onClick={() => setStep('files')}>
              ← Back
            </button>
            <button className="btn primary" onClick={() => setStep('config')}>
              Edit config →
            </button>
          </div>
        </>
      )}

      {step === 'config' && (
        <>
          <ConfigEditor value={config} onChange={setConfig} validate={(y) => p.validate(y, name)} initialResult={p.initialConfigResult} showEffective={false} minRows={14} />
          <div className="row" style={{ justifyContent: 'flex-end' }}>
            <button className="btn" onClick={() => setStep('preview')}>
              ← Back
            </button>
            <button className="btn primary" onClick={doRun} disabled={busy}>
              {p.mode === 'create' ? `Create “${name}”` : 'Import'}
            </button>
          </div>
        </>
      )}

      {step === 'run' && (
        <>
          <div className="progress" role="progressbar" aria-valuenow={pct} aria-valuemin={0} aria-valuemax={100}>
            <div style={{ width: pct + '%' }} />
          </div>
          <div className="xs muted mono">
            {pr ? `${pr.done}/${pr.total} · ${pr.file ?? ''}` : busy ? 'starting…' : result ? 'done' : ''}
          </div>
          {busy && !result && <Loading text={p.mode === 'create' ? 'Creating database and importing…' : 'Importing…'} />}
          {result && <ImportReportView report={result} />}
          {result && (
            <div className="row" style={{ justifyContent: 'flex-end' }}>
              <button className="btn primary" onClick={() => p.onDone?.(name)}>
                {p.mode === 'create' ? `Open “${name}”` : 'Done'}
              </button>
            </div>
          )}
        </>
      )}
    </div>
  );
}
