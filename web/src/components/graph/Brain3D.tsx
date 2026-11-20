// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useEffect, useRef } from 'react';
import { BrainScene, type SceneProps } from './scene';

export type Brain3DProps = SceneProps & {
  onSelect: (id: string) => void;
  onActivate: (id: string) => void;
  className?: string;
};

/** Imperative three.js brain wrapped as a React component. All rendering lives in BrainScene. */
export function Brain3D(props: Brain3DProps) {
  const host = useRef<HTMLDivElement>(null);
  const labels = useRef<HTMLDivElement>(null);
  const tip = useRef<HTMLDivElement>(null);
  const scene = useRef<BrainScene | null>(null);
  const cbs = useRef({ onSelect: props.onSelect, onActivate: props.onActivate });
  cbs.current = { onSelect: props.onSelect, onActivate: props.onActivate };

  useEffect(() => {
    if (!host.current || !labels.current || !tip.current) return;
    let s: BrainScene | null = null;
    try {
      s = new BrainScene(host.current, labels.current, tip.current, {
        onSelect: (id) => cbs.current.onSelect(id),
        onActivate: (id) => cbs.current.onActivate(id),
      });
    } catch (e) {
      host.current.textContent = 'WebGL is not available: ' + (e instanceof Error ? e.message : String(e));
      return;
    }
    scene.current = s;
    return () => {
      s?.dispose();
      scene.current = null;
    };
  }, []);

  useEffect(() => {
    scene.current?.update(props);
  });

  return (
    <div className={'canvas-wrap ' + (props.className ?? '')}>
      <div className="canvas-host" ref={host} />
      <div className="labels" ref={labels} />
      <div className="tip" ref={tip} />
    </div>
  );
}
