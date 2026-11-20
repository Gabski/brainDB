// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useMemo, useState } from 'react';
import type { BrainEvent } from '../../api/types';
import type { GraphIndex } from '../../model/graph';
import type { LiveStatus } from '../../api/client';

export interface AgentFeedProps {
  feed: BrainEvent[];
  live: LiveStatus;
  g?: GraphIndex | null;
  onGo?: (id: string) => void;
}

/** Live journal / SSE stream with actor and tool filters. Ids touched by agents glow cyan in 3D. */
export function AgentFeed({ feed, live, g, onGo }: AgentFeedProps) {
  const [actor, setActor] = useState('');
  const [tool, setTool] = useState('');
  const [text, setText] = useState('');
  const actors = useMemo(() => [...new Set(feed.map((e) => e.by))].sort(), [feed]);
  const tools = useMemo(() => [...new Set(feed.map((e) => e.tool || e.type))].sort(), [feed]);
  const list = feed.filter((e) => (!actor || e.by === actor) && (!tool || (e.tool || e.type) === tool) && (!text || (e.summary + ' ' + e.ids.join(' ')).toLowerCase().includes(text.toLowerCase())));
  return (
    <div className="split-v">
      <div className="toolbar">
        <span className="live">
          <span className={'dot ' + (live === 'live' ? 'ok' : live === 'connecting' ? 'warn' : 'error')} /> {live === 'live' ? 'LIVE' : live}
        </span>
        <span className="sep" />
        <select className="input" aria-label="Filter by actor" value={actor} onChange={(e) => setActor(e.target.value)}>
          <option value="">all actors</option>
          {actors.map((a) => (
            <option key={a}>{a}</option>
          ))}
        </select>
        <select className="input" aria-label="Filter by tool" value={tool} onChange={(e) => setTool(e.target.value)}>
          <option value="">all tools</option>
          {tools.map((a) => (
            <option key={a}>{a}</option>
          ))}
        </select>
        <input className="input" aria-label="Filter text" placeholder="filter…" value={text} onChange={(e) => setText(e.target.value)} />
        <span className="spacer" />
        <span className="xs muted">
          {list.length} / {feed.length} events
        </span>
      </div>
      <div className="tab-scroll">
        <div className="xs muted" style={{ marginBottom: 8 }}>
          Every change from the UI, MCP agents (<code>braindb mcp</code>) and file reloads appears here live. Memories an agent touched glow cyan in the Graph.
        </div>
        {!list.length && <div className="empty">no operations</div>}
        {list.map((ev, i) => (
          <div key={ev.ts + ':' + i} className="feed-item">
            <div className="row">
              <b className="mono small">{ev.tool || ev.type}</b>
              <span className="muted xs">
                {ev.by} · {new Date(ev.ts).toLocaleTimeString()}
                {ev.db ? ` · db ${ev.db}` : ''}
              </span>
              {ev.type === 'import.progress' && ev.total ? <span className="xs">{ev.done}/{ev.total}</span> : null}
            </div>
            <div className="sum">{ev.summary}</div>
            {ev.ids.length > 0 && (
              <div className="chips" style={{ marginTop: 4 }}>
                {ev.ids.slice(0, 14).map((id) => (
                  <button key={id} className="chip" onClick={() => onGo?.(id)} disabled={!!g && !g.nodes.has(id)}>
                    <i className="dot" style={{ background: g?.color(id) ?? 'var(--mut)' }} />
                    <span className="mono xs">{id}</span>
                  </button>
                ))}
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}

