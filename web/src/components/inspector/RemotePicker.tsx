// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useState } from 'react';
import type { RemoteHit, Source } from '../../api/types';
import { Modal } from '../common/Modal';
import { ErrorBox, Loading } from '../common/Async';
import { Slider } from '../common/Slider';
import { sourceColor } from '../graph/sceneShared';
import { Icon } from '../common/Icon';

export interface RemotePickerProps {
  from: string;
  sources: Source[];
  search: (source: string, q: string) => Promise<RemoteHit[]>;
  onPick: (remoteId: string, w: number) => void;
  onClose: () => void;
  initialQuery?: string;
  initialHits?: RemoteHit[];
}

/** "Link external…": search a mounted source and link the local node to a remote one. */
export function RemotePicker({ from, sources, search, onPick, onClose, initialQuery = '', initialHits }: RemotePickerProps) {
  const usable = sources.filter((s) => s.enabled && s.status !== 'error');
  const [src, setSrc] = useState(usable[0]?.name ?? '');
  const [q, setQ] = useState(initialQuery);
  const [w, setW] = useState(0.7);
  const [hits, setHits] = useState<RemoteHit[] | null>(initialHits ?? null);
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const run = async () => {
    if (!src || !q.trim()) return;
    setLoading(true);
    setErr(null);
    try {
      setHits(await search(src, q.trim()));
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };
  return (
    <Modal title={<><Icon name="source" /> Link {from} to an external node</>} onClose={onClose} width={620}>
      {!usable.length ? (
        <div className="muted">No healthy source is mounted. Add one in the Sources tab.</div>
      ) : (
        <>
          <form
            className="row nowrap"
            onSubmit={(e) => {
              e.preventDefault();
              run();
            }}
          >
            <select className="input" aria-label="Source" value={src} onChange={(e) => setSrc(e.target.value)}>
              {usable.map((s) => (
                <option key={s.name} value={s.name}>
                  {s.name}
                </option>
              ))}
            </select>
            <input className="input grow" aria-label="Remote search" placeholder="search the source…" value={q} onChange={(e) => setQ(e.target.value)} />
            <button className="btn primary" type="submit">
              Search
            </button>
          </form>
          <Slider label="link weight" value={w} min={0.1} max={1} step={0.05} onChange={setW} />
          {loading && <Loading text="Searching remote source…" />}
          {err && <ErrorBox error={err} />}
          {hits && !loading && (
            <div className="list" style={{ marginTop: 6 }}>
              {!hits.length && <span className="muted small">no results</span>}
              {hits.map((h) => (
                <button key={h.id} className="li" onClick={() => onPick(h.id, w)}>
                  <i className="dot ring" style={{ color: sourceColor(h.id.split(':')[0]) }} />
                  <span className="t">
                    <span className="id">{h.id}</span>
                    {h.text}
                  </span>
                  <span className="n">
                    {h.kind}
                    {h.score !== undefined ? ` · ${h.score.toFixed(2)}` : ''} · link
                  </span>
                </button>
              ))}
            </div>
          )}
        </>
      )}
    </Modal>
  );
}
