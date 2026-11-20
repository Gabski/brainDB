// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useCallback, useEffect, useMemo, useState } from 'react';
import type { ConfigResult, EmbedProvider, Kind } from '../../../api/types';
import { Icon } from '../../common/Icon';
import { Modal } from '../../common/Modal';
import { ConfigEditor } from '../ConfigEditor';
import { BoolField, type Ctl, InfoRow, NumField, RangeField, SelectField, TextField } from './fields';
import { addListItem, diffLines, ensureBlockMap, getYamlNumber, getYamlString, getYamlValue, listItems, removeYamlKey, setYamlValue, YamlEditUnsupported, type Scalar } from '../../../model/yamlEdit';

const SECTIONS = [
  { id: 'overview', label: 'Overview', icon: 'info' },
  { id: 'context', label: 'Context', icon: 'focus' },
  { id: 'kinds', label: 'Kinds & decay', icon: 'layers' },
  { id: 'embedding', label: 'Embeddings', icon: 'hybrid' },
  { id: 'import', label: 'Import', icon: 'upload' },
  { id: 'storage', label: 'Storage', icon: 'database' },
  { id: 'purpose', label: 'Purpose', icon: 'goal' },
  { id: 'access', label: 'Access & MCP', icon: 'lock' },
  { id: 'raw', label: 'Raw YAML', icon: 'code' },
] as const;
type SectionId = (typeof SECTIONS)[number]['id'];

export interface DbSettingsProps {
  db?: string | null;
  /** The saved file, for the diff. */
  saved: string;
  /** The edited file. */
  draft: string;
  onDraft: (yaml: string) => void;
  validate: (yaml: string) => Promise<ConfigResult>;
  initialResult?: ConfigResult;
  onSave: () => void | Promise<void>;
  onReload: () => void;
  saving?: boolean;
  /** Kinds as the server has them, used to show what a default actually is. */
  kinds?: Kind[];
  /** What the server actually runs on right now, for the Storage section. */
  storage?: { backend?: string; format?: string; nodes?: number; edges?: number; db?: string };
  /** Switch the storage backend (Addendum F5). Omitted when the build or the user cannot do it. */
  onMigrate?: (to: 'file' | 'sqlite') => void | Promise<void>;
  /** The embedding providers this server accepts (GET /api/embedding). */
  providers?: EmbedProvider[];
  readOnly?: boolean;
}

/**
 * Database settings: `braindb.yaml` as a form, one section at a time, with the raw file always one click away.
 *
 * The forms edit the text, not a parsed model, so comments survive and only the touched line changes. Keys the
 * forms do not cover (import rules, purpose topics, roles, patterns) are shown read-only with a pointer to the raw
 * editor, which is honest about where the boundary is.
 */
export function DbSettings(p: DbSettingsProps) {
  const [section, setSection] = useState<SectionId>('overview');
  const [res, setRes] = useState<ConfigResult | null>(p.initialResult ?? null);
  const [checking, setChecking] = useState(false);
  const [note, setNote] = useState<string | null>(null);
  const [showDiff, setShowDiff] = useState(false);
  const dirty = p.draft !== p.saved;

  useEffect(() => {
    let live = true;
    const t = setTimeout(async () => {
      setChecking(true);
      try {
        const r = await p.validate(p.draft);
        if (live) setRes(r);
      } catch {
        /* the editor reports its own validation errors; the toolbar just stops claiming "valid" */
        if (live) setRes(null);
      } finally {
        if (live) setChecking(false);
      }
    }, 400);
    return () => {
      live = false;
      clearTimeout(t);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [p.draft]);

  const apply = useCallback(
    (edit: (yaml: string) => string) => {
      try {
        p.onDraft(edit(p.draft));
        setNote(null);
      } catch (e) {
        setNote(e instanceof YamlEditUnsupported ? `${e.path}: ${e.message}` : e instanceof Error ? e.message : String(e));
      }
    },
    [p],
  );
  const set = useCallback((path: string, v: Scalar) => apply((y) => setYamlValue(y, path, v)), [apply]);
  const unset = useCallback((path: string) => apply((y) => removeYamlKey(y, path)), [apply]);
  const ctl: Ctl = { yaml: p.draft, set, unset, apply, readOnly: p.readOnly };
  const diff = useMemo(() => (showDiff ? diffLines(p.saved, p.draft) : []), [showDiff, p.saved, p.draft]);
  const errs = res?.errors ?? [];

  return (
    <div className="split-v">
      <div className="toolbar">
        <b className="small">braindb.yaml</b>
        <span className="muted xs">
          {p.db ? `database ${p.db} · ` : ''}precedence: CLI flags &gt; env &gt; braindb.yaml &gt; defaults
        </span>
        <span className="spacer" />
        {checking ? (
          <span className="row xs muted">
            <span className="spinner" /> validating…
          </span>
        ) : errs.length > 0 ? (
          <span className="xs bad">
            <Icon name="error" /> {errs.length} error{errs.length > 1 ? 's' : ''}, first at line {errs[0].line}
          </span>
        ) : res ? (
          <span className="xs muted">
            <Icon name="ok" /> valid
          </span>
        ) : null}
        <button className="btn" disabled={!dirty} onClick={() => setShowDiff(true)}>
          <Icon name="code" /> Diff{dirty ? ` (${diffLines(p.saved, p.draft).length})` : ''}
        </button>
        <button className="btn" onClick={p.onReload}>
          <Icon name="refresh" /> Reload
        </button>
        <button className="btn primary" disabled={!!p.saving || !dirty || errs.length > 0 || !!p.readOnly} onClick={() => p.onSave()}>
          <Icon name="save" /> Save
        </button>
      </div>
      <div className="set-nav" role="tablist" aria-label="Settings section">
        {SECTIONS.map((s) => (
          <button key={s.id} role="tab" className={'chip' + (section === s.id ? ' on' : '')} aria-selected={section === s.id} onClick={() => setSection(s.id)}>
            <Icon name={s.icon} /> {s.label}
          </button>
        ))}
      </div>
      {note ? (
        <div className="row small" style={{ padding: '6px 10px' }}>
          <span className="warn">
            <Icon name="warning" /> {note}
          </span>
          <button className="btn sm" onClick={() => setSection('raw')}>
            Open Raw YAML
          </button>
        </div>
      ) : null}
      <div className="tab-scroll">
        {section === 'overview' && <Overview ctl={ctl} draft={p.draft} />}
        {section === 'context' && <Context ctl={ctl} />}
        {section === 'kinds' && <Kinds ctl={ctl} kinds={p.kinds ?? []} />}
        {section === 'embedding' && <Embeddings ctl={ctl} providers={p.providers} />}
        {section === 'import' && <Import ctl={ctl} />}
        {section === 'storage' && <Storage ctl={ctl} live={p.storage} onMigrate={p.onMigrate} />}
        {section === 'purpose' && <PurposePanel ctl={ctl} />}
        {section === 'access' && <AccessMcp ctl={ctl} />}
        {section === 'raw' && (
          <ConfigEditor value={p.draft} onChange={p.onDraft} validate={p.validate} initialResult={res ?? p.initialResult} />
        )}
      </div>
      {showDiff ? (
        <Modal title={`Changes to braindb.yaml (${diff.length})`} onClose={() => setShowDiff(false)} width={900}>
          {diff.length === 0 ? (
            <p className="muted small">Nothing changed.</p>
          ) : (
            <table className="grid">
              <thead>
                <tr>
                  <th style={{ width: 60 }}>line</th>
                  <th>before</th>
                  <th>after</th>
                </tr>
              </thead>
              <tbody>
                {diff.map((d, i) => (
                  <tr key={i}>
                    <td className="num">{d.line}</td>
                    <td className="mono">{d.before ?? <span className="null">(added)</span>}</td>
                    <td className="mono">{d.after ?? <span className="null">(removed)</span>}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </Modal>
      ) : null}
    </div>
  );
}

function Card({ title, hint, children }: { title: string; hint?: string; children: React.ReactNode }) {
  return (
    <div className="card">
      <div className="card-h">
        {title} {hint ? <span className="muted xs">{hint}</span> : null}
      </div>
      <div className="card-b">{children}</div>
    </div>
  );
}

function listOf(yaml: string, path: string): string {
  const v = getYamlValue(yaml, path);
  return v ?? '—';
}

function Overview({ ctl, draft }: { ctl: Ctl; draft: string }) {
  const lines = draft.split('\n');
  const comments = lines.filter((l) => l.trim().startsWith('#')).length;
  const kinds = listItems(draft, 'kinds', 'name');
  const rules = listItems(draft, 'import.rules', 'match');
  const sources = listItems(draft, 'sources', 'name');
  const users = listItems(draft, 'access.users', 'name');
  return (
    <div className="cards">
      <Card title="Identity" hint="what this database is and what it is for">
        <TextField ctl={ctl} path="name" label="Name" help="Database name; also what federation uses to address it" width={320} />
        <TextField ctl={ctl} path="description" label="Description" width={420} />
        <TextField ctl={ctl} path="goal" label="Goal" help="Shipped with the context, so an agent knows what it is working towards" width={520} />
      </Card>
      <Card title="What this file configures" hint="counts read from the file itself">
        <InfoRow label="Custom kinds" value={kinds.length ? kinds.join(', ') : '—'} />
        <InfoRow label="Sections" value={listOf(draft, 'sections')} />
        <InfoRow label="Import rules" value={rules.length ? `${rules.length}: ${rules.join(', ')}` : '—'} help="Edited in Raw YAML" />
        <InfoRow label="Federated sources" value={sources.length ? sources.join(', ') : '—'} />
        <InfoRow label="Users" value={users.length ? users.join(', ') : '—'} />
        <InfoRow label="Templates directory" value={listOf(draft, 'templates.dir')} />
        <InfoRow label="File" value={`${lines.length} lines, ${comments} comment lines`} help="Form edits change one line at a time and keep every comment" />
      </Card>
    </div>
  );
}

function Context({ ctl }: { ctl: Ctl }) {
  return (
    <div className="cards">
      <Card title="Context defaults" hint="focus + depth + threshold + damping decide what a turn sees">
        <NumField ctl={ctl} path="context.depth" label="Depth" help="How many levels activation spreads from the focus" fallback={2} min={0} max={6} />
        <RangeField ctl={ctl} path="context.threshold" label="Threshold" help="Activation below this drops out of the slice. Raise it on a densely linked brain" fallback={0.16} min={0} max={0.6} step={0.01} />
        <RangeField ctl={ctl} path="context.hop" label="Hop damping" help="Activation retained per level" fallback={0.85} min={0.5} max={1} step={0.01} />
        <BoolField ctl={ctl} path="context.showSuperseded" label="Show superseded" help="Include replaced memories, faded, in the slice" fallback={false} />
        <InfoRow label="Focus" value={listOf(ctl.yaml, 'context.focus')} help="A list of ids: set it here in Raw YAML, or with CONTEXT FOCUS … SET" />
      </Card>
      <Card title="Budget" hint="the warning on the status line, not a hard limit">
        <NumField ctl={ctl} path="budget.warnTokens" label="Warn at tokens" fallback={4000} min={0} step={500} width={130} />
      </Card>
    </div>
  );
}

const KIND_FIELDS = [
  { key: 'halfLife', label: 'half-life (d)', step: 1 },
  { key: 'floor', label: 'floor', step: 0.05 },
  { key: 'base', label: 'base', step: 0.05 },
] as const;

function Kinds({ ctl, kinds }: { ctl: Ctl; kinds: Kind[] }) {
  const inFile = listItems(ctl.yaml, 'kinds', 'name');
  const notInFile = kinds.filter((k) => !inFile.includes(k.name));
  const [add, setAdd] = useState('');
  return (
    <div className="cards">
      <Card title="Kinds in this file" hint="custom kinds, and overrides of the built-in decay policy">
        {inFile.length === 0 ? (
          <p className="muted small">No kinds are declared here, so every kind uses the built-in policy below.</p>
        ) : (
          <table className="grid">
            <thead>
              <tr>
                <th>kind</th>
                <th>prefix</th>
                <th>section</th>
                {KIND_FIELDS.map((f) => (
                  <th key={f.key}>{f.label}</th>
                ))}
                <th style={{ width: 40 }} />
              </tr>
            </thead>
            <tbody>
              {inFile.map((name) => (
                <tr key={name}>
                  <td className="mono">{name}</td>
                  <td className="mono">{getYamlString(ctl.yaml, `kinds[name=${name}].prefix`) ?? '—'}</td>
                  <td className="mono">{getYamlString(ctl.yaml, `kinds[name=${name}].section`) ?? '—'}</td>
                  {KIND_FIELDS.map((f) => (
                    <td key={f.key}>
                      <input
                        type="number"
                        step={f.step}
                        style={{ width: 80 }}
                        disabled={ctl.readOnly}
                        value={getYamlNumber(ctl.yaml, `kinds[name=${name}].${f.key}`) ?? ''}
                        onChange={(e) =>
                          e.target.value === '' ? ctl.unset(`kinds[name=${name}].${f.key}`) : ctl.set(`kinds[name=${name}].${f.key}`, Number(e.target.value))
                        }
                      />
                    </td>
                  ))}
                  <td>
                    <button className="btn sm icon" title="Remove this kind entry" disabled={ctl.readOnly} onClick={() => ctl.unset(`kinds[name=${name}]`)}>
                      <Icon name="trash" />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        <div className="row" style={{ marginTop: 8 }}>
          <select value={add} disabled={ctl.readOnly} onChange={(e) => setAdd(e.target.value)}>
            <option value="">override a built-in kind…</option>
            {notInFile.map((k) => (
              <option key={k.name} value={k.name}>
                {k.name}
              </option>
            ))}
          </select>
          <button
            className="btn"
            disabled={!add || !!ctl.readOnly}
            onClick={() => {
              const k = kinds.find((x) => x.name === add);
              if (!k) return;
              const fields: Record<string, Scalar> = { name: k.name };
              if (k.halfLife != null) fields.halfLife = k.halfLife;
              if (k.floor != null) fields.floor = k.floor;
              fields.base = k.base;
              ctl.apply((y) => addListItem(y, 'kinds', fields));
              setAdd('');
            }}
          >
            <Icon name="add" /> Add override
          </button>
          <span className="muted xs">Starts from the values in force, so saving changes nothing until you edit a number.</span>
        </div>
      </Card>
      <Card title="Decay policy in force" hint="from the server; a kind listed above overrides its row">
        <table className="grid">
          <thead>
            <tr>
              <th>kind</th>
              <th>half-life</th>
              <th>floor</th>
              <th>base</th>
              <th>prefix</th>
              <th>section</th>
            </tr>
          </thead>
          <tbody>
            {kinds.map((k) => (
              <tr key={k.name}>
                <td className="mono">
                  {k.name} {k.builtin ? null : <span className="chip xs">custom</span>}
                </td>
                <td className="num mono">{k.halfLife == null ? 'never' : k.halfLife + ' d'}</td>
                <td className="num mono">{k.floor == null ? '—' : k.floor}</td>
                <td className="num mono">{k.base}</td>
                <td className="mono">{k.prefix}</td>
                <td className="mono">{k.section}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </Card>
    </div>
  );
}

const FALLBACK_PROVIDERS: EmbedProvider[] = [
  { name: 'hash', label: 'Built-in hash-256 (offline, lexical)', wire: 'hash', needsKey: false },
  { name: 'openai', label: 'OpenAI-compatible endpoint', wire: 'openai', needsKey: true },
];

function Embeddings({ ctl, providers }: { ctl: Ctl; providers?: EmbedProvider[] }) {
  const list = providers?.length ? providers : FALLBACK_PROVIDERS;
  const name = getYamlString(ctl.yaml, 'embedding.provider') ?? 'hash';
  const spec = list.find((x) => x.name === name);
  const local = !spec || spec.wire === 'hash';
  return (
    <div className="cards">
      <Card title="Engine" hint="hash-256 is offline and free; everything else is a hosted or self-hosted model">
        <SelectField
          ctl={ctl}
          path="embedding.provider"
          label="Provider"
          help="The server owns this list; a name it does not know is rejected when the file is saved"
          fallback="hash"
          options={list.map((x) => ({ value: x.name, label: x.wire === 'hash' ? x.label : `${x.label} · ${x.wire}` }))}
        />
        {local ? (
          <p className="small warn">
            <Icon name="warning" /> The built-in engine indexes word shapes, not meaning. Measured over two example brains, 8 to 14 of every 16 queries phrased
            in other words than the stored text return nothing relevant. Pick a real model for anything an agent asks in a user&apos;s words.
          </p>
        ) : (
          <>
            <InfoRow
              label="This provider"
              value={
                <>
                  wire <b>{spec!.wire}</b>
                  {spec!.maxBatch ? <> · batch ceiling {spec!.maxBatch}</> : null}
                  {spec!.inputType ? <> · input type {spec!.inputType}</> : null}
                  <br />
                  endpoint {spec!.baseUrl ?? <span className="warn">must be set below</span>}
                  {spec!.keyEnv ? (
                    <>
                      <br />
                      key usually in {spec!.keyEnv}
                    </>
                  ) : null}
                </>
              }
              help={spec!.note}
            />
            {spec!.baseUrl || spec!.model || spec!.keyEnv ? (
              <div className="row" style={{ margin: '4px 0 8px' }}>
                <button
                  className="btn"
                  disabled={!!ctl.readOnly}
                  onClick={() =>
                    ctl.apply((y) => {
                      let out = y;
                      if (spec!.model) out = setYamlValue(out, 'embedding.model', spec!.model);
                      if (spec!.keyEnv && spec!.needsKey) out = setYamlValue(out, 'embedding.apiKeyEnv', spec!.keyEnv);
                      if (!spec!.baseUrl) out = setYamlValue(out, 'embedding.baseUrl', '');
                      return out;
                    })
                  }
                >
                  <Icon name="add" /> Fill this provider&apos;s defaults
                </button>
                <span className="muted xs">Writes the example model and the conventional key variable; it never writes a key.</span>
              </div>
            ) : null}
          </>
        )}
        <TextField ctl={ctl} path="embedding.model" label="Model" placeholder={spec?.model ?? 'provider model id'} width={320} />
        <TextField
          ctl={ctl}
          path="embedding.baseUrl"
          label="Base URL"
          help={spec && !spec.baseUrl ? 'required for this provider' : 'overrides the provider default'}
          placeholder={spec?.baseUrl ?? 'http://host:8000/v1'}
          width={320}
        />
        <TextField
          ctl={ctl}
          path="embedding.apiKeyEnv"
          label="API key env var"
          help="The name of the variable, never the key itself"
          placeholder={spec?.keyEnv ?? 'PROVIDER_API_KEY'}
          width={240}
        />
        {spec?.inputType ? (
          <TextField
            ctl={ctl}
            path="embedding.inputType"
            label="Input type"
            help="Asymmetric models want to know whether a text is a document or a query. A brain stores documents."
            fallback={spec.inputType}
            placeholder={spec.inputType}
            width={240}
          />
        ) : null}
        {spec?.auth === 'azure' ? (
          <TextField ctl={ctl} path="embedding.apiVersion" label="API version" help="Azure's api-version query parameter" fallback="2024-02-01" placeholder="2024-02-01" width={160} />
        ) : null}
        <NumField ctl={ctl} path="embedding.dim" label="Dimension" fallback={spec && !local ? 'learned from the first response' : 256} min={16} step={16} />
        <NumField ctl={ctl} path="embedding.batch" label="Batch size" help={spec?.maxBatch ? `clamped to ${spec.maxBatch} for this provider` : undefined} fallback={64} min={1} />
        <SelectField
          ctl={ctl}
          path="embedding.fallback"
          label="On failure"
          fallback="hash"
          options={[
            { value: 'hash', label: 'hash — degrade and report the engine as degraded' },
            { value: 'error', label: 'error — fail the operation' },
          ]}
        />
        <TextField ctl={ctl} path="embedding.cache" label="Cache path" help="Vectors are keyed by sha256(model + text), so a re-import costs nothing" width={280} />
      </Card>
      <Card title="Graph enrichment" hint="h = (1 − α)·x + α·mean_w(neighbours), so a vector carries its neighbourhood">
        <RangeField ctl={ctl} path="embedding.alpha" label="Alpha" fallback={0.5} min={0} max={1} step={0.05} />
        <NumField ctl={ctl} path="embedding.hops" label="Hops" fallback={2} min={0} max={4} />
        <SelectField ctl={ctl} path="embedding.use" label="Use" fallback="enriched" options={[{ value: 'enriched' }, { value: 'raw' }]} />
        <RangeField ctl={ctl} path="embedding.lambda" label="Lambda" help="Weight of the enriched vector against the raw one" fallback={0.5} min={0} max={1} step={0.05} />
      </Card>
    </div>
  );
}

function Import({ ctl }: { ctl: Ctl }) {
  const rules = listItems(ctl.yaml, 'import.rules', 'match');
  return (
    <div className="cards">
      <Card title="Ingest" hint="how files become chunks and records">
        <BoolField ctl={ctl} path="import.keepRaw" label="Keep raw text" help="Store the original text of an imported file next to its chunks" fallback={false} />
        <NumField ctl={ctl} path="import.chunkSize" label="Chunk size" help="Characters per chunk before splitting" fallback={650} min={100} step={50} width={120} />
        <NumField ctl={ctl} path="import.limits.fileSize" label="Max file size" help="Bytes; 0 means the built-in limit" min={0} step={1024} width={140} />
      </Card>
      <Card title="Rules" hint="which importer handles which files">
        {rules.length === 0 ? (
          <p className="muted small">No rules. Importers are then chosen by file extension.</p>
        ) : (
          <table className="grid">
            <thead>
              <tr>
                <th>match</th>
                <th>importer</th>
                <th>kind</th>
                <th>section</th>
              </tr>
            </thead>
            <tbody>
              {rules.map((m) => (
                <tr key={m}>
                  <td className="mono">{m}</td>
                  <td className="mono">{getYamlString(ctl.yaml, `import.rules[match=${m}].importer`) ?? '—'}</td>
                  <td className="mono">{getYamlString(ctl.yaml, `import.rules[match=${m}].kind`) ?? '—'}</td>
                  <td className="mono">{getYamlString(ctl.yaml, `import.rules[match=${m}].section`) ?? '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        <p className="muted xs">A rule carries a text pattern and a concept list too; both are edited in Raw YAML.</p>
      </Card>
    </div>
  );
}

function Storage({ ctl, live, onMigrate }: { ctl: Ctl; live?: DbSettingsProps['storage']; onMigrate?: DbSettingsProps['onMigrate'] }) {
  const configured = getYamlString(ctl.yaml, 'storage.backend');
  const current = live?.backend;
  const other: 'file' | 'sqlite' = current === 'sqlite' ? 'file' : 'sqlite';
  const [confirm, setConfirm] = useState(false);
  const [busy, setBusy] = useState(false);
  return (
    <div className="cards">
      <Card title="Backend" hint="file keeps one readable brain.xml; sqlite adds the WAL, the indexes and concurrent readers">
        <SelectField
          ctl={ctl}
          path="storage.backend"
          label="Backend"
          help="Written into braindb.yaml. It takes effect on the next start, and does not convert an existing brain by itself"
          fallback="file"
          options={[
            { value: 'file', label: 'file — brain.xml, readable and diff-friendly' },
            { value: 'sqlite', label: 'sqlite — brain.db' },
          ]}
        />
        <TextField ctl={ctl} path="storage.path" label="Path" help="Overrides where the brain is read from and written to" placeholder="data/brain.db" width={320} />
        {configured && current && configured !== current ? (
          <p className="small warn">
            <Icon name="warning" /> The file says <b>{configured}</b> and the server is running on <b>{current}</b>. Convert below, or restart the server to pick
            the configured backend up.
          </p>
        ) : null}
      </Card>
      <Card title="Running on" hint="what the server reports right now, not what the file asks for">
        <InfoRow label="Backend" value={current ?? '—'} />
        <InfoRow label="Format" value={live?.format ?? '—'} help="The on-disk format of a file backend: xml, bin or both" />
        <InfoRow label="Database" value={live?.db ?? '—'} />
        <InfoRow label="Nodes / edges" value={live?.nodes != null ? `${live.nodes} / ${live.edges ?? '—'}` : '—'} />
        {onMigrate ? (
          <div className="row" style={{ marginTop: 8 }}>
            {confirm ? (
              <>
                <span className="small warn">
                  <Icon name="warning" /> Convert the live brain to <b>{other}</b>? It is written back in the new backend and the server keeps serving it.
                </span>
                <button
                  className="btn primary"
                  disabled={busy}
                  onClick={async () => {
                    setBusy(true);
                    try {
                      await onMigrate(other);
                    } finally {
                      setBusy(false);
                      setConfirm(false);
                    }
                  }}
                >
                  <Icon name="database" /> Convert to {other}
                </button>
                <button className="btn" disabled={busy} onClick={() => setConfirm(false)}>
                  Cancel
                </button>
              </>
            ) : (
              <>
                <button className="btn" disabled={!!ctl.readOnly} onClick={() => setConfirm(true)}>
                  <Icon name="database" /> Convert to {other}…
                </button>
                <span className="muted xs">Same content, other backend. The CLI spelling is `braindb migrate --brain … --to {other}`.</span>
              </>
            )}
          </div>
        ) : (
          <p className="muted xs">This build or this user cannot switch the backend from the UI.</p>
        )}
      </Card>
    </div>
  );
}

function PurposePanel({ ctl }: { ctl: Ctl }) {
  const mode = getYamlString(ctl.yaml, 'purpose.admission.mode') ?? 'advise';
  return (
    <div className="cards">
      <Card title="Admission" hint="every incoming item is scored against the purpose, with reasons">
        <SelectField
          ctl={ctl}
          path="purpose.admission.mode"
          label="Mode"
          fallback="advise"
          options={[
            { value: 'off', label: 'off — do not score' },
            { value: 'advise', label: 'advise — score, record, store everything' },
            { value: 'enforce', label: 'enforce — quarantine or reject below the thresholds' },
          ]}
        />
        {mode === 'enforce' ? (
          <p className="small warn">
            <Icon name="warning" /> Enforce drops knowledge on a similarity score. Read what advise mode has been scoring low before you rely on it.
          </p>
        ) : null}
        <RangeField ctl={ctl} path="purpose.admission.accept" label="Accept at" fallback={0.35} min={0} max={1} step={0.01} />
        <RangeField ctl={ctl} path="purpose.admission.review" label="Review at" help="Between review and accept, an item is quarantined instead of stored" fallback={0.2} min={0} max={1} step={0.01} />
        <InfoRow label="Applies to" value={listOf(ctl.yaml, 'purpose.admission.apply')} />
        <InfoRow label="Exempt actors" value={listOf(ctl.yaml, 'purpose.admission.exempt')} />
      </Card>
      <Card title="What belongs in this brain" hint="the text an agent receives with its context">
        <TextField ctl={ctl} path="purpose.name" label="Purpose name" width={360} />
        <InfoRow label="Description" value={getYamlString(ctl.yaml, 'purpose.description') ?? '—'} help="Multi-line text: edited in Raw YAML" />
        <InfoRow label="In-scope topics" value={listOf(ctl.yaml, 'purpose.inScope.topics')} />
        <InfoRow label="Out-of-scope topics" value={listOf(ctl.yaml, 'purpose.outOfScope.topics')} />
        <InfoRow
          label="Forbidden entities"
          value={listOf(ctl.yaml, 'purpose.outOfScope.entities')}
          help="An empty list forbids nothing; listing card rejects anything carrying a card number"
        />
      </Card>
    </div>
  );
}

const TOKEN_SCOPES = [
  { value: 'read', label: 'read' },
  { value: 'write', label: 'write' },
  { value: 'admin', label: 'admin' },
] as const;

const ENV_NAME_RE = /^[A-Za-z_][A-Za-z0-9_]*$/;

function AccessMcp({ ctl }: { ctl: Ctl }) {
  const roles = listItems(ctl.yaml, 'access.roles', 'name');
  const users = listItems(ctl.yaml, 'access.users', 'name');
  const labels = listItems(ctl.yaml, 'access.labels', 'name');
  return (
    <div className="cards">
      <Card title="Access" hint="what a user without clearance cannot see does not exist for them">
        <BoolField ctl={ctl} path="access.enabled" label="Enabled" fallback={false} />
        <InfoRow label="Labels" value={labels.length ? labels.join(', ') : '—'} />
        <InfoRow label="Roles" value={roles.length ? roles.join(', ') : '—'} help="Grants are edited in Raw YAML or in the Access tab" />
        <InfoRow label="Users" value={users.length ? users.join(', ') : '—'} />
      </Card>
      <ServerTokens ctl={ctl} />
      <Card title="MCP" hint="the tool surface an agent gets for this database">
        <TextField ctl={ctl} path="mcp.actor" label="Actor" help="Name recorded on everything the agent writes" width={200} />
        <InfoRow label="Tools" value={listOf(ctl.yaml, 'mcp.tools')} help="Default is view + run; the full set is view, search, inspect, query, fill, exec, ingest, sources" />
        <InfoRow
          label="Remote access"
          value={<code>Authorization: Bearer $TOKEN</code>}
          help="An agent reaching /mcp over HTTP authenticates with a token from the card above. A read token may call /mcp, but its writes are refused."
        />
      </Card>
    </div>
  );
}

/**
 * Server tokens: the API and `/mcp` credentials for this server.
 *
 * Every entry names an **environment variable**, never a token. The server reads the variable at start and logs a
 * warning when it is empty, so a token that is configured here but missing from the environment is not silently
 * treated as access. The form refuses anything that is not a variable name, which is also what stops a pasted
 * secret from landing in a file that is meant to be committed.
 */
function ServerTokens({ ctl }: { ctl: Ctl }) {
  const names = listItems(ctl.yaml, 'server.tokens', 'name');
  const [name, setName] = useState('');
  const [envVar, setEnvVar] = useState('');
  const [scope, setScope] = useState<string>('read');
  const trimmedName = name.trim();
  const trimmedEnv = envVar.trim();
  const duplicate = trimmedName !== '' && names.includes(trimmedName);
  const envLooksLikeSecret = trimmedEnv !== '' && !ENV_NAME_RE.test(trimmedEnv);
  const canAdd = trimmedName !== '' && !duplicate && ENV_NAME_RE.test(trimmedEnv);
  return (
    <Card title="Server tokens" hint="names of environment variables, never the tokens themselves">
      <TextField
        ctl={ctl}
        path="server.tokenEnv"
        label="Admin token variable"
        placeholder="BRAINDB_TOKEN"
        help="Variable holding the admin token. Ignored when BRAINDB_TOKEN is already set in the environment."
        fallback="no token, so the server serves loopback only"
        width={220}
      />
      <BoolField
        ctl={ctl}
        path="server.allowNoToken"
        label="Allow listening without a token"
        fallback={false}
        help="Only for a container published on 127.0.0.1. Ignored as soon as one token is configured."
      />
      {names.length === 0 ? (
        <p className="muted small">No scoped tokens. The API is open to whoever reaches the socket unless the admin variable above is set.</p>
      ) : (
        <table className="grid">
          <thead>
            <tr>
              <th>name</th>
              <th>environment variable</th>
              <th style={{ width: 110 }}>scope</th>
              <th>databases</th>
              <th style={{ width: 40 }} />
            </tr>
          </thead>
          <tbody>
            {names.map((n) => {
              const env = getYamlString(ctl.yaml, `server.tokens[name=${n}].tokenEnv`) ?? '';
              const bad = env !== '' && !ENV_NAME_RE.test(env);
              return (
                <tr key={n}>
                  <td className="mono">{n}</td>
                  <td>
                    <input
                      style={{ width: 200 }}
                      className={bad ? 'bad' : undefined}
                      value={env}
                      disabled={ctl.readOnly}
                      placeholder="BRAINDB_TOKEN_BOT"
                      onChange={(e) => ctl.set(`server.tokens[name=${n}].tokenEnv`, e.target.value)}
                    />
                  </td>
                  <td>
                    <select
                      value={getYamlString(ctl.yaml, `server.tokens[name=${n}].scope`) ?? 'read'}
                      disabled={ctl.readOnly}
                      onChange={(e) => ctl.set(`server.tokens[name=${n}].scope`, e.target.value)}
                    >
                      {TOKEN_SCOPES.map((o) => (
                        <option key={o.value} value={o.value}>
                          {o.label}
                        </option>
                      ))}
                    </select>
                  </td>
                  <td className="mono xs">{getYamlValue(ctl.yaml, `server.tokens[name=${n}].dbs`) ?? 'all'}</td>
                  <td>
                    <button className="btn sm icon" title="Remove this token entry" disabled={ctl.readOnly} onClick={() => ctl.unset(`server.tokens[name=${n}]`)}>
                      <Icon name="trash" />
                    </button>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
      <div className="row" style={{ marginTop: 8 }}>
        <input style={{ width: 140 }} placeholder="name, e.g. bot" value={name} disabled={ctl.readOnly} onChange={(e) => setName(e.target.value)} />
        <input
          style={{ width: 200 }}
          className={envLooksLikeSecret ? 'bad' : undefined}
          placeholder="BRAINDB_TOKEN_BOT"
          value={envVar}
          disabled={ctl.readOnly}
          onChange={(e) => setEnvVar(e.target.value)}
        />
        <select value={scope} disabled={ctl.readOnly} onChange={(e) => setScope(e.target.value)}>
          {TOKEN_SCOPES.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
        <button
          className="btn"
          disabled={!canAdd || !!ctl.readOnly}
          onClick={() => {
            ctl.apply((y) => addListItem(ensureBlockMap(y, 'server'), 'server.tokens', { name: trimmedName, tokenEnv: trimmedEnv, scope }));
            setName('');
            setEnvVar('');
            setScope('read');
          }}
        >
          <Icon name="add" /> Add token
        </button>
      </div>
      <p className="muted xs" style={{ marginTop: 6 }}>
        {duplicate ? (
          <span className="bad">A token named {trimmedName} is already in the file.</span>
        ) : envLooksLikeSecret ? (
          <span className="bad">That is not a variable name. Put the token in the environment and name the variable here — the file must never hold the value.</span>
        ) : (
          <>
            read = queries, context and search · write = read plus exec and import · admin = everything, including config and access. Databases are limited with a{' '}
            <code>dbs</code> list in Raw YAML; the server warns on start when a variable named here is empty.
          </>
        )}
      </p>
    </Card>
  );
}
