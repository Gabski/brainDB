// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useMemo, useState } from 'react';

export interface Command {
  id: string;
  group: string;
  label: string;
  hint?: string;
  run: () => void;
}

/** Ctrl/Cmd+K: run query, open tab, jump to node, set focus, switch view, add source. */
export function CommandPalette({ commands, onClose, initial = '' }: { commands: Command[]; onClose: () => void; initial?: string }) {
  const [q, setQ] = useState(initial);
  const [sel, setSel] = useState(0);
  const list = useMemo(() => {
    const s = q.toLowerCase().trim();
    const words = s.split(/\s+/).filter(Boolean);
    return commands.filter((c) => words.every((w) => (c.group + ' ' + c.label + ' ' + (c.hint ?? '')).toLowerCase().includes(w))).slice(0, 60);
  }, [q, commands]);
  const pick = (c?: Command) => {
    if (!c) return;
    onClose();
    c.run();
  };
  return (
    <div className="modal-back" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="modal palette" role="dialog" aria-label="Command palette">
        <input
          autoFocus
          aria-label="Command"
          placeholder="Type a command, a node id or text…"
          value={q}
          onChange={(e) => {
            setQ(e.target.value);
            setSel(0);
          }}
          onKeyDown={(e) => {
            if (e.key === 'ArrowDown') {
              e.preventDefault();
              setSel((sel + 1) % Math.max(1, list.length));
            } else if (e.key === 'ArrowUp') {
              e.preventDefault();
              setSel((sel - 1 + list.length) % Math.max(1, list.length));
            } else if (e.key === 'Enter') pick(list[sel]);
            else if (e.key === 'Escape') onClose();
          }}
        />
        <div className="items" role="listbox">
          {list.map((c, i) => (
            <div key={c.id} role="option" aria-selected={i === sel} className="item" onMouseEnter={() => setSel(i)} onClick={() => pick(c)}>
              <span className="grp">{c.group}</span>
              <span className="ellipsis">{c.label}</span>
              {c.hint && <span className="hint">{c.hint}</span>}
            </div>
          ))}
          {!list.length && <div className="empty">no match</div>}
        </div>
      </div>
    </div>
  );
}
