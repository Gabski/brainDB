// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useState } from 'react';
import type { KindInput } from '../../api/types';
import { Slider } from '../common/Slider';

/** Define a custom kind: name, colour, half-life, base weight. */
export function NewKindForm({ onAdd, busy }: { onAdd: (k: KindInput) => void; busy?: boolean }) {
  const [name, setName] = useState('');
  const [color, setColor] = useState('#63e6be');
  const [halfLife, setHalfLife] = useState(14);
  const [base, setBase] = useState(0.5);
  return (
    <form
      className="col"
      onSubmit={(e) => {
        e.preventDefault();
        if (!name.trim()) return;
        onAdd({ name: name.trim(), color, halfLife, base });
        setName('');
      }}
    >
      <div className="row nowrap">
        <input className="input grow" aria-label="Kind name" placeholder="e.g. entity, source, requirement" value={name} onChange={(e) => setName(e.target.value)} />
        <input type="color" aria-label="Kind colour" value={color} onChange={(e) => setColor(e.target.value)} style={{ width: 32, height: 26, padding: 0, border: '1px solid var(--line)', borderRadius: 4, background: 'none' }} />
      </div>
      <Slider label="half-life (days)" value={halfLife} min={1} max={120} step={1} digits={0} onChange={setHalfLife} />
      <Slider label="base weight" value={base} min={0.1} max={1} step={0.05} onChange={setBase} />
      <button className="btn" type="submit" disabled={busy || !name.trim()}>
        Add kind
      </button>
    </form>
  );
}
