// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

export interface SliderProps {
  label: string;
  value: number;
  min: number;
  max: number;
  step: number;
  digits?: number;
  onChange: (v: number) => void;
  id?: string;
}

/** Labelled range input with the current value on the right. */
export function Slider({ label, value, min, max, step, digits = 2, onChange, id }: SliderProps) {
  const sid = id ?? 'sl-' + label.replace(/\W+/g, '-').toLowerCase();
  return (
    <div className="field">
      <label className="lbl-row" htmlFor={sid}>
        {label} <b>{digits === 0 ? value : value.toFixed(digits)}</b>
      </label>
      <input id={sid} type="range" min={min} max={max} step={step} value={value} onChange={(e) => onChange(+e.target.value)} />
    </div>
  );
}
