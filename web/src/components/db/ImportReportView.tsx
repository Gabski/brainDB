// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { ImportReport } from '../../api/types';
import { CodeView } from '../common/CodeView';
import { Icon, type IconName } from '../common/Icon';

const STATUS_ICON: Record<string, IconName> = { created: 'ok', updated: 'refresh', skipped: 'minus', error: 'error' };
const STATUS_CLS: Record<string, string> = { created: 'ok', updated: 'ok', skipped: 'muted', error: 'bad' };

/** Import report: totals, files grouped by importer, detected SQL tables/columns/FKs, proposed concepts, YAML mapping. */
export function ImportReportView({ report, onUseMapping }: { report: ImportReport; onUseMapping?: (yaml: string) => void }) {
  const groups = new Map<string, typeof report.files>();
  for (const f of report.files) {
    if (!groups.has(f.importer)) groups.set(f.importer, []);
    groups.get(f.importer)!.push(f);
  }
  const t = report.totals;
  const hasDecisions = report.files.some((f) => f.admitted !== undefined || f.quarantined !== undefined || f.rejected !== undefined);
  return (
    <div className="col">
      <div className="stats" style={{ gridTemplateColumns: 'repeat(auto-fit, minmax(110px, 1fr))' }}>
        {(
          [
            ['files', t.files ?? report.files.length],
            ['nodes', t.nodes],
            ['edges', t.edges],
            ['records', t.records],
            ['skipped', t.skipped ?? report.files.filter((f) => f.status === 'skipped').length],
            ['errors', t.errors ?? report.files.filter((f) => f.status === 'error').length],
            ...(hasDecisions
              ? ([
                  ['admitted', t.admitted ?? report.files.reduce((a, f) => a + (f.admitted ?? 0), 0)],
                  ['quarantined', t.quarantined ?? report.files.reduce((a, f) => a + (f.quarantined ?? 0), 0)],
                  ['rejected', t.rejected ?? report.files.reduce((a, f) => a + (f.rejected ?? 0), 0)],
                ] as [string, number][])
              : []),
          ] as [string, number][]
        ).map(([k, v]) => (
          <div key={k} className={'stat' + ((k === 'errors' || k === 'rejected') && v > 0 ? ' alert' : '')}>
            <div className="v">{v.toLocaleString()}</div>
            <div className="k">
              {k}
              {report.dryRun && ['files', 'nodes', 'edges', 'records'].includes(k) ? ' (would create)' : ''}
            </div>
          </div>
        ))}
      </div>
      {[...groups.entries()].map(([imp, files]) => (
        <div key={imp} className="card">
          <div className="card-h">
            {imp} <span className="muted mono">{files.length}</span>
          </div>
          <div className="card-b flush">
            <table className="tbl">
              <thead>
                <tr>
                  <th>path</th>
                  <th>status</th>
                  <th className="num">nodes</th>
                  <th className="num">edges</th>
                  <th className="num">records</th>
                  {hasDecisions && (
                    <>
                      <th className="num" title="admitted">admitted</th>
                      <th className="num" title="quarantined (review band)">review</th>
                      <th className="num" title="rejected by purpose">rejected</th>
                    </>
                  )}
                  <th>message</th>
                </tr>
              </thead>
              <tbody>
                {files.map((f) => (
                  <tr key={f.path}>
                    <td className="mono">{f.path}</td>
                    <td className={STATUS_CLS[f.status] ?? ''}>
                      <Icon name={STATUS_ICON[f.status] ?? 'info'} size={14} /> {f.status}
                    </td>
                    <td className="num">{f.nodes}</td>
                    <td className="num">{f.edges}</td>
                    <td className="num">{f.records}</td>
                    {hasDecisions && (
                      <>
                        <td className="num ok">{f.admitted ?? '–'}</td>
                        <td className={'num' + (f.quarantined ? ' warn' : '')}>{f.quarantined ?? '–'}</td>
                        <td className={'num' + (f.rejected ? ' bad' : '')}>{f.rejected ?? '–'}</td>
                      </>
                    )}
                    <td className={'small ' + (f.status === 'error' ? 'bad' : 'muted')} style={{ whiteSpace: 'normal' }}>
                      {f.message}
                      {f.reasons?.map((r, i) => (
                        <div key={i} className={f.rejected ? 'bad' : 'warn'}>
                          <Icon name={f.rejected ? 'denied' : 'warning'} size={14} /> {r}
                        </div>
                      ))}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      ))}
      {report.tables?.length ? (
        <div className="card">
          <div className="card-h">Detected SQL tables</div>
          <div className="card-b flush">
            <table className="tbl">
              <thead>
                <tr>
                  <th>table</th>
                  <th>columns</th>
                  <th className="num">rows</th>
                  <th>foreign keys</th>
                  <th>concept columns</th>
                </tr>
              </thead>
              <tbody>
                {report.tables.map((tb) => (
                  <tr key={tb.name}>
                    <td className="mono">{tb.name}</td>
                    <td className="mono xs" style={{ whiteSpace: 'normal' }}>
                      {tb.columns.map((c, i) => (<span key={c}>{i > 0 && ', '}{c === tb.primaryKey && <Icon name="key" size={14} label="primary key" />}{c}</span>))}
                    </td>
                    <td className="num">{tb.rows}</td>
                    <td className="mono xs">{tb.foreignKeys?.map((f) => `${f.column} → ${f.references}`).join(', ') || '—'}</td>
                    <td className="xs">{tb.concepts?.join(', ') || '—'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      ) : null}
      {report.concepts?.length ? (
        <div>
          <h3 className="section-h">Proposed concepts</h3>
          <div className="chips">
            {report.concepts.map((c) => (
              <span key={c} className="chip">
                ◆ {c}
              </span>
            ))}
          </div>
        </div>
      ) : null}
      {report.mapping && (
        <div>
          <h3 className="section-h">
            Generated braindb.yaml snippet
            {onUseMapping && (
              <button className="btn sm" onClick={() => onUseMapping(report.mapping)}>
                Use in config
              </button>
            )}
          </h3>
          <CodeView text={report.mapping} lang="yaml" maxHeight={260} />
        </div>
      )}
    </div>
  );
}
