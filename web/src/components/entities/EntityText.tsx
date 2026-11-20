// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { Entity } from '../../api/types';
import { hash } from '../../model/util';
import { Icon } from '../common/Icon';

const FIXED: Record<string, number> = { date: 200, datetime: 210, duration: 230, money: 140, percent: 120, number_unit: 160, email: 280, phone: 310, url: 250, domain: 260, person: 30, org: 0, iban: 340, vat: 350, card: 355, ip: 190, version: 180, id_code: 50, hashtag: 90, mention: 100, geo: 170, address: 20 };
export const entityHue = (type: string) => FIXED[type] ?? hash(type) % 360;

/** Item text with entities highlighted inline; hover shows type + normalised value, masked PII shows a lock icon. */
export function EntityText({ text, entities, className }: { text: string; entities?: Entity[]; className?: string }) {
  const list = [...(entities ?? [])].filter((e) => e.start >= 0 && e.end <= text.length && e.end > e.start).sort((a, b) => a.start - b.start);
  const parts: React.ReactNode[] = [];
  let pos = 0;
  list.forEach((e, i) => {
    if (e.start < pos) return; // overlapping: keep the first
    if (e.start > pos) parts.push(text.slice(pos, e.start));
    const h = entityHue(e.type);
    parts.push(
      <mark
        key={i}
        className="ent"
        style={{ ['--h' as string]: String(h) }}
        title={`${e.type}: ${e.value}${e.pii ? ' · PII' : ''}${e.masked ? ' (masked)' : ''} · confidence ${e.confidence.toFixed(2)}`}
        tabIndex={0}
      >
        {e.masked && <Icon name="lock" size={14} label="masked" />}
        {e.masked ? e.value : text.slice(e.start, e.end)}
        <span className="ent-type">{e.type}</span>
      </mark>,
    );
    pos = e.end;
  });
  if (pos < text.length) parts.push(text.slice(pos));
  return <span className={className}>{parts}</span>;
}

/** Entities table (Inspector). */
export function EntitiesTable({ entities, onValue }: { entities: Entity[]; onValue?: (type: string, value: string) => void }) {
  if (!entities.length) return <div className="empty">no entities recognised</div>;
  return (
    <table className="tbl">
      <thead>
        <tr>
          <th>type</th>
          <th>value</th>
          <th>raw</th>
          <th className="num">conf.</th>
        </tr>
      </thead>
      <tbody>
        {entities.map((e, i) => (
          <tr key={i}>
            <td>
              <span className="ent" style={{ ['--h' as string]: String(entityHue(e.type)) }}>
                {e.type}
              </span>
            </td>
            <td className="mono">
              {e.masked && <Icon name="lock" size={14} label="masked" />} 
              {onValue ? (
                <button className="link mono" onClick={() => onValue(e.type, e.value)}>
                  {e.value}
                </button>
              ) : (
                e.value
              )}
              {e.pii && !e.masked && <span className="muted xs"> PII</span>}
            </td>
            <td className="mono muted ellipsis" style={{ maxWidth: 110 }}>
              {e.masked ? '••••' : e.raw}
            </td>
            <td className="num">{e.confidence.toFixed(2)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
