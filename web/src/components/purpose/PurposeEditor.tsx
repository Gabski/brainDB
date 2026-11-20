// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useEffect, useState } from 'react';
import type { AdmissionMode, Purpose, PurposeCheck } from '../../api/types';
import { Icon } from '../common/Icon';
import { ErrorBox, SectionH } from '../common/Async';
import { CheckResult, ThresholdBand } from './CheckResult';

export interface PurposeEditorProps {
  value: Purpose;
  onSave: (p: Purpose) => void;
  check: (q: { text: string; kind?: string; section?: string }) => Promise<PurposeCheck>;
  saving?: boolean;
  kinds?: string[];
  sections?: string[];
  initialCheckText?: string;
  initialCheck?: PurposeCheck;
}

const lines = (a: string[]) => a.join('\n');
const split = (s: string) => s.split('\n').map((x) => x.trim()).filter(Boolean);

function ListField({ label, value, onChange, rows = 3, hint }: { label: string; value: string[]; onChange: (v: string[]) => void; rows?: number; hint?: string }) {
  const [t, setT] = useState(lines(value));
  useEffect(() => setT(lines(value)), [value]);
  return (
    <label className="field">
      <span className="lbl-row">
        {label} <span className="xs muted">{hint ?? 'one per line'}</span>
      </span>
      <textarea className="input" rows={rows} value={t} onChange={(e) => setT(e.target.value)} onBlur={() => onChange(split(t))} />
    </label>
  );
}

/** Settings → Purpose: what the brain is for, scope, examples, thresholds, mode, plus a live Check box. */
export function PurposeEditor({ value, onSave, check, saving, kinds = [], sections = [], initialCheckText = '', initialCheck }: PurposeEditorProps) {
  const [p, setP] = useState<Purpose>(value);
  useEffect(() => setP(value), [value]);
  const [text, setText] = useState(initialCheckText);
  const [kind, setKind] = useState('');
  const [section, setSection] = useState('');
  const [res, setRes] = useState<PurposeCheck | null>(initialCheck ?? null);
  const [err, setErr] = useState<string | null>(null);
  useEffect(() => {
    if (!text.trim()) return;
    let live = true;
    const t = setTimeout(async () => {
      try {
        const r = await check({ text, kind: kind || undefined, section: section || undefined });
        if (live) {
          setRes(r);
          setErr(null);
        }
      } catch (e) {
        if (live) setErr(e instanceof Error ? e.message : String(e));
      }
    }, 300);
    return () => {
      live = false;
      clearTimeout(t);
    };
  }, [text, kind, section, check]);
  const a = p.admission;
  const setA = (x: Partial<Purpose['admission']>) => setP({ ...p, admission: { ...a, ...x } });
  const bad = a.review > a.accept;
  return (
    <div className="split-h" style={{ gap: 0 }}>
      <div className="tab-scroll" style={{ flex: 3 }}>
        <div className="row">
          <label className="field grow">
            <span className="lbl-row">name</span>
            <input className="input" value={p.name} onChange={(e) => setP({ ...p, name: e.target.value })} />
          </label>
          <label className="field">
            <span className="lbl-row">mode</span>
            <div className="seg" role="radiogroup" aria-label="Admission mode">
              {(['off', 'advise', 'enforce'] as AdmissionMode[]).map((m) => (
                <button key={m} role="radio" aria-checked={a.mode === m} aria-pressed={a.mode === m} onClick={() => setA({ mode: m })}>
                  {m}
                </button>
              ))}
            </div>
          </label>
        </div>
        <label className="field">
          <span className="lbl-row">description (the positive anchor; also shown to agents and in the context header)</span>
          <textarea className="input" rows={3} value={p.description} onChange={(e) => setP({ ...p, description: e.target.value })} style={{ fontFamily: 'var(--font)', fontSize: 13 }} />
        </label>
        <div className="cards" style={{ gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))' }}>
          <div className="card">
            <div className="card-h">
              <Icon name="ok" /> In scope
            </div>
            <div className="card-b">
              <ListField label="topics" value={p.inScope.topics} onChange={(v) => setP({ ...p, inScope: { ...p.inScope, topics: v } })} rows={5} />
              <ListField label="concepts" value={p.inScope.concepts} onChange={(v) => setP({ ...p, inScope: { ...p.inScope, concepts: v } })} hint="ids or labels" />
              <ListField label="kinds" value={p.inScope.kinds} onChange={(v) => setP({ ...p, inScope: { ...p.inScope, kinds: v } })} hint={kinds.length ? 'e.g. ' + kinds.slice(0, 4).join(', ') : undefined} />
              <ListField label="sections" value={p.inScope.sections} onChange={(v) => setP({ ...p, inScope: { ...p.inScope, sections: v } })} hint={sections.length ? 'e.g. ' + sections.slice(0, 4).join(', ') : undefined} />
            </div>
          </div>
          <div className="card">
            <div className="card-h">
              <Icon name="denied" /> Out of scope
            </div>
            <div className="card-b">
              <ListField label="topics" value={p.outOfScope.topics} onChange={(v) => setP({ ...p, outOfScope: { ...p.outOfScope, topics: v } })} rows={4} />
              <ListField label="forbidden entity types" value={p.outOfScope.entities} onChange={(v) => setP({ ...p, outOfScope: { ...p.outOfScope, entities: v } })} hint="always rejected, e.g. card" />
            </div>
          </div>
          <div className="card">
            <div className="card-h">
              <Icon name="docs" /> Examples
            </div>
            <div className="card-b">
              <ListField label="accept" value={p.examples.accept} onChange={(v) => setP({ ...p, examples: { ...p.examples, accept: v } })} />
              <ListField label="reject" value={p.examples.reject} onChange={(v) => setP({ ...p, examples: { ...p.examples, reject: v } })} />
            </div>
          </div>
        </div>
        <SectionH>Thresholds</SectionH>
        <div className="row">
          <label className="field" style={{ width: 200 }}>
            <span className="lbl-row">
              review from <b>{a.review.toFixed(2)}</b>
            </span>
            <input type="range" min={0} max={1} step={0.01} value={a.review} onChange={(e) => setA({ review: +e.target.value })} />
          </label>
          <label className="field" style={{ width: 200 }}>
            <span className="lbl-row">
              accept from <b>{a.accept.toFixed(2)}</b>
            </span>
            <input type="range" min={0} max={1} step={0.01} value={a.accept} onChange={(e) => setA({ accept: +e.target.value })} />
          </label>
          <div className="grow" style={{ minWidth: 220 }}>
            <ThresholdBand accept={Math.max(a.accept, a.review)} review={Math.min(a.accept, a.review)} score={res?.score} />
          </div>
        </div>
        {bad && <div className="xs bad">review must be ≤ accept</div>}
        {p.counters && (
          <div className="xs muted" style={{ marginTop: 6 }}>
            admitted {p.counters.admitted} · quarantined {p.counters.quarantined} · rejected {p.counters.rejected}
          </div>
        )}
        <div className="row" style={{ justifyContent: 'flex-end', marginTop: 10 }}>
          <button className="btn" onClick={() => setP(value)}>
            Reset
          </button>
          <button className="btn primary" disabled={saving || bad || !p.description.trim()} onClick={() => onSave(p)}>
            <Icon name="save" /> Save purpose
          </button>
        </div>
      </div>
      <aside className="graph-side" style={{ width: 340, borderRight: 0, borderLeft: '1px solid var(--line)' }} aria-label="Purpose check">
        <SectionH>
          <Icon name="test" size={14} /> Check
        </SectionH>
        <div className="xs muted" style={{ marginBottom: 6 }}>
          Paste any text: nothing is stored. Scored against the saved purpose.
        </div>
        <textarea className="input" rows={4} aria-label="Text to check" placeholder="e.g. Lunch menu for Friday" value={text} onChange={(e) => setText(e.target.value)} style={{ fontFamily: 'var(--font)', fontSize: 13 }} />
        <div className="row" style={{ margin: '6px 0' }}>
          <input className="input" aria-label="Kind" placeholder="kind" list="pk-kinds" style={{ width: 110 }} value={kind} onChange={(e) => setKind(e.target.value)} />
          <input className="input" aria-label="Section" placeholder="section" list="pk-secs" style={{ width: 110 }} value={section} onChange={(e) => setSection(e.target.value)} />
          <datalist id="pk-kinds">
            {kinds.map((k) => (
              <option key={k} value={k} />
            ))}
          </datalist>
          <datalist id="pk-secs">
            {sections.map((k) => (
              <option key={k} value={k} />
            ))}
          </datalist>
        </div>
        {err && <ErrorBox error={err} />}
        {res && text.trim() ? <CheckResult r={res} accept={a.accept} review={a.review} /> : <div className="empty">Type text to see its score.</div>}
      </aside>
    </div>
  );
}
