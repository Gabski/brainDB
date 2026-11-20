// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useEffect, useState } from 'react';
import { useStore, useStoreApi } from '../../state/StoreProvider';
import { isReviewer } from '../../state/store';
import { ErrorBox, Loading } from '../common/Async';
import type { QuarantineItem } from '../../api/types';
import { Icon } from '../common/Icon';
import { Modal } from '../common/Modal';

export interface ReviewQueueProps {
  items: QuarantineItem[];
  sections: string[];
  onAdmit: (ids: string[], section?: string) => void;
  onReject: (ids: string[], reason: string) => void;
  onGo?: (id: string) => void;
  accept?: number;
  review?: number;
}

/** Quarantined items (review band) with Admit / Admit into… / Reject with reason. */
export function ReviewQueue({ items, sections, onAdmit, onReject, onGo, accept = 0.35, review = 0.2 }: ReviewQueueProps) {
  const [sel, setSel] = useState<Set<string>>(new Set());
  const [dlg, setDlg] = useState<null | { kind: 'into' | 'reject'; ids: string[] }>(null);
  const [section, setSection] = useState(sections[0] ?? '');
  const [reason, setReason] = useState('');
  const all = items.length > 0 && sel.size === items.length;
  const ids = [...sel];
  return (
    <div className="split-v">
      <div className="toolbar">
        <b className="small">
          <Icon name="inbox" size={14} /> Review queue
        </b>
        <span className="xs muted">
          {items.length} quarantined · review band {review.toFixed(2)}–{accept.toFixed(2)} · invisible to everyone without the review grant
        </span>
        <span className="spacer" />
        <button className="btn" disabled={!ids.length} onClick={() => onAdmit(ids)}>
          <Icon name="ok" /> Admit {ids.length || ''}
        </button>
        <button className="btn" disabled={!ids.length} onClick={() => setDlg({ kind: 'into', ids })}>
          <Icon name="layers" /> Admit into…
        </button>
        <button className="btn danger" disabled={!ids.length} onClick={() => setDlg({ kind: 'reject', ids })}>
          <Icon name="denied" /> Reject
        </button>
      </div>
      <div className="tab-scroll" style={{ padding: 0 }}>
        {!items.length ? (
          <div className="empty">
            <Icon name="ok" /> Nothing waiting for review.
          </div>
        ) : (
          <table className="tbl">
            <thead>
              <tr>
                <th style={{ width: 28 }}>
                  <input type="checkbox" aria-label="Select all" checked={all} onChange={() => setSel(all ? new Set() : new Set(items.map((i) => i.id)))} />
                </th>
                <th>id</th>
                <th>text</th>
                <th className="num">score</th>
                <th>reasons</th>
                <th>intended section</th>
                <th>by</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {items.map((it) => (
                <tr key={it.id}>
                  <td>
                    <input
                      type="checkbox"
                      aria-label={'Select ' + it.id}
                      checked={sel.has(it.id)}
                      onChange={() => {
                        const n = new Set(sel);
                        if (n.has(it.id)) n.delete(it.id);
                        else n.add(it.id);
                        setSel(n);
                      }}
                    />
                  </td>
                  <td>
                    <button className="link mono" onClick={() => onGo?.(it.id)}>
                      {it.id}
                    </button>
                    <div className="xs muted">{it.kind}</div>
                  </td>
                  <td style={{ whiteSpace: 'normal', maxWidth: 360 }}>{it.text}</td>
                  <td className="num warn">{it.score.toFixed(2)}</td>
                  <td style={{ whiteSpace: 'normal', maxWidth: 300 }} className="xs">
                    {it.reasons.map((r, i) => (
                      <div key={i}>{r}</div>
                    ))}
                  </td>
                  <td className="mono xs">{it.intendedSection ?? '—'}</td>
                  <td className="xs muted">{it.by ?? ''}</td>
                  <td>
                    <span className="row nowrap">
                      <button className="btn sm" onClick={() => onAdmit([it.id])} title="Admit into the intended section">
                        <Icon name="ok" size={14} /> Admit
                      </button>
                      <button className="btn sm" onClick={() => setDlg({ kind: 'into', ids: [it.id] })}>
                        Into…
                      </button>
                      <button className="btn sm danger" onClick={() => setDlg({ kind: 'reject', ids: [it.id] })}>
                        <Icon name="denied" size={14} /> Reject
                      </button>
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
      {dlg && (
        <Modal title={dlg.kind === 'into' ? `Admit ${dlg.ids.join(', ')} into…` : `Reject ${dlg.ids.join(', ')}`} onClose={() => setDlg(null)} width={460}>
          <form
            className="col"
            onSubmit={(e) => {
              e.preventDefault();
              if (dlg.kind === 'into') onAdmit(dlg.ids, section);
              else if (reason.trim()) onReject(dlg.ids, reason.trim());
              else return;
              setDlg(null);
              setSel(new Set());
              setReason('');
            }}
          >
            {dlg.kind === 'into' ? (
              <label className="field">
                <span className="lbl-row">section</span>
                <input className="input" list="rq-secs" value={section} onChange={(e) => setSection(e.target.value)} autoFocus />
                <datalist id="rq-secs">
                  {sections.map((s) => (
                    <option key={s} value={s} />
                  ))}
                </datalist>
              </label>
            ) : (
              <label className="field">
                <span className="lbl-row">reason (required, journaled)</span>
                <input className="input" value={reason} onChange={(e) => setReason(e.target.value)} placeholder="off-topic" autoFocus />
              </label>
            )}
            <div className="row" style={{ justifyContent: 'flex-end' }}>
              <button type="button" className="btn" onClick={() => setDlg(null)}>
                Cancel
              </button>
              <button className={'btn primary' + (dlg.kind === 'reject' ? ' danger' : '')} type="submit" disabled={dlg.kind === 'reject' ? !reason.trim() : !section.trim()}>
                {dlg.kind === 'into' ? 'Admit' : 'Reject'}
              </button>
            </div>
          </form>
        </Modal>
      )}
    </div>
  );
}

/** Connected Review queue tab. */
export function ReviewTab() {
  const store = useStoreApi();
  const q = useStore((s) => s.quarantine);
  const who = useStore((s) => s.whoami);
  const brain = useStore((s) => s.brain);
  const purpose = useStore((s) => s.purpose.data);
  useEffect(() => {
    if (!q.data && !q.loading) store.loadQuarantine();
  }, [q.data, q.loading, store]);
  if (who && !isReviewer(who)) return <div className="tab-scroll"><ErrorBox error="The review queue needs the review grant." /></div>;
  if (q.loading && !q.data) return <Loading />;
  if (q.error && !q.data) return <div className="tab-scroll"><ErrorBox error={q.error} onRetry={() => store.loadQuarantine()} /></div>;
  return (
    <ReviewQueue
      items={q.data ?? []}
      sections={brain?.sections ?? []}
      accept={purpose?.admission.accept}
      review={purpose?.admission.review}
      onAdmit={(ids, sec) => store.admitItems(ids, sec)}
      onReject={(ids, reason) => store.rejectItems(ids, reason)}
      onGo={(id) => store.select(id, true)}
    />
  );
}
