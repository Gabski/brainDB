// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { ReactNode } from 'react';
import { Icon } from '../../common/Icon';
import { getYamlBool, getYamlNumber, getYamlString, getYamlValue, type Scalar } from '../../../model/yamlEdit';

/** What a settings field needs: the current YAML and the two operations that change it. */
export interface Ctl {
  yaml: string;
  set: (path: string, v: Scalar) => void;
  unset: (path: string) => void;
  /** Any other edit the module can make, such as appending a list item. Errors surface the same way. */
  apply: (edit: (yaml: string) => string) => void;
  readOnly?: boolean;
}

interface Common {
  ctl: Ctl;
  path: string;
  label: string;
  help?: ReactNode;
  /** Shown when the key is not in the file: what the server uses instead. */
  fallback?: ReactNode;
}

/**
 * One row of the settings form.
 *
 * The chip on the right is the point: a key is either written in `braindb.yaml` or it is not, and a form that hides
 * that difference teaches the wrong model of the file. "file" means the value is in the YAML and `Reset` removes
 * the line; "default" means nothing is written and the server decides.
 */
function Row({ label, help, set, fallback, onReset, children, readOnly }: { label: string; help?: ReactNode; set: boolean; fallback?: ReactNode; onReset: () => void; children: ReactNode; readOnly?: boolean }) {
  return (
    <div className="set-row">
      <div className="set-label">
        <label>{label}</label>
        {help ? <div className="muted xs">{help}</div> : null}
      </div>
      <div className="set-ctl">{children}</div>
      <div className="set-state xs">
        {set ? (
          <>
            <span className="chip">file</span>
            {readOnly ? null : (
              <button className="btn sm icon" title="Remove the key from braindb.yaml and fall back to the default" onClick={onReset}>
                <Icon name="close" />
              </button>
            )}
          </>
        ) : (
          <span className="muted">default{fallback !== undefined ? <> {fallback}</> : null}</span>
        )}
      </div>
    </div>
  );
}

export function TextField({ ctl, path, label, help, fallback, placeholder, width = 260 }: Common & { placeholder?: string; width?: number }) {
  const v = getYamlString(ctl.yaml, path);
  return (
    <Row label={label} help={help} set={v !== undefined} fallback={fallback} readOnly={ctl.readOnly} onReset={() => ctl.unset(path)}>
      <input
        style={{ width }}
        value={v ?? ''}
        placeholder={placeholder}
        disabled={ctl.readOnly}
        onChange={(e) => (e.target.value === '' ? ctl.unset(path) : ctl.set(path, e.target.value))}
      />
    </Row>
  );
}

export function NumField({ ctl, path, label, help, fallback, min, max, step = 1, width = 110 }: Common & { min?: number; max?: number; step?: number; width?: number }) {
  const v = getYamlNumber(ctl.yaml, path);
  return (
    <Row label={label} help={help} set={getYamlValue(ctl.yaml, path) !== undefined} fallback={fallback} readOnly={ctl.readOnly} onReset={() => ctl.unset(path)}>
      <input
        type="number"
        style={{ width }}
        min={min}
        max={max}
        step={step}
        value={v ?? ''}
        disabled={ctl.readOnly}
        onChange={(e) => (e.target.value === '' ? ctl.unset(path) : ctl.set(path, Number(e.target.value)))}
      />
    </Row>
  );
}

export function BoolField({ ctl, path, label, help, fallback }: Common) {
  const v = getYamlBool(ctl.yaml, path);
  return (
    <Row label={label} help={help} set={v !== undefined} fallback={fallback} readOnly={ctl.readOnly} onReset={() => ctl.unset(path)}>
      <label className="check">
        <input type="checkbox" checked={v ?? false} disabled={ctl.readOnly} onChange={(e) => ctl.set(path, e.target.checked)} /> <span className="xs muted">{String(v ?? false)}</span>
      </label>
    </Row>
  );
}

export function SelectField({ ctl, path, label, help, fallback, options }: Common & { options: { value: string; label?: string }[] }) {
  const v = getYamlString(ctl.yaml, path);
  return (
    <Row label={label} help={help} set={v !== undefined} fallback={fallback} readOnly={ctl.readOnly} onReset={() => ctl.unset(path)}>
      <select value={v ?? ''} disabled={ctl.readOnly} onChange={(e) => (e.target.value === '' ? ctl.unset(path) : ctl.set(path, e.target.value))}>
        <option value="">(default)</option>
        {options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label ?? o.value}
          </option>
        ))}
      </select>
    </Row>
  );
}

/** A number with a slider next to it, for the three activation parameters that are worth feeling rather than typing. */
export function RangeField({ ctl, path, label, help, fallback, min, max, step, digits = 2 }: Common & { min: number; max: number; step: number; digits?: number }) {
  const v = getYamlNumber(ctl.yaml, path);
  const shown = v ?? (typeof fallback === 'number' ? fallback : min);
  return (
    <Row label={label} help={help} set={v !== undefined} fallback={fallback} readOnly={ctl.readOnly} onReset={() => ctl.unset(path)}>
      <div className="row" style={{ gap: 8, width: 260 }}>
        <input type="range" min={min} max={max} step={step} value={shown} disabled={ctl.readOnly} onChange={(e) => ctl.set(path, Number(e.target.value))} />
        <b className="mono xs" style={{ minWidth: 34, textAlign: 'right' }}>
          {digits === 0 ? shown : shown.toFixed(digits)}
        </b>
      </div>
    </Row>
  );
}

/** A read-only row for values the forms do not edit (lists, nested rules), pointing at the raw editor. */
export function InfoRow({ label, value, help }: { label: string; value: ReactNode; help?: ReactNode }) {
  return (
    <div className="set-row">
      <div className="set-label">
        <label>{label}</label>
        {help ? <div className="muted xs">{help}</div> : null}
      </div>
      <div className="set-ctl mono xs">{value}</div>
      <div className="set-state xs" />
    </div>
  );
}
