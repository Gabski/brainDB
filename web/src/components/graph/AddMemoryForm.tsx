// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useMemo, useState } from 'react';
import type { GraphIndex } from '../../model/graph';
import { otherEnd } from '../../model/graph';
import { stems } from '../../model/util';

export interface AddMemoryFormProps {
  g: GraphIndex;
  onAdd: (kind: string, text: string, concepts: string[], fields?: Record<string, string>) => void;
  busy?: boolean;
}

/** Add a memory: kind, text, concepts (hinted from words), optional fields for records. */
export function AddMemoryForm({ g, onAdd, busy }: AddMemoryFormProps) {
  const kinds = [...g.kinds.values()].filter((k) => k.name !== 'concept');
  const [kind, setKind] = useState(kinds[0]?.name ?? 'fact');
  const [text, setText] = useState('');
  const [picked, setPicked] = useState<string[]>([]);
  const [fields, setFields] = useState('');
  // hint: a concept is suggested when one of its members shares a word stem with the text
  const hinted = useMemo(() => {
    const s = stems(text);
    if (!s.size) return new Set<string>();
    const out = new Set<string>();
    for (const c of g.conceptIds) {
      const cn = g.nodes.get(c);
      const words = [cn?.text ?? '', ...(g.adj.get(c) ?? []).map((e) => g.nodes.get(otherEnd(e, c))?.text ?? '')];
      if (words.some((w) => [...stems(w)].some((x) => s.has(x)))) out.add(c);
    }
    return out;
  }, [text, g]);
  const submit = () => {
    if (!text.trim()) return;
    const concepts = picked.length ? picked : [...hinted].slice(0, 2);
    const f: Record<string, string> = {};
    fields
      .split(/[\n,]/)
      .map((x) => x.split('='))
      .forEach(([k, v]) => {
        if (k?.trim() && v !== undefined) f[k.trim()] = v.trim();
      });
    onAdd(kind, text.trim(), concepts, Object.keys(f).length ? f : undefined);
    setText('');
    setPicked([]);
    setFields('');
  };
  return (
    <form
      className="col"
      onSubmit={(e) => {
        e.preventDefault();
        submit();
      }}
    >
      <select className="input" aria-label="Kind" value={kind} onChange={(e) => setKind(e.target.value)}>
        {kinds.map((k) => (
          <option key={k.name} value={k.name}>
            {k.label}
          </option>
        ))}
      </select>
      <input className="input" aria-label="Memory text" placeholder="memory text" value={text} onChange={(e) => setText(e.target.value)} />
      <div className="xs muted">concepts (dashed = suggested from the text)</div>
      <div className="chips">
        {g.conceptIds.map((c) => (
          <button
            type="button"
            key={c}
            className={'chip' + (picked.includes(c) ? ' on' : hinted.has(c) ? ' hint' : '')}
            aria-pressed={picked.includes(c)}
            onClick={() => setPicked(picked.includes(c) ? picked.filter((x) => x !== c) : [...picked, c])}
          >
            ◆ {g.nodes.get(c)?.text}
          </button>
        ))}
      </div>
      {kind === 'record' && (
        <input className="input" aria-label="Fields" placeholder="fields: stage=won, value=12000" value={fields} onChange={(e) => setFields(e.target.value)} />
      )}
      <button className="btn primary" type="submit" disabled={busy || !text.trim()}>
        Add and connect
      </button>
    </form>
  );
}
