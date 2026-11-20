// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useState } from 'react';
import type { Connector } from '../../api/types';
import type { GraphIndex } from '../../model/graph';
import { NodeRow } from '../common/NodeRow';
import { SectionH } from '../common/Async';
import { Icon } from '../common/Icon';

export interface ConnectorInspectorProps {
  g: GraphIndex;
  connector: Connector;
  /** knowledge nodes joined by the connector */
  members: string[];
  onGo: (id: string) => void;
  /** runs a BDQL statement */
  onRun: (bdql: string, label: string) => void;
}

const q = (s: string) => '"' + s.replace(/\\/g, '\\\\').replace(/"/g, '\\"') + '"';

/** Inspector for a ⧉ connector: type, key, gain, transparency, degree, sources spanned, routes used; BDQL actions. */
export function ConnectorInspector({ g, connector: c, members, onGo, onRun }: ConnectorInspectorProps) {
  const [target, setTarget] = useState('');
  const sources = [...new Set(members.map((m) => g.nodes.get(m)?.source ?? 'local'))];
  const routed = g.state.nodes.filter((n) => n.route?.includes(c.id));
  return (
    <div>
      <div className="insp-head">
        <div className="row">
          <span className="kind-badge" style={{ background: 'transparent', color: 'var(--fg)', border: '1px dashed var(--line-strong)' }}>
            ⧉ {c.type}
          </span>
          <b className="mono">{c.id}</b>
        </div>
        <div className="itext">{c.label}</div>
      </div>
      <div className="insp-body">
        <table className="props">
          <tbody>
            <tr><th>type</th><td>{c.type}</td></tr>
            {c.key && <tr><th>key</th><td>{c.key}</td></tr>}
            <tr><th>gain</th><td>{c.gain.toFixed(2)}</td></tr>
            <tr><th>transparent</th><td>{c.transparent ? 'yes (does not consume depth)' : 'no (consumes a depth level)'}</td></tr>
            <tr><th>degree</th><td>{c.degree}</td></tr>
            {c.terms?.length ? <tr><th>terms</th><td>{c.terms.join(', ')}</td></tr> : null}
            {c.source && <tr><th>source</th><td>{c.source}</td></tr>}
            <tr><th>spans sources</th><td>{sources.join(', ')}</td></tr>
            <tr><th>routes used</th><td>{routed.length} lit item{routed.length === 1 ? '' : 's'} in the current context</td></tr>
          </tbody>
        </table>
        <div className="xs muted" style={{ marginTop: 6 }}>
          Connectors carry no knowledge: they are never lit or counted in tokens, they only route activation and search.
        </div>
        <div className="actions">
          <div className="row nowrap" style={{ gridColumn: '1 / -1' }}>
            <input className="input grow" aria-label="Node id to connect" placeholder="node id, e.g. f103" value={target} onChange={(e) => setTarget(e.target.value)} />
            <button className="btn" disabled={!target.trim()} onClick={() => onRun(`CONNECT ${target.trim()} TO ${c.id};`, 'connect')}>
              Connect
            </button>
            <button className="btn" disabled={!target.trim()} onClick={() => onRun(`DISCONNECT ${target.trim()} FROM ${c.id};`, 'disconnect')}>
              Disconnect
            </button>
          </div>
          {(c.type === 'hub' || c.type === 'anchor') && (
            <button
              className="btn"
              onClick={() => onRun(`UPDATE connectors SET type = ${c.type === 'hub' ? 'anchor' : 'hub'} WHERE id = ${c.id};`, 'convert connector')}
            >
              <Icon name="supersede" /> Convert to {c.type === 'hub' ? 'anchor' : 'hub'}
            </button>
          )}
          <button className="btn danger" onClick={() => onRun(`FORGET ${c.id};`, 'dissolve connector')} title={`Dissolve ${q(c.label)}: removes the connector, keeps the knowledge`}>
            <Icon name="unlink" /> Dissolve
          </button>
        </div>
        <SectionH count={members.length}>Joins</SectionH>
        <div className="list">
          {members.map((m) => (
            <NodeRow key={m} g={g} id={m} onGo={onGo} showId right={g.nodes.get(m)?.source ?? ''} />
          ))}
        </div>
        {routed.length > 0 && (
          <>
            <SectionH count={routed.length}>Lit through this connector</SectionH>
            <div className="list">
              {routed.map((n) => (
                <NodeRow key={n.id} g={g} id={n.id} onGo={onGo} right={n.activation?.toFixed(2)} />
              ))}
            </div>
          </>
        )}
      </div>
    </div>
  );
}
