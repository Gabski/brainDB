// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useCallback, useEffect, useState } from 'react';
import { useStore, useStoreApi } from '../../state/StoreProvider';
import type { ConfigResult, EmbedProvider } from '../../api/types';
import { ImportWizard } from './ImportWizard';
import { DbSettings } from './settings/DbSettings';
import { Modal } from '../common/Modal';
import { ErrorBox, Loading } from '../common/Async';
import { PurposeEditor } from '../purpose/PurposeEditor';
import type { Purpose } from '../../api/types';
import { Icon } from '../common/Icon';

/** Import tab: the import flow into the active database. */
export function ImportTab() {
  const store = useStoreApi();
  const db = useStore((s) => s.db);
  const progress = useStore((s) => s.importProgress);
  const [key, setKey] = useState(0);
  return (
    <div className="tab-scroll" style={{ maxWidth: 1000 }}>
      <ImportWizard
        key={key}
        mode="import"
        dbName={db ?? undefined}
        progress={progress}
        dryRun={(files, config) => store.api.importFiles(files, { db: db ?? undefined, config: config || undefined, dryRun: true })}
        validate={(y) => store.api.putConfig(y, { db: db ?? undefined, dryRun: true })}
        run={async (files, config) => {
          const r = await store.api.importFiles(files, { db: db ?? undefined, config: config || undefined });
          await store.refresh();
          return r;
        }}
        onDone={() => setKey(key + 1)}
      />
    </div>
  );
}

/** Settings tab: braindb.yaml editor or the Purpose editor. */
export function SettingsTab({ initialSection = 'config' }: { initialSection?: 'config' | 'purpose' }) {
  const [section, setSection] = useState<'config' | 'purpose'>(initialSection);
  return (
    <div className="split-v">
      <div className="toolbar">
        <div className="seg" aria-label="Settings section">
          <button aria-pressed={section === 'config'} onClick={() => setSection('config')}>
            <Icon name="settings" size={14} /> braindb.yaml
          </button>
          <button aria-pressed={section === 'purpose'} onClick={() => setSection('purpose')}>
            <Icon name="goal" size={14} /> Purpose
          </button>
        </div>
      </div>
      {section === 'config' ? <ConfigSettings /> : <PurposeSettings />}
    </div>
  );
}

function PurposeSettings() {
  const store = useStoreApi();
  const purpose = useStore((s) => s.purpose);
  const brain = useStore((s) => s.brain);
  const check = useCallback((q: { text: string; kind?: string; section?: string }) => store.api.checkPurpose(q), [store]);
  if (purpose.loading && purpose.data === undefined) return <Loading />;
  if (purpose.error && purpose.data === undefined) return <div className="tab-scroll"><ErrorBox error={purpose.error} onRetry={() => store.loadPurpose()} /></div>;
  const value: Purpose = purpose.data ?? {
    name: brain?.name ?? '',
    description: '',
    inScope: { topics: [], concepts: [], kinds: [], sections: [] },
    outOfScope: { topics: [], entities: ['card'] },
    examples: { accept: [], reject: [] },
    admission: { mode: 'advise', accept: 0.35, review: 0.2 },
  };
  return (
    <>
      {!purpose.data && <div className="xs muted" style={{ padding: '6px 12px' }}>No purpose declared yet: the admission filter is off. Describe the brain and save to turn it on.</div>}
      <PurposeEditor value={value} onSave={(p) => store.savePurpose(p)} check={check} kinds={brain?.kinds.map((k) => k.name)} sections={brain?.sections} />
    </>
  );
}

function ConfigSettings() {
  const store = useStoreApi();
  const db = useStore((s) => s.db);
  const brain = useStore((s) => s.brain);
  const [saved, setSaved] = useState<string | null>(null);
  const [draft, setDraft] = useState('');
  const [initial, setInitial] = useState<ConfigResult | undefined>();
  const [err, setErr] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [storage, setStorage] = useState<{ backend?: string; format?: string; nodes?: number; edges?: number; db?: string } | undefined>();
  const [providers, setProviders] = useState<EmbedProvider[] | undefined>();
  const load = useCallback(async () => {
    setErr(null);
    try {
      const r = await store.api.getConfig(db ?? undefined);
      setSaved(r.yaml);
      setDraft(r.yaml);
      setInitial(r.result);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    }
    // the Storage section reports what the server runs on; a build without stats simply shows nothing
    try {
      const [h, st] = await Promise.all([store.api.health(), store.api.stats()]);
      setStorage({ backend: st.storage ?? h.storage, format: st.format ?? h.format, nodes: st.nodes, edges: st.edges, db: st.db ?? db ?? undefined });
    } catch {
      setStorage(undefined);
    }
    try {
      setProviders(await store.api.embeddingProviders());
    } catch {
      setProviders(undefined); // the form falls back to its two built-in entries
    }
  }, [store, db]);
  useEffect(() => {
    load();
  }, [load]);
  const validate = useCallback((y: string) => store.api.putConfig(y, { db: db ?? undefined, dryRun: true }), [store, db]);
  const save = useCallback(async () => {
    setSaving(true);
    try {
      const r = await store.api.putConfig(draft, { db: db ?? undefined });
      if (r.ok) {
        store.toastMsg('braindb.yaml saved');
        setSaved(draft);
        setInitial(r);
        await store.refresh();
      } else store.toastMsg(`Not saved: ${r.errors.length} error(s), first at line ${r.errors[0]?.line}`, false);
    } catch (e) {
      store.toastMsg('save: ' + (e instanceof Error ? e.message : String(e)), false);
    } finally {
      setSaving(false);
    }
  }, [store, db, draft]);
  if (err) return <div className="tab-scroll"><ErrorBox error={err} onRetry={load} /></div>;
  if (saved === null) return <Loading />;
  return (
    <DbSettings
      db={db}
      saved={saved}
      draft={draft}
      onDraft={setDraft}
      validate={validate}
      initialResult={initial}
      onSave={save}
      onReload={load}
      saving={saving}
      kinds={brain?.kinds}
      storage={storage}
      providers={providers}
      onMigrate={async (to) => {
        try {
          await store.api.migrate(to);
          store.toastMsg(`storage backend is now ${to}`);
          await store.refresh();
          await load();
        } catch (e) {
          store.toastMsg('migrate: ' + (e instanceof Error ? e.message : String(e)), false);
        }
      }}
    />
  );
}

/** "New database" wizard in a modal (from the connection selector). */
export function NewDbWizardConnected() {
  const store = useStoreApi();
  const progress = useStore((s) => s.importProgress);
  const [examples, setExamples] = useState<{ name: string; description?: string }[]>([]);
  useEffect(() => {
    store.api.examples().then(setExamples).catch(() => setExamples([]));
  }, [store]);
  const close = () => store.set({ wizardOpen: false });
  return (
    <Modal title="New database" onClose={close} width={980}>
      <ImportWizard
        mode="create"
        progress={progress}
        examples={examples}
        dryRun={(files, config, name) => store.api.importFiles(files, { db: name, config: config || undefined, dryRun: true })}
        validate={(y, name) => store.api.putConfig(y, { db: name, dryRun: true })}
        run={async (files, config, name) => {
          await store.api.createDb(name, config || undefined);
          return store.api.importFiles(files, { db: name, config: config || undefined });
        }}
        onInstallExample={async (example, name) => {
          await store.api.installExample(name, example);
        }}
        onDone={(name) => {
          close();
          store.loadDbs().then(() => store.switchDb(name));
        }}
        onCancel={close}
      />
    </Modal>
  );
}
