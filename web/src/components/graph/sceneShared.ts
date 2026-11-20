// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

// Scene types and constants shared with non-3D components (kept free of three.js imports).
import { hash } from '../../model/util';

export type EdgesMode = 'all' | 'ctx' | 'none';

export type ColorMode = 'kind' | 'depth' | 'access';

export type Theme = 'dark' | 'light';

export interface PulseItem {
  from: string;
  to: string;
  delay: number;
  dur: number;
  color: string;
  loop?: boolean;
  size?: number;
}

export interface PulseBatch {
  token: number;
  items: PulseItem[];
  /** Loop period for looping pulses (seconds). */
  period?: number;
}

export const CONNECTOR_COLOR = '#9aa3b2';

/** Deterministic colour of an external source. */
export const sourceColor = (name: string) => `hsl(${hash(name) % 360}, 70%, 62%)`;

/** Above this many nodes the scene switches to a point cloud + meshes only for lit/featured nodes. */
export const BIG_GRAPH = 5000;

/** Access overlay colour of a label (Addendum M); unlabelled nodes are neutral. */
const LABEL_PALETTE = ['#ff6b6b', '#f59f00', '#fcc419', '#94d82d', '#20c997', '#22b8cf', '#4dabf7', '#748ffc', '#b197fc', '#f783ac', '#ff922b', '#63e6be'];
export const labelColor = (label: string | undefined) => (label ? LABEL_PALETTE[hash('acl:' + label) % LABEL_PALETTE.length] : '#5c6370');
