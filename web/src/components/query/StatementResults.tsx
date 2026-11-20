// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { PlanNode, RunResult, StatementResult } from '../../api/types';
import { guessLang } from '../../model/highlight';
import { CodeView } from '../common/CodeView';
import { DataGrid } from '../common/DataGrid';
import { ErrorBox } from '../common/Async';

export interface StatementResultsProps {
  result: RunResult;
  active: number;
  onActive: (i: number) => void;
  onGo?: (id: string) => void;
  onCopied?: (w: string) => void;
}

/** One result tab per statement: rows → grid, text → viewer, changes → list, plan → EXPLAIN tree. */
export function StatementResults({ result, active, onActive, onGo, onCopied }: StatementResultsProps) {
  const res = result.results;
  const cur = res[Math.min(active, res.length - 1)];
  return (
    <div className="pane" style={{ flex: 1 }}>
      {result.error && (
        <div style={{ padding: 8 }}>
          <ErrorBox error={`statement ${result.error.statement}, line ${result.error.line}, col ${result.error.col}: ${result.error.message}${result.ok === false ? '\nNothing was applied.' : ''}`} />
        </div>
      )}
      {res.length > 0 && (
        <div className="result-tabs" role="tablist">
          {res.map((r, i) => (
            <button key={i} role="tab" aria-selected={r === cur} onClick={() => onActive(i)} title={r.statement}>
              {i + 1}. {r.kind === 'rows' ? `▦ ${r.rows?.length ?? 0} rows` : r.kind === 'text' ? '¶ text' : r.kind === 'changes' ? `± ${r.changes?.length ?? 0} changes` : '⎇ plan'} · {r.elapsed_ms} ms
            </button>
          ))}
        </div>
      )}
      {cur && <OneResult r={cur} onGo={onGo} onCopied={onCopied} />}
      {result.status && (
        <div className="grid-foot mono" style={{ fontSize: 11 }}>
          {result.status}
        </div>
      )}
    </div>
  );
}

function OneResult({ r, onGo, onCopied }: { r: StatementResult; onGo?: (id: string) => void; onCopied?: (w: string) => void }) {
  const head = (
    <div className="pane-h mono ellipsis" title={r.statement}>
      {r.statement}
    </div>
  );
  if (r.kind === 'rows')
    return (
      <div className="pane" style={{ flex: 1 }}>
        {head}
        <DataGrid columns={r.columns ?? Object.keys(r.rows?.[0] ?? {})} rows={r.rows ?? []} elapsedMs={r.elapsed_ms} onRowActivate={(row) => row.id && onGo?.(String(row.id))} exportName={`statement-${r.index + 1}`} onCopied={onCopied} />
      </div>
    );
  if (r.kind === 'text')
    return (
      <div className="pane" style={{ flex: 1 }}>
        {head}
        <div className="tab-scroll">
          <CodeView text={r.text ?? ''} lang={guessLang(undefined, r.text ?? '')} />
        </div>
      </div>
    );
  if (r.kind === 'changes') return <ChangeList r={r} onGo={onGo} head={head} />;
  return (
    <div className="pane" style={{ flex: 1 }}>
      {head}
      <div className="tab-scroll">{r.plan ? <PlanTree plan={r.plan} /> : <CodeView text={r.text ?? JSON.stringify(r.rows ?? [], null, 2)} lang="text" />}</div>
    </div>
  );
}

const ID_RE = /([A-Za-z]+:[\w.-]+|\b[a-z]\d{2,}\b)/g;

/** Write results: one line per change with clickable ids. */
export function ChangeList({ r, onGo, head }: { r: StatementResult; onGo?: (id: string) => void; head?: React.ReactNode }) {
  const known = new Set(r.ids ?? []);
  return (
    <div className="pane" style={{ flex: 1 }}>
      {head}
      <div className="tab-scroll">
        <ul className="list" style={{ listStyle: 'none', margin: 0, padding: 0 }}>
          {(r.changes ?? []).map((c, i) => (
            <li key={i} className="mono small" style={{ padding: '3px 0', borderBottom: '1px solid var(--line)' }}>
              {c.split(ID_RE).map((part, j) =>
                known.has(part) || (j % 2 === 1 && onGo) ? (
                  <button key={j} className="link mono" onClick={() => onGo?.(part)}>
                    {part}
                  </button>
                ) : (
                  <span key={j}>{part}</span>
                ),
              )}
            </li>
          ))}
        </ul>
        {r.ids?.length ? (
          <div className="xs muted" style={{ marginTop: 8 }}>
            touched: {r.ids.join(', ')}
          </div>
        ) : null}
      </div>
    </div>
  );
}

/** EXPLAIN plan tree. */
export function PlanTree({ plan }: { plan: PlanNode }) {
  const Node = ({ p }: { p: PlanNode }) => (
    <li>
      <span className="op">{p.op}</span> {p.detail && <span>{p.detail}</span>}{' '}
      <span className="muted xs">
        {p.rows !== undefined && `~${p.rows} rows`}
        {p.index && ` · index ${p.index}`}
        {p.cache && ` · cache ${p.cache}`}
      </span>
      {p.children?.length ? (
        <ul>
          {p.children.map((c, i) => (
            <Node key={i} p={c} />
          ))}
        </ul>
      ) : null}
    </li>
  );
  return (
    <div className="plan small">
      <ul>
        <Node p={plan} />
      </ul>
    </div>
  );
}
