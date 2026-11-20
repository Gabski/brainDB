// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useEffect, useState } from 'react';
import { useApi } from '../../api/ApiProvider';
import { useStore, useStoreApi } from '../../state/StoreProvider';
import type { FillOutput, FillResult, TemplateFormat } from '../../api/types';
import { CodeEditor } from '../common/CodeEditor';
import { CodeView } from '../common/CodeView';
import { ErrorBox, Loading, SectionH } from '../common/Async';
import { EvidenceTable } from './EvidenceTable';
import { guessLang, type Lang } from '../../model/highlight';
import { copyText, downloadText } from '../../model/util';
import { Icon } from '../common/Icon';

export interface TemplateTabProps {
  /** Pre-filled result (stories). */
  initialResult?: FillResult;
  initialError?: string;
}

const langOf = (f: string, content: string): Lang => (f === 'auto' ? guessLang(undefined, content) === 'text' ? 'yaml' : guessLang(undefined, content) : (f as Lang));

/** Template fill (A4): editor + picker, formats, filled preview and per-slot evidence. */
export function TemplateTab({ initialResult, initialError }: TemplateTabProps) {
  const api = useApi();
  const store = useStoreApi();
  const templates = useStore((s) => s.templates);
  const draft = useStore((s) => s.templateDraft);
  const [content, setContent] = useState(draft?.content ?? templates.data?.[0]?.content ?? '');
  const [name, setName] = useState(draft?.name ?? templates.data?.[0]?.name ?? 'template');
  const [tf, setTf] = useState<TemplateFormat>('auto');
  const [out, setOut] = useState<FillOutput>('same');
  const [scope, setScope] = useState<'memory' | 'context'>('memory');
  const [withSources, setWithSources] = useState(false);
  const [min, setMin] = useState(0.2);
  const [res, setRes] = useState<FillResult | null>(initialResult ?? null);
  const [err, setErr] = useState<string | null>(initialError ?? null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (draft) {
      setContent(draft.content);
      setName(draft.name ?? 'template');
    }
  }, [draft?.token]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (!content && templates.data?.[0]) {
      setContent(templates.data[0].content);
      setName(templates.data[0].name);
    }
  }, [templates.data]); // eslint-disable-line react-hooks/exhaustive-deps

  const fill = async () => {
    setLoading(true);
    setErr(null);
    try {
      setRes(await api.fill({ template: content, templateFormat: tf, output: out, scope, withSources, min }));
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };
  const ext = res?.format ?? 'txt';
  return (
    <div className="split-h">
      <div className="pane" style={{ flex: 1, borderRight: '1px solid var(--line)' }}>
        <div className="toolbar">
          <select
            className="input"
            aria-label="Template"
            value={templates.data?.some((t) => t.name === name) ? name : ''}
            onChange={(e) => {
              const t = templates.data?.find((x) => x.name === e.target.value);
              if (t) {
                setContent(t.content);
                setName(t.name);
              }
            }}
          >
            <option value="">(custom)</option>
            {templates.data?.map((t) => (
              <option key={t.path} value={t.name}>
                {t.name}
              </option>
            ))}
          </select>
          {templates.loading && <span className="spinner" />}
          <label className="row small">
            template
            <select className="input" value={tf} onChange={(e) => setTf(e.target.value as TemplateFormat)}>
              {['auto', 'json', 'yaml', 'xml', 'md'].map((f) => (
                <option key={f}>{f}</option>
              ))}
            </select>
          </label>
          <label className="row small">
            output
            <select className="input" value={out} onChange={(e) => setOut(e.target.value as FillOutput)}>
              {['same', 'json', 'yaml', 'xml', 'md'].map((f) => (
                <option key={f}>{f}</option>
              ))}
            </select>
          </label>
          <label className="row small">
            scope
            <select className="input" value={scope} onChange={(e) => setScope(e.target.value as 'memory' | 'context')}>
              <option>memory</option>
              <option>context</option>
            </select>
          </label>
          <label className="row small">
            min
            <input className="input" type="number" step={0.05} min={0} max={1} value={min} onChange={(e) => setMin(+e.target.value)} style={{ width: 64 }} />
          </label>
          <label className="check small">
            <input type="checkbox" checked={withSources} onChange={(e) => setWithSources(e.target.checked)} /> sources
          </label>
          <button className="btn primary" onClick={fill} disabled={loading || !content.trim()}>
            <Icon name="play" /> Fill
          </button>
        </div>
        <div className="tab-scroll">
          <CodeEditor value={content} onChange={setContent} lang={langOf(tf, content)} onRun={fill} onRunAll={fill} minRows={14} ariaLabel="Template editor" />
          <div className="xs muted" style={{ marginTop: 6 }}>
            Slots: <code>"description[:type][|option…]"</code> in JSON/YAML/XML, or <code>{'{{ description:type }}'}</code> and <code>{'{{#each list|kind=…}}…{{/each}}'}</code> in text. Types: text, name, number, money, percent, date, email, phone,
            url, bool, list, id, org, domain, duration, iban, vat, address, version.
          </div>
        </div>
      </div>
      <div className="pane" style={{ flex: 1 }}>
        <div className="toolbar">
          <b className="small">Filled preview</b>
          {res && (
            <span className={'small ' + (res.missing.length ? 'warn' : 'ok')}>
              {res.values.length - res.missing.length}/{res.values.length} slots filled{res.missing.length ? ` · ${res.missing.length} missing` : ''}
            </span>
          )}
          <span className="spacer" />
          <button className="btn" disabled={!res} onClick={async () => res && (await copyText(res.output)) && store.toastMsg('Copied output')}>
            Copy
          </button>
          <button className="btn" disabled={!res} onClick={() => res && downloadText(`${name.replace(/\.\w+$/, '')}.filled.${ext}`, res.output)}>
            <Icon name="download" /> Download
          </button>
        </div>
        <div className="tab-scroll">
          {loading ? (
            <Loading text="Filling…" />
          ) : err ? (
            <ErrorBox error={err} onRetry={fill} />
          ) : res ? (
            <>
              <CodeView text={res.output} lang={guessLang(res.format, res.output)} maxHeight={320} />
              <SectionH count={res.values.length}>Evidence</SectionH>
              <EvidenceTable values={res.values} missing={res.missing} onSource={(id) => store.select(id, true)} />
              {res.missing.length > 0 && (
                <div className="xs muted" style={{ marginTop: 6 }}>
                  Missing slots scored below <b>min</b>. Complete them, then store the values with BDQL (e.g. <code>REMEMBER fact "…"</code>).
                </div>
              )}
            </>
          ) : (
            <div className="empty">Pick or write a template and press Fill.</div>
          )}
        </div>
      </div>
    </div>
  );
}
