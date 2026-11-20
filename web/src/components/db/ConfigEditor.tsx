// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useEffect, useRef, useState } from 'react';
import type { ConfigResult } from '../../api/types';
import { CodeEditor } from '../common/CodeEditor';
import { CodeView } from '../common/CodeView';
import { Icon } from '../common/Icon';

export interface ConfigEditorProps {
  value: string;
  onChange: (v: string) => void;
  /** Dry-run validation (PUT /api/config?dryRun=1). */
  validate: (yaml: string) => Promise<ConfigResult>;
  initialResult?: ConfigResult;
  showEffective?: boolean;
  minRows?: number;
}

/** braindb.yaml editor with live validation: errors are shown inline by line, plus the effective config. */
export function ConfigEditor({ value, onChange, validate, initialResult, showEffective = true, minRows = 18 }: ConfigEditorProps) {
  const [res, setRes] = useState<ConfigResult | null>(initialResult ?? null);
  const [checking, setChecking] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const first = useRef(!!initialResult);
  useEffect(() => {
    if (first.current) {
      first.current = false;
      return;
    }
    let live = true;
    const t = setTimeout(async () => {
      setChecking(true);
      try {
        const r = await validate(value);
        if (live) {
          setRes(r);
          setErr(null);
        }
      } catch (e) {
        if (live) setErr(e instanceof Error ? e.message : String(e));
      } finally {
        if (live) setChecking(false);
      }
    }, 400);
    return () => {
      live = false;
      clearTimeout(t);
    };
  }, [value, validate]);
  const eff = res?.effective;
  return (
    <div className="col">
      <div className="row small">
        {checking ? (
          <span className="row">
            <span className="spinner" /> validating…
          </span>
        ) : err ? (
          <span className="bad">validation unavailable: {err}</span>
        ) : res ? (
          res.ok ? (
            <span className="ok"><Icon name="ok" /> valid</span>
          ) : (
            <span className="bad">
              <Icon name="error" /> {res.errors.length} error{res.errors.length === 1 ? '' : 's'}
            </span>
          )
        ) : null}
      </div>
      <CodeEditor value={value} onChange={onChange} lang="yaml" markers={(res?.errors ?? []).map((e) => ({ line: e.line, col: 1, message: e.message }))} minRows={minRows} ariaLabel="braindb.yaml editor" />
      {res && res.errors.length > 0 && (
        <div className="err-box">
          {res.errors.map((e, i) => (
            <div key={i}>
              line {e.line}: {e.message}
            </div>
          ))}
        </div>
      )}
      {showEffective && eff !== null && eff !== undefined && (
        <details className="fold" open>
          <summary>Effective config (secrets masked)</summary>
          <CodeView text={typeof eff === 'string' ? eff : JSON.stringify(eff, null, 2)} lang={typeof eff === 'string' ? 'yaml' : 'json'} maxHeight={300} />
        </details>
      )}
    </div>
  );
}
