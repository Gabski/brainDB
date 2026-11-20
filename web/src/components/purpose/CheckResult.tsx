// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { PurposeCheck } from '../../api/types';
import { Icon, type IconName } from '../common/Icon';

const norm = (d: string) => (/^(accept|admitted)/.test(d) ? 'accept' : /^(review|quarantine)/.test(d) ? 'review' : 'reject');
const DEC: Record<string, { icon: IconName; cls: string; label: string }> = {
  accept: { icon: 'ok', cls: 'ok', label: 'accept → admitted' },
  review: { icon: 'warning', cls: 'warn', label: 'review → quarantine' },
  reject: { icon: 'denied', cls: 'bad', label: 'reject → not stored' },
};

/** Threshold band: reject | review | accept with an optional score marker. */
export function ThresholdBand({ accept, review, score }: { accept: number; review: number; score?: number }) {
  return (
    <div className="band" aria-label={`reject below ${review}, review ${review}–${accept}, accept from ${accept}`}>
      <div className="band-seg reject" style={{ width: `${review * 100}%` }}>reject</div>
      <div className="band-seg review" style={{ width: `${(accept - review) * 100}%` }}>review</div>
      <div className="band-seg accept" style={{ width: `${(1 - accept) * 100}%` }}>accept</div>
      {score !== undefined && <div className="band-mark" style={{ left: `${Math.max(0, Math.min(1, score)) * 100}%` }} title={`score ${score.toFixed(2)}`} />}
    </div>
  );
}

/** Score gauge + pos/neg/bonus contribution bars + top match/conflict + rules. */
export function CheckResult({ r, accept, review }: { r: PurposeCheck; accept: number; review: number }) {
  const d = DEC[norm(r.decision)];
  const Bar = ({ label, v, color, sign }: { label: string; v: number; color: string; sign: string }) => (
    <div className="row nowrap small">
      <span style={{ width: 52 }} className="muted">
        {label}
      </span>
      <div className="bar grow">
        <div style={{ width: `${Math.min(1, Math.abs(v)) * 100}%`, background: color }} />
      </div>
      <span className="mono xs" style={{ width: 48, textAlign: 'right' }}>
        {sign}
        {v.toFixed(2)}
      </span>
    </div>
  );
  return (
    <div className="col">
      <div className="row">
        <span className="mono" style={{ fontSize: 22, fontWeight: 600 }}>
          {r.score.toFixed(2)}
        </span>
        <span className={'row nowrap ' + d.cls} style={{ fontWeight: 600 }}>
          <Icon name={d.icon} /> {d.label}
        </span>
      </div>
      <ThresholdBand accept={accept} review={review} score={r.score} />
      <Bar label="pos" v={r.pos} color="var(--ok)" sign="+" />
      <Bar label="neg" v={r.neg} color="var(--bad)" sign="−" />
      <Bar label="bonus" v={r.bonus} color="var(--accent)" sign="+" />
      <table className="props">
        <tbody>
          <tr>
            <th>top match</th>
            <td className="plain">{r.topMatch || '—'}</td>
          </tr>
          <tr>
            <th>top conflict</th>
            <td className="plain">{r.topConflict || '—'}</td>
          </tr>
        </tbody>
      </table>
      {r.rules.length > 0 && (
        <ul className="small" style={{ margin: 0, paddingLeft: 18 }}>
          {r.rules.map((x, i) => (
            <li key={i}>{x}</li>
          ))}
        </ul>
      )}
    </div>
  );
}
