// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { Embedding, EmbeddingPatch } from '../../api/types';
import { Slider } from '../common/Slider';

/** Embedding model info, raw/enriched space, APPNP α and hops, hybrid λ, edge coherence. */
export function EmbeddingControls({ embedding, onChange }: { embedding: Embedding; onChange: (p: EmbeddingPatch) => void }) {
  return (
    <div>
      <div className="field">
        <label className="lbl-row" htmlFor="emb-model">
          model
        </label>
        <select id="emb-model" className="input" value={embedding.model} disabled>
          <option value={embedding.model}>{embedding.model} (server, dim {embedding.dim})</option>
        </select>
      </div>
      <div className="row small">
        <span className="muted">space</span>
        <div className="seg">
          <button aria-pressed={embedding.use === 'enriched'} onClick={() => onChange({ use: 'enriched' })}>
            graph-enriched
          </button>
          <button aria-pressed={embedding.use === 'raw'} onClick={() => onChange({ use: 'raw' })}>
            raw
          </button>
        </div>
      </div>
      <Slider label="α enrichment (neighbour share)" value={embedding.alpha} min={0} max={0.9} step={0.05} onChange={(v) => onChange({ alpha: v })} />
      <Slider label="vector propagation hops" value={embedding.hops} min={0} max={5} step={1} digits={0} onChange={(v) => onChange({ hops: v })} />
      <Slider label="hybrid λ (vector ↔ graph)" value={embedding.lambda} min={0} max={1} step={0.05} onChange={(v) => onChange({ lambda: v })} />
      <div className="xs muted">
        edge coherence {embedding.coherenceRaw.toFixed(2)} → {embedding.coherenceEnriched.toFixed(2)} (raw → enriched)
      </div>
    </div>
  );
}
