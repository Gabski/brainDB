// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useRef, useState } from 'react';

/** Click or drop files to ingest them into the brain. */
export function DropZone({ onFiles, busy, focus, onFocusChange }: { onFiles: (f: File[]) => void; busy?: boolean; focus?: boolean; onFocusChange?: (v: boolean) => void }) {
  const inp = useRef<HTMLInputElement>(null);
  const [over, setOver] = useState(false);
  return (
    <div>
      <button
        type="button"
        className={'drop' + (over ? ' over' : '')}
        onClick={() => inp.current?.click()}
        onDragOver={(e) => {
          if ([...e.dataTransfer.types].includes('Files')) {
            e.preventDefault();
            setOver(true);
          }
        }}
        onDragLeave={() => setOver(false)}
        onDrop={(e) => {
          e.preventDefault();
          setOver(false);
          if (e.dataTransfer.files.length) onFiles([...e.dataTransfer.files]);
        }}
      >
        {busy ? (
          <span className="row" style={{ justifyContent: 'center' }}>
            <span className="spinner" /> ingesting…
          </span>
        ) : (
          <>
            Drop files here or on the brain
            <br />
            <span className="xs">md · txt · csv · code · pdf · images · ctxc state.json / &lt;context&gt; XML</span>
          </>
        )}
      </button>
      <input
        ref={inp}
        type="file"
        multiple
        hidden
        onChange={(e) => {
          if (e.target.files?.length) onFiles([...e.target.files]);
          e.target.value = '';
        }}
      />
      <div className="xs muted" style={{ margin: '4px 0' }}>
        File → node + chunks → embedding → auto-links to concepts (◆) and similar memories (~).
      </div>
      {onFocusChange && (
        <label className="check small">
          <input type="checkbox" checked={!!focus} onChange={(e) => onFocusChange(e.target.checked)} /> add new files to the focus
        </label>
      )}
    </div>
  );
}
