// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useEffect } from 'react';

export function Toast({ text, ok, onClose }: { text: string; ok: boolean; onClose: () => void }) {
  useEffect(() => {
    const t = setTimeout(onClose, ok ? 3500 : 8000);
    return () => clearTimeout(t);
  }, [text, ok, onClose]);
  return (
    <div className={'toast' + (ok ? '' : ' bad')} role={ok ? 'status' : 'alert'}>
      <span className="grow">{text}</span>
      <button className="btn ghost sm icon" aria-label="Dismiss" onClick={onClose}>
        ×
      </button>
    </div>
  );
}
