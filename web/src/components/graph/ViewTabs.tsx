// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { VIEWS, type ViewName } from '../../model/layout';

export function ViewTabs({ view, onChange }: { view: ViewName; onChange: (v: ViewName) => void }) {
  return (
    <div className="seg" role="tablist" aria-label="Graph view">
      {VIEWS.map((v) => (
        <button key={v.id} role="tab" aria-pressed={v.id === view} aria-selected={v.id === view} onClick={() => onChange(v.id)}>
          {v.label}
        </button>
      ))}
    </div>
  );
}
