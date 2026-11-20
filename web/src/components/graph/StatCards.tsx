// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { Params, Stats } from '../../api/types';

/** Eight context stat cards. The first turns red above the token budget. */
export function StatCards({ stats, params, warnTokens = 4000 }: { stats: Stats; params: Params; warnTokens?: number }) {
  const cov = stats.memories ? Math.round((stats.lit / stats.memories) * 100) : 0;
  const cards: [string, string, boolean?][] = [
    [`${stats.lit} / ${stats.memories}`, 'memories lit', stats.tokens > warnTokens],
    [cov + '%', 'memory coverage'],
    [`${stats.reach} / ${params.depth}`, 'reach / depth'],
    ['~' + stats.tokens, 'context tokens', stats.tokens > warnTokens],
    [String(stats.concepts), 'active concepts'],
    [stats.avgWeight.toFixed(2), 'avg weight in context'],
    [String(stats.superseded), 'superseded (faded)'],
    [String(stats.newLinks), 'new links'],
  ];
  if (stats.connectors !== undefined) cards.push([`${stats.connectors} / ${stats.routesUsed ?? 0}`, 'connectors / routes used']);
  return (
    <div className="stats">
      {cards.map(([v, k, alert]) => (
        <div key={k} className={'stat' + (alert ? ' alert' : '')}>
          <div className="v">{v}</div>
          <div className="k">{k}</div>
        </div>
      ))}
    </div>
  );
}
