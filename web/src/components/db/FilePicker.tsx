// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useRef, useState } from 'react';
import type { ImportFile } from '../../api/types';
import { filesFromDrop, filesFromInput } from './collectFiles';

const fmt = (b: number) => (b > 1e6 ? (b / 1e6).toFixed(1) + ' MB' : (b / 1e3).toFixed(1) + ' KB');

/** Drop many files or whole folders (relative paths kept), or pick files / a folder. */
export function FilePicker({ files, onFiles, onPickExample, examples }: { files: ImportFile[]; onFiles: (f: ImportFile[]) => void; onPickExample?: (name: string) => void; examples?: string[] }) {
  const fileIn = useRef<HTMLInputElement>(null);
  const dirIn = useRef<HTMLInputElement | null>(null);
  const [over, setOver] = useState(false);
  const [busy, setBusy] = useState(false);
  const merge = (add: ImportFile[]) => {
    const m = new Map(files.map((f) => [f.path, f]));
    add.forEach((f) => m.set(f.path, f));
    onFiles([...m.values()]);
  };
  const total = files.reduce((a, f) => a + f.file.size, 0);
  return (
    <div className="col">
      <div
        className={'drop' + (over ? ' over' : '')}
        style={{ padding: 24 }}
        onDragOver={(e) => {
          e.preventDefault();
          setOver(true);
        }}
        onDragLeave={() => setOver(false)}
        onDrop={async (e) => {
          e.preventDefault();
          setOver(false);
          setBusy(true);
          try {
            merge(await filesFromDrop(e.dataTransfer));
          } finally {
            setBusy(false);
          }
        }}
      >
        {busy ? (
          <span className="row" style={{ justifyContent: 'center' }}>
            <span className="spinner" /> reading folders…
          </span>
        ) : (
          <>
            <div style={{ fontSize: 14, color: 'var(--fg)' }}>Drop files or whole folders here</div>
            <div className="xs">md · txt · html · pdf · docx · csv/tsv · json/jsonl · yaml · sql dumps · sqlite · images</div>
          </>
        )}
        <div className="row" style={{ justifyContent: 'center', marginTop: 10 }}>
          <button type="button" className="btn" onClick={() => fileIn.current?.click()}>
            Choose files…
          </button>
          <button type="button" className="btn" onClick={() => dirIn.current?.click()}>
            Choose folder…
          </button>
          {examples?.length && onPickExample ? (
            <select className="input" aria-label="Example" defaultValue="" onChange={(e) => e.target.value && onPickExample(e.target.value)}>
              <option value="">or pick an example…</option>
              {examples.map((x) => (
                <option key={x}>{x}</option>
              ))}
            </select>
          ) : null}
        </div>
      </div>
      <input ref={fileIn} type="file" multiple hidden onChange={(e) => e.target.files && (merge(filesFromInput(e.target.files)), (e.target.value = ''))} />
      <input
        ref={(el) => {
          dirIn.current = el;
          if (el) {
            el.setAttribute('webkitdirectory', '');
            el.setAttribute('directory', '');
          }
        }}
        type="file"
        multiple
        hidden
        onChange={(e) => e.target.files && (merge(filesFromInput(e.target.files)), (e.target.value = ''))}
      />
      {files.length > 0 && (
        <div className="box" style={{ maxHeight: 220, overflow: 'auto' }}>
          <table className="tbl">
            <thead>
              <tr>
                <th>path</th>
                <th className="num">size</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {files.map((f) => (
                <tr key={f.path}>
                  <td className="mono">{f.path}</td>
                  <td className="num">{fmt(f.file.size)}</td>
                  <td>
                    <button className="btn sm ghost icon" aria-label={'Remove ' + f.path} onClick={() => onFiles(files.filter((x) => x !== f))}>
                      ×
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {files.length > 0 && (
        <div className="xs muted">
          {files.length} files · {fmt(total)}
        </div>
      )}
    </div>
  );
}
