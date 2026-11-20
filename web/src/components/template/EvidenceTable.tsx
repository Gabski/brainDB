// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { SlotValue } from '../../api/types';
import { Icon } from '../common/Icon';

/** Evidence per slot: path, value, source (clickable), score, snippet. Missing slots are highlighted. */
export function EvidenceTable({ values, missing, onSource }: { values: SlotValue[]; missing: string[]; onSource?: (id: string) => void }) {
  const miss = new Set(missing);
  return (
    <table className="tbl">
      <thead>
        <tr>
          <th>path</th>
          <th>type</th>
          <th>value</th>
          <th>source</th>
          <th className="num">score</th>
          <th>snippet</th>
        </tr>
      </thead>
      <tbody>
        {values.map((v) => {
          const isMissing = miss.has(v.path) || v.value === null;
          return (
            <tr key={v.path} className={isMissing ? 'slot-missing' : ''} title={v.description}>
              <td className="mono">
                {isMissing && <span className="warn"><Icon name="warning" size={14} label="missing" /> </span>}
                {v.path}
              </td>
              <td className="muted xs">{v.type}</td>
              <td className="mono" style={{ whiteSpace: 'normal' }}>
                {v.value ?? <span className="muted">null — missing</span>}
              </td>
              <td>
                {v.source ? (
                  <button className="link mono" onClick={() => onSource?.(v.source!)}>
                    {v.source}
                  </button>
                ) : (
                  <span className="muted">—</span>
                )}
              </td>
              <td className="num">{v.score.toFixed(2)}</td>
              <td className="muted ellipsis" style={{ maxWidth: 260 }} title={v.snippet}>
                {v.snippet}
              </td>
            </tr>
          );
        })}
        {missing
          .filter((m) => !values.some((v) => v.path === m))
          .map((m) => (
            <tr key={m} className="slot-missing">
              <td className="mono"><Icon name="warning" size={14} label="missing" /> {m}</td>
              <td colSpan={5} className="muted">
                missing
              </td>
            </tr>
          ))}
      </tbody>
    </table>
  );
}
