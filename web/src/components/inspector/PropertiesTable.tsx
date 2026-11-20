// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { BrainNode, InspectItem, Kind } from '../../api/types';
import type { GraphIndex } from '../../model/graph';
import { isConceptNode } from '../../model/graph';
import { KindBadge } from '../common/KindBadge';
import { Icon } from '../common/Icon';

export interface PropertiesTableProps {
  g: GraphIndex;
  node: BrainNode;
  kind: Kind;
  item?: InspectItem;
  onGo: (id: string) => void;
}

const Id = ({ id, onGo }: { id: string; onGo: (id: string) => void }) => (
  <button className="link mono" onClick={() => onGo(id)}>
    {id}
  </button>
);

/** Properties of a node incl. the weight breakdown, activation, path from the focus and routes. */
export function PropertiesTable({ g, node: n, kind, item, onGo }: PropertiesTableProps) {
  const concept = isConceptNode(n);
  const decay = item?.decay ?? (concept || n.pinned || !kind.halfLife ? 1 : Math.pow(0.5, n.age / kind.halfLife));
  const lit = n.activation !== undefined && n.activation !== null;
  const chain = item?.chain ?? [];
  const parts = [...(g.adj.get(n.id) ?? [])].filter((e) => e.type === 'part' && e.a === n.id).length;
  return (
    <table className="props">
      <tbody>
        <tr>
          <th>id</th>
          <td>{n.id}</td>
        </tr>
        <tr>
          <th>kind</th>
          <td>
            <KindBadge label={kind.label} color={kind.color} />
          </td>
        </tr>
        <tr>
          <th>section</th>
          <td>{n.section}</td>
        </tr>
        {n.source && (
          <tr>
            <th>source</th>
            <td>
              {n.source}
              {n.external ? ' (external stub)' : ''}
            </td>
          </tr>
        )}
        <tr>
          <th>weight w</th>
          <td>
            <div className="row nowrap">
              {n.weight.toFixed(2)}
              <div className="bar grow">
                <div style={{ width: `${n.weight * 100}%` }} />
              </div>
            </div>
          </td>
        </tr>
        <tr>
          <th>base</th>
          <td>{n.base.toFixed(2)}</td>
        </tr>
        <tr>
          <th>decay</th>
          <td>
            {concept ? (
              <span className="plain">none (concept)</span>
            ) : n.pinned ? (
              <span className="plain">none (pinned <Icon name="pin" size={14} />)</span>
            ) : (
              <>
                {decay.toFixed(3)} <span className="plain muted">(age {n.age} d, half-life {kind.halfLife ?? '∞'} d)</span>
              </>
            )}
          </td>
        </tr>
        {n.supersededBy && (
          <tr>
            <th>superseded by</th>
            <td>
              <Id id={n.supersededBy} onGo={onGo} /> <span className="plain muted">(× 0.3, faded)</span>
            </td>
          </tr>
        )}
        <tr>
          <th>in context</th>
          <td>{lit ? <>yes · act {n.activation!.toFixed(2)} · depth {n.depth}</> : <span className="plain muted">no (faded)</span>}</td>
        </tr>
        {n.via && (
          <tr>
            <th>via</th>
            <td>
              <Id id={n.via} onGo={onGo} />
            </td>
          </tr>
        )}
        {chain.length > 1 && (
          <tr>
            <th>path from focus</th>
            <td>
              {chain.map((c, i) => (
                <span key={c}>
                  {i > 0 && ' → '}
                  <Id id={c} onGo={onGo} />
                </span>
              ))}
            </td>
          </tr>
        )}
        {n.route?.length ? (
          <tr>
            <th>route</th>
            <td>
              <RouteChips g={g} route={n.route} onGo={onGo} />
            </td>
          </tr>
        ) : null}
        {n.file && (
          <tr>
            <th>file</th>
            <td>
              {n.file.name} · {n.file.mime} · {(n.file.size / 1024).toFixed(1)} KB{parts ? ` · ${parts} chunks` : ''}
            </td>
          </tr>
        )}
        {n.parent && (
          <tr>
            <th>part of</th>
            <td>
              #{(n.index ?? 0) + 1} of <Id id={n.parent} onGo={onGo} />
            </td>
          </tr>
        )}
      </tbody>
    </table>
  );
}

/** Connector route chips (⧉ label) for an item reached through connectors. */
export function RouteChips({ g, route, onGo }: { g: GraphIndex; route: string[]; onGo: (id: string) => void }) {
  const conns = new Map((g.state.connectors ?? []).map((c) => [c.id, c]));
  return (
    <span className="chips" style={{ display: 'inline-flex' }}>
      {route.map((r) => (
        <button key={r} className="chip" style={{ height: 20 }} onClick={() => onGo(r)} title={r}>
          ⧉ {conns.get(r)?.label ?? r}
        </button>
      ))}
    </span>
  );
}

/** Record fields as a key/value table. */
export function FieldsTable({ fields }: { fields: Record<string, string> }) {
  const keys = Object.keys(fields);
  if (!keys.length) return null;
  return (
    <table className="props">
      <tbody>
        {keys.map((k) => (
          <tr key={k}>
            <th className="mono" style={{ fontFamily: 'var(--mono)', fontSize: 11.5 }}>
              {k}
            </th>
            <td>{fields[k]}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
