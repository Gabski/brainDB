// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useState } from 'react';
import type { ConnectorSuggestion } from '../../api/types';
import type { GraphIndex } from '../../model/graph';
import { NodeRow } from '../common/NodeRow';
import { Icon } from '../common/Icon';

const q = (s: string) => '"' + s.replace(/\\/g, '\\\\').replace(/"/g, '\\"') + '"';
const sq = (s: string) => "'" + s.replace(/'/g, "''") + "'";

/** BDQL that creates a suggested connector. */
export function connectorBdql(s: ConnectorSuggestion): string {
  return `CONNECTOR ${s.type} ${q(s.label)}${s.key ? ` KEY ${sq(s.key)}` : ''}${s.terms?.length ? ` TERMS ${s.terms.map(sq).join(', ')}` : ''}${s.members.length ? ` CONNECT ${s.members.join(', ')}` : ''};`;
}

/** "Suggested connectors" with evidence and one-click create (via BDQL). */
export function ConnectorSuggestions({ g, list, onCreate, onGo }: { g: GraphIndex; list: ConnectorSuggestion[]; onCreate: (bdql: string) => void; onGo: (id: string) => void }) {
  const [done, setDone] = useState<Set<number>>(new Set());
  return (
    <div className="tool">
      <h4><Icon name="connector" /> Suggested connectors</h4>
      <div className="xs muted">Structural junctions that would join knowledge from different places. They add no facts, only routes.</div>
      {!list.length && <div className="muted small">no suggestions</div>}
      {list.map((s, i) => (
        <div key={i} style={{ borderTop: '1px solid var(--line)', marginTop: 6, paddingTop: 6 }}>
          <div className="row">
            <span className="kind-badge" style={{ background: 'transparent', color: 'var(--fg)', border: '1px dashed var(--line-strong)' }}>
              ⧉ {s.type}
            </span>
            <b>{s.label}</b>
            {s.key && <span className="mono xs muted">{s.key}</span>}
            <span className="spacer" />
            <span className="mono xs">{s.score.toFixed(2)}</span>
            <button
              className="btn sm"
              disabled={done.has(i)}
              onClick={() => {
                onCreate(connectorBdql(s));
                setDone(new Set([...done, i]));
              }}
            >
              {done.has(i) ? <><Icon name="check" /> created</> : <><Icon name="add" /> Create</>}
            </button>
          </div>
          <div className="xs muted">{s.evidence}</div>
          {s.terms?.length ? <div className="xs">terms: {s.terms.join(', ')}</div> : null}
          <div className="list">
            {s.members.map((m) => (
              <NodeRow key={m} g={g} id={m} onGo={onGo} showId />
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}
