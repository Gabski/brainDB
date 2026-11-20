// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { InspectPath, InspectTest } from '../../api/types';
import { depthColor, EDGE_GLYPH, edgeKey, type GraphIndex } from '../../model/graph';
import { short } from '../../model/util';
import { NodeRow } from '../common/NodeRow';
import { SuggestionList } from '../search/SuggestionList';
import type { ToolOutput as Tool } from '../../state/store';
import { Icon } from '../common/Icon';

export interface ToolOutputProps {
  g: GraphIndex;
  tool: Tool;
  depth: number;
  threshold: number;
  onGo: (id: string) => void;
  onReplay?: (id: string) => void;
  onEdgeAdjust?: (a: string, b: string, d: number) => void;
  onEdgeRemove?: (a: string, b: string) => void;
  onShowPath?: (path: string[]) => void;
  onFocusBoth?: (ids: string[]) => void;
  onPreview?: (u: string, v: string) => void;
  onAddLink?: (u: string, v: string, w: number) => void;
}

/** Output of test links / suggest / paths / messages for the selected node. */
export function ToolOutput(p: ToolOutputProps) {
  const { g, tool } = p;
  if (tool.type === 'message')
    return (
      <div className="tool">
        <h4>{tool.ok === false ? <Icon name={/denied/i.test(tool.title) ? 'denied' : 'warning'} /> : <Icon name="ok" />}{tool.title}</h4>
        <div className="small" style={{ whiteSpace: 'pre-wrap' }}>
          {tool.text}
        </div>
      </div>
    );
  if (tool.type === 'suggest')
    return <SuggestionList g={g} title={`Proposed new links — ${tool.id}`} list={tool.data} onGo={p.onGo} onPreview={p.onPreview} onAdd={p.onAddLink} />;
  if (tool.type === 'paths') return <PathsOutput {...p} from={tool.from} to={tool.to} paths={tool.data} />;
  return <TestOutput {...p} id={tool.id} test={tool.data} />;
}

function TestOutput({ g, id, test, depth, threshold, onGo, onReplay, onEdgeAdjust, onEdgeRemove }: ToolOutputProps & { id: string; test: InspectTest }) {
  const mx = Math.max(0.01, ...test.ablation.map((r) => Math.abs(r.dSum)));
  const lit = g.nodes.get(id)?.activation !== undefined;
  return (
    <div className="tool">
      <h4><Icon name="test" /> Link test — {id}</h4>
      <div className="xs muted">
        1) Propagation from this memory alone (depth {depth}, threshold {threshold.toFixed(2)}) lights <b>{test.reached.length}</b> nodes. Pulses show the signal path.
      </div>
      <div className="list" style={{ margin: '6px 0' }}>
        {test.reached.slice(0, 8).map((r) => (
          <NodeRow key={r.id} g={g} id={r.id} onGo={onGo} right={<span style={{ color: depthColor(r.depth) }}>d{r.depth} · {r.act.toFixed(2)}</span>} />
        ))}
        {!test.reached.length && <span className="muted small">the signal dies at once — raise the depth or lower the threshold</span>}
      </div>
      <div className="xs muted">
        2) Ablation: how the current context changes when each synapse is removed (Δ lit / Δ activation sum).
        {!lit && <b> This memory is outside the context, so its synapses do not affect the slice.</b>}
      </div>
      <div className="list" style={{ marginTop: 6 }}>
        {test.ablation.map((r) => (
          <div key={r.to + r.type} className="li" style={{ gridTemplateColumns: '10px minmax(0,1fr) auto auto', cursor: 'default' }}>
            <i className="dot" style={{ background: g.color(r.to) }} />
            <button className="link t" style={{ color: 'var(--fg)', textAlign: 'left' }} onClick={() => onGo(r.to)}>
              {short(g.nodes.get(r.to)?.text ?? r.to, 26)} <span className="muted xs">{EDGE_GLYPH[r.type] ?? ''}w{r.w.toFixed(2)}</span>
            </button>
            <span className="n">
              <span className="mbar" style={{ width: (Math.abs(r.dSum) / mx) * 40, background: r.dSum < 0 ? 'var(--bad)' : 'var(--ok)' }} />
              {r.dLit > 0 ? '+' : ''}
              {r.dLit} / {r.dSum.toFixed(2)}
              {r.critical && <span className="warn" title="critical: without it the neighbour drops out of the context"> <Icon name="warning" size={14} label="critical" /></span>}
            </span>
            <span className="row nowrap">
              {onEdgeAdjust && (
                <>
                  <button className="btn sm icon" title="strengthen synapse" aria-label={`Strengthen ${id}–${r.to}`} onClick={() => onEdgeAdjust(id, r.to, 0.1)}>
                    +
                  </button>
                  <button className="btn sm icon" title="weaken synapse" aria-label={`Weaken ${id}–${r.to}`} onClick={() => onEdgeAdjust(id, r.to, -0.1)}>
                    −
                  </button>
                </>
              )}
              {onEdgeRemove && (
                <button className="btn sm icon danger" title="remove synapse" aria-label={`Remove ${id}–${r.to}`} onClick={() => onEdgeRemove(id, r.to)}>
                  ×
                </button>
              )}
            </span>
          </div>
        ))}
      </div>
      <div className="xs muted" style={{ marginTop: 4 }}>
        <Icon name="warning" size={14} /> = critical synapse: without it the neighbour drops out of the context.
      </div>
      {onReplay && (
        <button className="btn sm" style={{ marginTop: 6 }} onClick={() => onReplay(id)}>
          <Icon name="refresh" /> Replay pulse
        </button>
      )}
    </div>
  );
}

function PathsOutput({ g, from, to, paths, depth, onGo, onShowPath, onFocusBoth }: ToolOutputProps & { from: string; to: string; paths: InspectPath[] }) {
  if (!paths.length)
    return (
      <div className="tool">
        <h4><Icon name="path" /> No path</h4>
        {from} and {to} are not connected in memory.
      </div>
    );
  const hops = paths[0].path.length - 1;
  const edgeType = (a: string, b: string) => g.state.edges.find((e) => edgeKey(e.a, e.b) === edgeKey(a, b))?.type;
  return (
    <div className="tool">
      <h4>
        <Icon name="path" /> Knowledge paths {from} → {to}
      </h4>
      <div className="xs muted">
        Strongest paths (product of synapse weights × memory weights). The first is highlighted. Context at depth {depth}{' '}
        {hops <= depth ? <b className="ok">reaches</b> : <b className="bad">does not reach</b>} the target (needs {hops}).
      </div>
      {paths.map((p, i) => (
        <div key={i} style={{ marginTop: 8 }}>
          <div className="row">
            <b>#{i + 1}</b>
            <span className="muted xs">
              strength {p.strength.toFixed(3)} · {p.path.length - 1} hops
            </span>
            {onShowPath && (
              <button className="btn sm" onClick={() => onShowPath(p.path)}>
                show
              </button>
            )}
            {onFocusBoth && (
              <button className="btn sm" onClick={() => onFocusBoth([from, to])}>
                focus both
              </button>
            )}
          </div>
          <div className="list" style={{ marginTop: 4 }}>
            {p.path.map((id, j) => (
              <NodeRow key={id + j} g={g} id={id} onGo={onGo} right={j ? `${edgeType(p.path[j - 1], id) === 'concept' ? '◆ ' : ''}${(p.weights[j - 1] ?? 0).toFixed(2)}` : 'start'} />
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}
