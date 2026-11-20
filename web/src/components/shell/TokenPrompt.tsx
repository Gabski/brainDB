// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useState } from 'react';
import { Modal } from '../common/Modal';
import { Icon } from '../common/Icon';

/** Shown when the API answers 401: the server runs with BRAINDB_TOKEN. */
export function TokenPrompt({ onSubmit, onCancel, error }: { onSubmit: (t: string) => void; onCancel?: () => void; error?: string }) {
  const [t, setT] = useState('');
  return (
    <Modal title={<><Icon name="lock" /> API token required</>} onClose={onCancel ?? (() => {})} width={440}>
      <form
        className="col"
        onSubmit={(e) => {
          e.preventDefault();
          if (t.trim()) onSubmit(t.trim());
        }}
      >
        <div className="small muted">
          This brainDB server was started with <code>BRAINDB_TOKEN</code>. Enter the token. It is kept only for this browser session. You can also open the UI with <code>?token=…</code>.
        </div>
        <input className="input mono" type="password" autoComplete="off" aria-label="API token" placeholder="token" value={t} onChange={(e) => setT(e.target.value)} />
        {error && <div className="xs bad">{error}</div>}
        <div className="row" style={{ justifyContent: 'flex-end' }}>
          <button className="btn primary" type="submit" disabled={!t.trim()}>
            Connect
          </button>
        </div>
      </form>
    </Modal>
  );
}
