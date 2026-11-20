// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useEffect, useState } from 'react';
import type { Entity, Pattern } from '../../api/types';
import { EntityText } from '../entities/EntityText';
import { ErrorBox, Loading } from '../common/Async';
import { Icon } from '../common/Icon';

export interface PatternsPanelProps {
  patterns?: Pattern[];
  loading?: boolean;
  error?: string;
  onToggle: (name: string, enabled: boolean) => void;
  onAdd: (p: { name: string; regex: string; type: string; pii: boolean }) => void;
  onRemove: (name: string) => void;
  test: (text: string) => Promise<Entity[]>;
  initialText?: string;
}

/** Pattern library (Addendum I): built-in + custom patterns, enable/disable, add, live tester. */
export function PatternsPanel({ patterns, loading, error, onToggle, onAdd, onRemove, test, initialText = 'Order SKU-10422 for Kim Aalto, €1,200 due 2026-12-15, call +48 600 100 200' }: PatternsPanelProps) {
  const [text, setText] = useState(initialText);
  const [ents, setEnts] = useState<Entity[]>([]);
  const [terr, setTerr] = useState<string | null>(null);
  const [nw, setNw] = useState({ name: '', regex: '', type: 'id_code', pii: false });
  useEffect(() => {
    let live = true;
    const t = setTimeout(async () => {
      try {
        const r = await test(text);
        if (live) {
          setEnts(r);
          setTerr(null);
        }
      } catch (e) {
        if (live) setTerr(e instanceof Error ? e.message : String(e));
      }
    }, 250);
    return () => {
      live = false;
      clearTimeout(t);
    };
  }, [text, test, patterns]);
  let regexErr: string | null = null;
  try {
    if (nw.regex) new RegExp(nw.regex);
  } catch (e) {
    regexErr = (e as Error).message;
  }
  return (
    <div>
      <label className="field">
        <span className="lbl-row">Live tester — type any text</span>
        <textarea className="input" rows={2} value={text} onChange={(e) => setText(e.target.value)} aria-label="Pattern tester text" />
      </label>
      <div className="box" style={{ padding: 8, minHeight: 34, lineHeight: 1.9 }} aria-live="polite">
        <EntityText text={text} entities={ents} />
      </div>
      <div className="xs muted" style={{ margin: '4px 0 8px' }}>
        {terr ? <span className="bad">{terr}</span> : `${ents.length} match${ents.length === 1 ? '' : 'es'}: ${ents.map((e) => e.type).join(', ')}`}
      </div>
      {loading && !patterns ? (
        <Loading />
      ) : error && !patterns ? (
        <ErrorBox error={error} />
      ) : (
        <table className="tbl">
          <thead>
            <tr>
              <th>on</th>
              <th>name</th>
              <th>type</th>
              <th>source</th>
              <th>PII</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {(patterns ?? []).map((p) => (
              <tr key={p.name}>
                <td>
                  <input type="checkbox" aria-label={`Enable ${p.name}`} checked={p.enabled} onChange={(e) => onToggle(p.name, e.target.checked)} />
                </td>
                <td className="mono">{p.name}</td>
                <td className="mono xs">{p.type}</td>
                <td className="mono xs muted ellipsis" style={{ maxWidth: 200 }} title={p.regex}>
                  {p.builtin ? `built-in${p.locales ? ' · ' + p.locales.join(',') : ''}` : p.regex}
                </td>
                <td>{p.pii ? <Icon name="lock" size={14} label="PII" /> : ''}</td>
                <td>
                  {!p.builtin && (
                    <button className="btn sm danger" onClick={() => onRemove(p.name)}>
                      Remove
                    </button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <form
        className="row"
        style={{ marginTop: 8 }}
        onSubmit={(e) => {
          e.preventDefault();
          if (nw.name && nw.regex && !regexErr) {
            onAdd(nw);
            setNw({ name: '', regex: '', type: 'id_code', pii: false });
          }
        }}
      >
        <input className="input" style={{ width: 90 }} aria-label="Pattern name" placeholder="name" value={nw.name} onChange={(e) => setNw({ ...nw, name: e.target.value.replace(/\W/g, '_') })} />
        <input className="input grow mono" aria-label="Pattern regex" placeholder="\bSKU-\d{5}\b" value={nw.regex} onChange={(e) => setNw({ ...nw, regex: e.target.value })} />
        <input className="input" style={{ width: 90 }} aria-label="Entity type" placeholder="type" value={nw.type} onChange={(e) => setNw({ ...nw, type: e.target.value })} />
        <label className="check small">
          <input type="checkbox" checked={nw.pii} onChange={(e) => setNw({ ...nw, pii: e.target.checked })} /> PII
        </label>
        <button className="btn" type="submit" disabled={!nw.name || !nw.regex || !!regexErr}>
          <Icon name="add" /> Add pattern
        </button>
        {regexErr && <span className="xs bad">{regexErr}</span>}
      </form>
    </div>
  );
}
