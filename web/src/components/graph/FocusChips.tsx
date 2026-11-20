// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { GraphIndex } from '../../model/graph';
import { short } from '../../model/util';

export interface FocusChipsProps {
  g: GraphIndex;
  focus: string[];
  onSelect: (id: string) => void;
  onRemove: (id: string) => void;
  onGoal?: () => void;
  onClear?: () => void;
}

/** Focus = activation seeds. Click a chip to inspect it, × to remove it from the focus. */
export function FocusChips({ g, focus, onSelect, onRemove, onGoal, onClear }: FocusChipsProps) {
  return (
    <div>
      <div className="chips" aria-label="Focus">
        {focus.length ? (
          focus.map((id) => {
            const n = g.nodes.get(id);
            return (
              <span key={id} className="chip" title={n?.text ?? id}>
                <button className="link" style={{ color: 'inherit', display: 'inline-flex', gap: 4, alignItems: 'center' }} onClick={() => onSelect(id)}>
                  <i className="dot" style={{ background: g.color(id) }} />
                  <span className="mono xs">{id}</span> {short(n?.text ?? '', 18)}
                </button>
                <button className="x" aria-label={'Remove ' + id + ' from focus'} onClick={() => onRemove(id)}>
                  ×
                </button>
              </span>
            );
          })
        ) : (
          <span className="muted small">none — nothing is lit</span>
        )}
      </div>
      {(onGoal || onClear) && (
        <div className="row" style={{ marginTop: 6 }}>
          {onGoal && (
            <button className="btn sm" onClick={onGoal}>
              Focus = goal
            </button>
          )}
          {onClear && (
            <button className="btn sm" onClick={onClear}>
              Clear focus
            </button>
          )}
        </div>
      )}
    </div>
  );
}
