// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useEffect, useMemo, useState } from 'react';
import { useStore, useStoreApi } from '../../state/StoreProvider';
import type { QueryFormat } from '../../api/types';
import { CodeEditor } from '../common/CodeEditor';
import { CodeView } from '../common/CodeView';
import { DataGrid } from '../common/DataGrid';
import { Empty, ErrorBox, Loading } from '../common/Async';
import { StatementResults } from './StatementResults';
import { bdqlCompletions, BDQL_SNIPPETS, compactCompletions } from '../../model/completions';
import { Modal } from '../common/Modal';
import { downloadText } from '../../model/util';
import { Icon } from '../common/Icon';

/** Query workspace tab: BDQL-first editor (+ compact/JSON raw query mode) and results. */
export function QueryTab({ initialCompletion }: { initialCompletion?: { prefix: string } }) {
  const store = useStoreApi();
  const bdql = useStore((s) => s.bdql);
  const q = useStore((s) => s.query);
  const g = useStore((s) => s.index);
  const templates = useStore((s) => s.templates.data);
  const sources = useStore((s) => s.sources.data);
  const [snips, setSnips] = useState(false);
  const [view, setView] = useState<'grid' | 'rendered'>('grid');
  const isBdql = bdql.mode === 'bdql';
  const completions = useMemo(() => (isBdql ? bdqlCompletions(g, templates ?? [], bdql.script) : compactCompletions(g)), [isBdql, g, templates, bdql.script]);

  useEffect(() => {
    if (isBdql) store.lintBdql(bdql.script);
  }, [bdql.script, isBdql, store]);

  const go = (id: string) => store.select(id, true);
  const copied = (w: string) => store.toastMsg(`Copied ${w}`);
  const save = () => {
    const name = window.prompt('Save query as', '');
    if (name) store.saveQuery(name, isBdql ? bdql.script : q.text);
  };

  return (
    <div className="split-v">
      <div className="toolbar">
        <div className="seg" aria-label="Query language">
          <button aria-pressed={isBdql} onClick={() => store.setBdql({ mode: 'bdql' })}>
            BDQL
          </button>
          <button aria-pressed={!isBdql} onClick={() => store.setBdql({ mode: 'compact' })}>
            Compact / JSON
          </button>
        </div>
        <span className="sep" />
        {isBdql ? (
          <>
            <button className="btn primary" onClick={() => store.runBdqlEditor(cursorOf())} title="Run statement at cursor (Ctrl+Enter)">
              <Icon name="play" /> Run <kbd>Ctrl+↵</kbd>
            </button>
            <button className="btn" onClick={() => store.runBdqlEditor()} title="Run all (Ctrl+Shift+Enter)">
              <Icon name="playAll" /> Run all
            </button>
            <label className="check small">
              <input type="checkbox" checked={bdql.dryRun} onChange={(e) => store.setBdql({ dryRun: e.target.checked })} /> dry run
            </label>
          </>
        ) : (
          <button className="btn primary" onClick={() => store.runQuery()}>
            <Icon name="play" /> Run <kbd>Ctrl+↵</kbd>
          </button>
        )}
        <label className="row small">
          format
          <select className="input" value={q.format} onChange={(e) => store.setQuery({ format: e.target.value as QueryFormat })}>
            {['md', 'json', 'yaml', 'xml', 'csv'].map((f) => (
              <option key={f}>{f}</option>
            ))}
          </select>
        </label>
        {!isBdql && sources && sources.length > 0 && (
          <label className="row small">
            sources
            <select
              className="input"
              value={q.sources.join(',')}
              onChange={(e) => store.setQuery({ sources: e.target.value ? e.target.value.split(',') : [] })}
            >
              <option value="">local only</option>
              <option value="*">all (*)</option>
              {sources.map((s) => (
                <option key={s.name} value={s.name}>
                  {s.name}
                </option>
              ))}
            </select>
          </label>
        )}
        <span className="spacer" />
        {isBdql && (
          <button className="btn" onClick={() => setSnips(true)}>
            <Icon name="snippet" /> Snippets
          </button>
        )}
        {isBdql && (
          <>
            <button className="btn" title="Download the script as .bdql" onClick={() => downloadText('query.bdql', bdql.script, 'text/plain')}>
              <Icon name="download" /> .bdql
            </button>
            <label className="btn" title="Open a .bdql script">
              <Icon name="upload" /> Open
              <input
                type="file"
                accept=".bdql,.sql,.txt"
                hidden
                onChange={async (e) => {
                  const f = e.target.files?.[0];
                  if (f) store.setBdql({ script: await f.text() });
                  e.target.value = '';
                }}
              />
            </label>
          </>
        )}
        <button className="btn" onClick={save}>
          <Icon name="save" /> Save
        </button>
      </div>
      <div style={{ padding: 8, borderBottom: '1px solid var(--line)', flex: 'none', maxHeight: '45%', overflow: 'auto' }}>
        {isBdql ? (
          <>
            <CodeEditor
              value={bdql.script}
              onChange={(v) => store.setBdql({ script: v })}
              lang="bdql"
              completions={completions}
              markers={bdql.lint.map((e) => ({ line: e.line, col: e.col, message: e.message }))}
              onRun={(c) => store.runBdqlEditor(c)}
              onRunAll={() => store.runBdqlEditor()}
              minRows={8}
              ariaLabel="BDQL editor"
              initialCompletion={initialCompletion}
            />
            {bdql.lint.length > 0 && (
              <div className="xs bad" style={{ marginTop: 4 }} role="status">
                {bdql.lint.map((e, i) => (
                  <div key={i}>
                    ● statement {e.statement}, {e.line}:{e.col} — {e.message}
                  </div>
                ))}
              </div>
            )}
          </>
        ) : (
          <>
            <CodeEditor
              value={q.text}
              onChange={(v) => store.setQuery({ text: v })}
              lang="compact"
              completions={completions}
              onRun={() => store.runQuery()}
              onRunAll={() => store.runQuery()}
              minRows={3}
              ariaLabel="Compact or JSON query"
            />
            <div className="xs muted" style={{ marginTop: 4 }}>
              Compact: <code>kind:record users&gt;100 owner~kim "free text" near:"risk" sort:-users limit:10 select:id,text,fields.users</code> — or a JSON object.
            </div>
          </>
        )}
      </div>
      {isBdql ? (
        bdql.loading ? (
          <Loading text="Running…" />
        ) : bdql.error ? (
          <div style={{ padding: 10 }}>
            <ErrorBox error={bdql.error} />
          </div>
        ) : bdql.result ? (
          <StatementResults result={bdql.result} active={bdql.active} onActive={(i) => store.setBdql({ active: i })} onGo={go} onCopied={copied} />
        ) : (
          <Empty>Run a statement to see results. One tab per statement.</Empty>
        )
      ) : q.loading ? (
        <Loading text="Running…" />
      ) : q.error ? (
        <div style={{ padding: 10 }}>
          <ErrorBox error={q.error} />
        </div>
      ) : q.result ? (
        <div className="pane" style={{ flex: 1 }}>
          <div className="pane-h">
            <div className="seg">
              <button aria-pressed={view === 'grid'} onClick={() => setView('grid')}>
                Grid
              </button>
              <button aria-pressed={view === 'rendered'} onClick={() => setView('rendered')}>
                Rendered ({q.format})
              </button>
            </div>
            <span className="spacer" />
            {q.result.count} of {q.result.total}
          </div>
          {view === 'grid' ? (
            <DataGrid columns={q.result.columns} rows={q.result.rows} total={q.result.total} elapsedMs={q.ms} onRowActivate={(r) => r.id && go(String(r.id))} onCopied={copied} />
          ) : (
            <div className="tab-scroll">
              <CodeView text={q.result.rendered} lang={q.format === 'csv' ? 'text' : q.format} />
            </div>
          )}
        </div>
      ) : (
        <Empty>Run the query to see results.</Empty>
      )}
      {snips && (
        <Modal title="BDQL snippets" onClose={() => setSnips(false)} width={720}>
          <SnippetLibrary
            onPick={(code) => {
              store.setBdql({ script: bdql.script.trimEnd() + '\n\n' + code + '\n' });
              setSnips(false);
            }}
          />
        </Modal>
      )}
    </div>
  );
}

function cursorOf(): number | undefined {
  const el = document.querySelector<HTMLTextAreaElement>('textarea[aria-label="BDQL editor"]');
  return el ? el.selectionStart : undefined;
}

/** Snippet library grouped by statement family. */
export function SnippetLibrary({ onPick }: { onPick: (code: string) => void }) {
  const groups = [...new Set(BDQL_SNIPPETS.map((s) => s.group))];
  return (
    <div className="col">
      {groups.map((gname) => (
        <div key={gname}>
          <h3 className="section-h">{gname}</h3>
          {BDQL_SNIPPETS.filter((s) => s.group === gname).map((s) => (
            <div key={s.name} style={{ marginBottom: 8 }}>
              <div className="row">
                <b className="small">{s.name}</b>
                <span className="spacer" />
                <button className="btn sm" onClick={() => onPick(s.code)}>
                  Insert
                </button>
              </div>
              <CodeView text={s.code} lang="bdql" />
            </div>
          ))}
        </div>
      ))}
    </div>
  );
}
