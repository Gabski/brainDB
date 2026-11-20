// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useEffect, useRef, type ReactNode } from 'react';

export function Modal({ title, onClose, children, width }: { title: ReactNode; onClose: () => void; children: ReactNode; width?: number }) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const prev = document.activeElement as HTMLElement | null;
    ref.current?.querySelector<HTMLElement>('input,textarea,select,button')?.focus();
    return () => prev?.focus?.();
  }, []);
  return (
    <div className="modal-back" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div
        className="modal"
        role="dialog"
        aria-modal="true"
        ref={ref}
        style={width ? { width: `min(${width}px, 94vw)` } : undefined}
        onKeyDown={(e) => {
          if (e.key === 'Escape') {
            e.stopPropagation();
            onClose();
          }
        }}
      >
        <div className="modal-h">
          {title}
          <span className="spacer" />
          <button className="btn ghost sm icon" aria-label="Close" onClick={onClose}>
            ×
          </button>
        </div>
        <div className="modal-b">{children}</div>
      </div>
    </div>
  );
}

export function ConfirmDialog({ title, text, confirmLabel = 'Confirm', danger, onConfirm, onCancel }: { title: string; text: ReactNode; confirmLabel?: string; danger?: boolean; onConfirm: () => void; onCancel: () => void }) {
  return (
    <Modal title={title} onClose={onCancel} width={440}>
      <div style={{ marginBottom: 12 }}>{text}</div>
      <div className="row" style={{ justifyContent: 'flex-end' }}>
        <button className="btn" onClick={onCancel}>
          Cancel
        </button>
        <button className={'btn primary' + (danger ? ' danger' : '')} onClick={onConfirm}>
          {confirmLabel}
        </button>
      </div>
    </Modal>
  );
}
