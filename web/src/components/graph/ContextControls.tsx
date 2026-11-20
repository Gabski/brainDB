// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { Params } from '../../api/types';
import { Slider } from '../common/Slider';
import type { ColorMode, EdgesMode } from './sceneShared';

export interface ContextControlsProps {
  params: Params;
  onParams: (p: Partial<Params>) => void;
  edgesMode?: EdgesMode;
  colorMode?: ColorMode;
  autoRotate?: boolean;
  showConnectors?: boolean;
  onDisplay?: (p: { edgesMode?: EdgesMode; colorMode?: ColorMode; autoRotate?: boolean; showConnectors?: boolean }) => void;
}

/** Depth / threshold / hop sliders, superseded toggle, and (optionally) 3D display options. */
export function ContextControls({ params, onParams, edgesMode, colorMode, autoRotate, showConnectors, onDisplay }: ContextControlsProps) {
  return (
    <div>
      <Slider label="Context depth" value={params.depth} min={0} max={6} step={1} digits={0} onChange={(v) => onParams({ depth: v })} />
      <Slider label="Lit threshold (act ≥)" value={params.threshold} min={0.02} max={0.6} step={0.01} onChange={(v) => onParams({ threshold: v })} />
      <Slider label="Damping per hop" value={params.hop} min={0.4} max={1} step={0.01} onChange={(v) => onParams({ hop: v })} />
      <label className="check small">
        <input type="checkbox" checked={params.showSuperseded} onChange={(e) => onParams({ showSuperseded: e.target.checked })} /> superseded items conduct
      </label>
      {onDisplay && (
        <div className="col" style={{ marginTop: 8 }}>
          <div className="row small">
            <label className="row">
              edges
              <select className="input" value={edgesMode} onChange={(e) => onDisplay({ edgesMode: e.target.value as EdgesMode })}>
                <option value="all">all</option>
                <option value="ctx">context only</option>
                <option value="none">none</option>
              </select>
            </label>
            <label className="row">
              colour
              <select className="input" value={colorMode} onChange={(e) => onDisplay({ colorMode: e.target.value as ColorMode })}>
                <option value="kind">kind</option>
                <option value="depth">depth</option>
              </select>
            </label>
          </div>
          <div className="row small">
            <label className="check">
              <input type="checkbox" checked={!!autoRotate} onChange={(e) => onDisplay({ autoRotate: e.target.checked })} /> auto-rotate
            </label>
            <label className="check">
              <input type="checkbox" checked={!!showConnectors} onChange={(e) => onDisplay({ showConnectors: e.target.checked })} /> show connectors ⧉
            </label>
          </div>
        </div>
      )}
    </div>
  );
}
