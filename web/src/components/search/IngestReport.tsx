// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { IngestResult } from '../../api/types';
import type { GraphIndex } from '../../model/graph';
import { short } from '../../model/util';
import { Icon, type IconName } from '../common/Icon';

/** Report lines are data from the API: strip any pictographs and pick an icon from keywords. */
export function reportLine(line: string): { icon: IconName; text: string; bad: boolean } {
  const text = line.replace(/[\p{Extended_Pictographic}\uFE0F]/gu, '').replace(/^\s+/, '');
  const l = text.toLowerCase();
  const bad = /^(warning|error)\b|\b(error|failed|cannot|not installed|without text)\b/.test(l) || /^\u26a0/.test(line);
  const icon: IconName = bad ? 'warning' : /\brecords?\b/.test(l) ? 'table' : /\b(image|png|jpe?g|webp)\b/.test(l) ? 'image' : /\bbrain\b/.test(l) ? 'brain' : /elements|items/.test(l) ? 'database' : 'file';
  return { icon, text: text.replace(/^(warning|error):\s*/i, ''), bad };
}

/** Result of a file drop: per-file lines + embedding auto-links. */
export function IngestReport({ g, result, onGo }: { g: GraphIndex; result: IngestResult; onGo: (id: string) => void }) {
  return (
    <div className="tool">
      <h4><Icon name="inbox" /> Ingested into the brain</h4>
      <div className="small">
        {result.report.map((r, i) => {
          const { icon, text, bad } = reportLine(r);
          return (
            <div key={i} className={'row nowrap' + (bad ? ' warn' : '')} style={{ alignItems: 'flex-start' }}>
              <Icon name={icon} size={14} /> <span>{text}</span>
            </div>
          );
        })}
      </div>
      {result.links.length > 0 && (
        <>
          <div className="xs muted" style={{ marginTop: 6 }}>
            Auto-links from embeddings (◆ concept, ~ similarity):
          </div>
          <div className="list" style={{ marginTop: 4 }}>
            {result.links.slice(0, 24).map((l) => (
              <div key={l.a + l.b} className="li" style={{ cursor: 'default' }}>
                <i className="dot" style={{ background: g.color(l.a) }} />
                <span className="t">
                  <button className="link" onClick={() => onGo(l.a)}>
                    {short(g.nodes.get(l.a)?.text ?? l.a, 22)}
                  </button>{' '}
                  {l.type === 'concept' ? '◆' : '~'}{' '}
                  <button className="link" onClick={() => onGo(l.b)}>
                    {short(g.nodes.get(l.b)?.text ?? l.b, 22)}
                  </button>
                </span>
                <span className="n">{l.cos.toFixed(2)}</span>
              </div>
            ))}
          </div>
        </>
      )}
      <div className="xs muted mono" style={{ marginTop: 6 }}>
        {result.status}
      </div>
    </div>
  );
}
