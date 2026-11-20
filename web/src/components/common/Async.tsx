// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { ReactNode } from 'react';

export function Loading({ text = 'Loading…' }: { text?: string }) {
  return (
    <div className="loading-row" role="status">
      <span className="spinner" aria-hidden /> {text}
    </div>
  );
}

export function ErrorBox({ error, onRetry }: { error: string; onRetry?: () => void }) {
  return (
    <div className="err-box" role="alert">
      {error}
      {onRetry && (
        <div style={{ marginTop: 6 }}>
          <button className="btn sm" onClick={onRetry}>
            Retry
          </button>
        </div>
      )}
    </div>
  );
}

export function Empty({ children }: { children: ReactNode }) {
  return <div className="empty">{children}</div>;
}

export function SectionH({ children, count }: { children: ReactNode; count?: number | string }) {
  return (
    <h3 className="section-h">
      {children}
      {count !== undefined && <span className="count">{count}</span>}
    </h3>
  );
}
