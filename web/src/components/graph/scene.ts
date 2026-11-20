// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

// Imperative three.js scene for the brain: glow/fade nodes,
// additive edge colouring, pulses, labels, orbit camera. React drives it through update().
import * as THREE from 'three';
import type { BrainNode, Connector, Edge, Vec3 } from '../../api/types';
import { computeLayout, LOBES, timeX, type ViewName } from '../../model/layout';
import { layoutMap } from '../../model/layout2d';
import { DEPTH_COLORS, edgeKey, isConceptNode } from '../../model/graph';
import { clamp, gauss, hash, mulberry32, short } from '../../model/util';
import { BIG_GRAPH, CONNECTOR_COLOR, labelColor, sourceColor, type ColorMode, type EdgesMode, type PulseBatch, type PulseItem, type Theme } from './sceneShared';
export { BIG_GRAPH, CONNECTOR_COLOR, sourceColor };
export type { ColorMode, EdgesMode, PulseBatch, PulseItem, Theme };



export interface SceneProps {
  nodes: BrainNode[];
  edges: Edge[];
  kindColor: (kind: string) => string;
  kindLabel: (kind: string) => string;
  conceptIds: string[];
  view: ViewName;
  serverPositions?: Record<string, Vec3>;
  treeEdges?: [string, string][];
  colorMode: ColorMode;
  edgesMode: EdgesMode;
  showSuperseded: boolean;
  selected: string | null;
  focus: string[];
  highlight: string[];
  pathEdges: [string, string][];
  pulses: PulseBatch;
  flyTo: { id: string; token: number } | null;
  autoRotate: boolean;
  theme: Theme;
  embeddingUse: string;
  depth: number;
  pickMode: boolean;
  /** Addendum H: structural connectors (hollow rings, dashed edges). */
  connectors?: Connector[];
  showConnectors?: boolean;
}


interface CObj {
  c: Connector;
  members: string[];
  mesh: THREE.Mesh;
  cur: THREE.Vector3;
}

export interface SceneCallbacks {
  onSelect(id: string): void;
  onActivate(id: string): void;
}

/** Position state of every node (shared by meshes and the big-graph point cloud). */
interface PNode {
  node: BrainNode;
  cur: THREE.Vector3;
  tgt: THREE.Vector3;
}
interface NObj {
  node: BrainNode;
  mesh: THREE.Mesh;
  glow: THREE.Sprite;
  cur: THREE.Vector3;
  tgt: THREE.Vector3;
  gs: number;
  phase: number;
}
interface Pulse {
  sp: THREE.Sprite;
  from: string;
  to: string;
  t: number;
  dur: number;
  loop: boolean;
  period: number;
}


const PALETTE = {
  dark: { bg: '#05070d', fade: '#0b1020', shellL: 0.25, shellOpacity: 0.45 },
  light: { bg: '#f2f4f8', fade: '#dde2ec', shellL: 0.55, shellOpacity: 0.35 },
};


function radialTex(ring = false): THREE.Texture {
  const c = document.createElement('canvas');
  c.width = c.height = 128;
  const g = c.getContext('2d');
  if (g) {
    if (ring) {
      g.strokeStyle = '#fff';
      g.lineWidth = 7;
      g.beginPath();
      g.arc(64, 64, 52, 0, Math.PI * 2);
      g.stroke();
    } else {
      const gr = g.createRadialGradient(64, 64, 0, 64, 64, 64);
      gr.addColorStop(0, 'rgba(255,255,255,1)');
      gr.addColorStop(0.25, 'rgba(255,255,255,.45)');
      gr.addColorStop(1, 'rgba(255,255,255,0)');
      g.fillStyle = gr;
      g.fillRect(0, 0, 128, 128);
    }
  }
  return new THREE.CanvasTexture(c);
}

function circle(r: number, axis: number, color: number): THREE.Line {
  const pts: THREE.Vector3[] = [];
  for (let i = 0; i <= 96; i++) {
    const a = (i / 96) * Math.PI * 2,
      c = Math.cos(a) * r,
      s = Math.sin(a) * r;
    pts.push(axis === 0 ? new THREE.Vector3(0, c, s) : axis === 1 ? new THREE.Vector3(c, 0, s) : new THREE.Vector3(c, s, 0));
  }
  return new THREE.Line(new THREE.BufferGeometry().setFromPoints(pts), new THREE.LineBasicMaterial({ color, transparent: true, opacity: 0.7 }));
}
const seg = (a: Vec3, b: Vec3, color: number) =>
  new THREE.Line(new THREE.BufferGeometry().setFromPoints([new THREE.Vector3(...a), new THREE.Vector3(...b)]), new THREE.LineBasicMaterial({ color }));

/** Satellite ring position of an external node (federated sources orbit outside the brain). */
export function satellitePos(n: BrainNode, sources: string[]): Vec3 {
  const src = n.source ?? n.id.split(':')[0];
  const si = Math.max(0, sources.indexOf(src));
  const th = ((si + 0.5) / Math.max(1, sources.length)) * Math.PI * 2 + 0.6;
  const cx = Math.cos(th) * 125,
    cz = Math.sin(th) * 125,
    cy = 55;
  const r = mulberry32(hash(n.id));
  const a = r() * Math.PI * 2,
    R = 20 + r() * 4;
  return [cx + Math.cos(a) * R * Math.cos(th), cy + Math.sin(a) * R, cz + Math.cos(a) * R * Math.sin(th)];
}

export class BrainScene {
  private renderer: THREE.WebGLRenderer;
  private scene = new THREE.Scene();
  private camera = new THREE.PerspectiveCamera(52, 1, 0.1, 3000);
  private ortho = new THREE.OrthographicCamera(-1, 1, 1, -1, 0.1, 20000);
  private GLOW = radialTex();
  private RING = radialTex(true);
  private geoSphere = new THREE.SphereGeometry(1, 16, 12);
  private geoOcta = new THREE.OctahedronGeometry(1, 0);
  private geoTorus = new THREE.TorusGeometry(1, 0.38, 8, 20);
  private shell: THREE.Points;
  private deco: Record<string, THREE.Group> = {};
  private objs = new Map<string, NObj>();
  private P = new Map<string, PNode>();
  private big = false;
  private cloud: THREE.Points | null = null;
  private cloudIds: string[] = [];
  private featuredKey = '';
  private C = new Map<string, CObj>();
  private connLines: THREE.LineSegments | null = null;
  private connEdges: [string, string][] = [];
  private hoverConn: string | null = null;
  private geoRing = new THREE.TorusGeometry(1, 0.12, 6, 24);
  private edgeLines: THREE.LineSegments | null = null;
  private edgeList: Edge[] = [];
  private pulses: Pulse[] = [];
  private selRing: THREE.Sprite;
  private raycaster = new THREE.Raycaster();
  private mouse = new THREE.Vector2();
  private orbit = { target: new THREE.Vector3(), goal: null as THREE.Vector3 | null, r: 270, th: 0.7, ph: 1.2 };
  private drag: { x: number; y: number; sx: number; sy: number; pan: boolean } | null = null;
  private hover: string | null = null;
  private mapNotes: { t: string; p: Vec3 }[] = [];
  private shellTarget = 0.45;
  private raf = 0;
  private last = performance.now();
  private labelPool: HTMLDivElement[] = [];
  private p: SceneProps | null = null;
  private prev: Partial<SceneProps> = {};
  private ro: ResizeObserver;
  private disposed = false;
  private tmpC = new THREE.Color();
  private tmpC2 = new THREE.Color();

  constructor(
    private host: HTMLElement,
    private labelsEl: HTMLElement,
    private tipEl: HTMLElement,
    private cb: SceneCallbacks,
  ) {
    this.renderer = new THREE.WebGLRenderer({ antialias: true, preserveDrawingBuffer: true });
    this.renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2));
    host.appendChild(this.renderer.domElement);
    this.scene.background = new THREE.Color(PALETTE.dark.bg);
    this.scene.fog = new THREE.FogExp2(0x05070d, 0.0022);
    this.shell = this.makeShell();
    this.makeDeco();
    this.selRing = new THREE.Sprite(new THREE.SpriteMaterial({ map: this.RING, color: '#4dabf7', transparent: true, depthWrite: false }));
    this.selRing.visible = false;
    this.scene.add(this.selRing);
    this.bindEvents();
    this.ro = new ResizeObserver(() => this.resize());
    this.ro.observe(host);
    this.resize();
    this.raf = requestAnimationFrame(this.frame);
  }

  // ------------------------------------------------------------------ setup
  private makeShell(): THREE.Points {
    const N = 6500,
      pos = new Float32Array(N * 3),
      col = new Float32Array(N * 3),
      r = mulberry32(3);
    for (let i = 0; i < N; i++) {
      let x = gauss(r),
        y = gauss(r),
        z = gauss(r);
      const l = Math.hypot(x, y, z) || 1;
      x /= l;
      y /= l;
      z /= l;
      const s = x < 0 ? -1 : 1,
        ax = Math.abs(x);
      const sul = 1 + 0.05 * Math.sin(y * 17 + z * 11) * Math.sin(ax * 9 + z * 6);
      const depthIn = 1 - Math.pow(r(), 6) * 0.25;
      pos[i * 3] = s * (3 + ax * 66 * sul * depthIn);
      pos[i * 3 + 1] = (y < 0 ? y * 46 : y * 60) * sul * depthIn + (z < -0.3 && y < 0 ? -8 * -z : 0);
      pos[i * 3 + 2] = z * 92 * sul * depthIn;
      col[i * 3] = 0.6 + r() * 0.08; // hue, converted in applyTheme
      col[i * 3 + 1] = r();
    }
    const g = new THREE.BufferGeometry();
    g.setAttribute('position', new THREE.BufferAttribute(pos, 3));
    g.setAttribute('color', new THREE.BufferAttribute(new Float32Array(N * 3), 3));
    g.userData.seed = col;
    const p = new THREE.Points(
      g,
      new THREE.PointsMaterial({ size: 1.2, vertexColors: true, transparent: true, opacity: 0.45, depthWrite: false, blending: THREE.AdditiveBlending }),
    );
    this.scene.add(p);
    return p;
  }

  private makeDeco() {
    for (const k of ['layers', 'sphere', 'time', 'embed', 'tree']) {
      const g = new THREE.Group();
      g.visible = false;
      this.scene.add(g);
      this.deco[k] = g;
    }
    const axis = 0x24325a;
    this.deco.embed.add(seg([-125, 0, 0], [125, 0, 0], axis), seg([0, -100, 0], [0, 100, 0], axis), seg([0, 0, -125], [0, 0, 125], axis));
    for (const y of [80, 40, 0, -40, -80]) {
      const gh = new THREE.GridHelper(240, 12, 0x1a2440, 0x101830);
      gh.position.y = y;
      const m = gh.material as THREE.Material;
      m.transparent = true;
      m.opacity = 0.45;
      this.deco.layers.add(gh);
    }
    for (let d = 1; d <= 6; d++) for (let ax = 0; ax < 3; ax++) this.deco.sphere.add(circle(d * 30, ax, 0x1c2a4a));
    this.deco.sphere.add(circle(165, 1, 0x1c2a4a));
    this.deco.time.add(seg([-105, -45, 0], [125, -45, 0], axis), seg([-105, -45, 0], [-105, 50, 0], axis));
    for (const a of [0, 7, 30, 100, 300]) this.deco.time.add(seg([timeX(a), -45, -60], [timeX(a), -45, 60], 0x162038));
  }

  private applyTheme(theme: Theme) {
    const P = PALETTE[theme];
    (this.scene.background as THREE.Color).set(P.bg);
    (this.scene.fog as THREE.FogExp2).color.set(P.bg);
    const g = this.shell.geometry;
    const seed = g.userData.seed as Float32Array;
    const col = g.getAttribute('color') as THREE.BufferAttribute;
    const c = new THREE.Color();
    for (let i = 0; i < col.count; i++) {
      c.setHSL(seed[i * 3], 0.5, P.shellL + seed[i * 3 + 1] * 0.15);
      col.setXYZ(i, c.r, c.g, c.b);
    }
    col.needsUpdate = true;
    const blending = theme === 'dark' ? THREE.AdditiveBlending : THREE.NormalBlending;
    (this.shell.material as THREE.PointsMaterial).blending = blending;
    (this.shell.material as THREE.PointsMaterial).needsUpdate = true;
    if (this.edgeLines) {
      (this.edgeLines.material as THREE.LineBasicMaterial).blending = blending;
      (this.edgeLines.material as THREE.LineBasicMaterial).needsUpdate = true;
    }
    for (const o of this.objs.values()) {
      o.glow.material.blending = blending;
      o.glow.material.needsUpdate = true;
    }
    for (const g2 of Object.values(this.deco))
      g2.traverse((ch) => {
        const m = (ch as THREE.Line).material as THREE.LineBasicMaterial | undefined;
        if (m && 'color' in m && !(ch instanceof THREE.GridHelper)) m.color.set(theme === 'dark' ? 0x24325a : 0xb8c2d6);
      });
  }

  // ------------------------------------------------------------------ update from React
  update(p: SceneProps) {
    const prev = this.prev;
    this.p = p;
    if (prev.theme !== p.theme) this.applyTheme(p.theme);
    if (prev.nodes !== p.nodes) this.syncNodes(p.nodes);
    const featured = this.featured(p);
    const fKey = this.big && featured ? [...featured].sort().join(',') : '';
    if (prev.nodes !== p.nodes || fKey !== this.featuredKey) this.syncMeshes(p.nodes, featured);
    if (prev.edges !== p.edges || prev.nodes !== p.nodes || fKey !== this.featuredKey) this.rebuildEdges(p.edges, featured);
    this.featuredKey = fKey;
    if (prev.connectors !== p.connectors || prev.edges !== p.edges || prev.nodes !== p.nodes) this.syncConnectors(p);
    for (const o of this.C.values()) o.mesh.visible = !!p.showConnectors;
    if (this.connLines) this.connLines.visible = !!p.showConnectors && p.edgesMode !== 'none';
    const mapPick = p.view === 'map' && (prev.selected !== p.selected || prev.focus !== p.focus);
    if (prev.nodes !== p.nodes || prev.view !== p.view || prev.edges !== p.edges || prev.serverPositions !== p.serverPositions || mapPick) this.applyLayout(p, prev.view !== p.view);
    if (prev.view !== p.view) this.onViewChange(p.view);
    if (prev.pulses?.token !== p.pulses.token) this.setPulses(p.pulses);
    if (p.flyTo && prev.flyTo?.token !== p.flyTo.token) {
      const o = this.P.get(p.flyTo.id);
      if (o) this.orbit.goal = o.tgt.clone();
    }
    this.renderer.domElement.style.cursor = p.pickMode ? 'crosshair' : 'grab';
    this.renderer.domElement.setAttribute(
      'aria-label',
      p.view === 'map'
        ? '2D map. Drag to pan, wheel to zoom, click a node to inspect it.'
        : '3D brain view. Drag to rotate, wheel to zoom, click a node to inspect it.',
    );
    this.paint();
    this.prev = { ...p };
  }

  /** Nodes that get a full mesh: all of them on normal graphs, only lit/selected/highlighted ones on big graphs. */
  private featured(p: SceneProps): Set<string> | null {
    if (!this.big) return null;
    const f = new Set<string>();
    for (const n of p.nodes) if ((n.activation !== undefined && n.activation !== null) || n.external) f.add(n.id);
    for (const id of [...p.focus, ...p.highlight, ...p.pathEdges.flat()]) f.add(id);
    if (p.selected) f.add(p.selected);
    return f;
  }

  private syncNodes(nodes: BrainNode[]) {
    this.big = nodes.length > BIG_GRAPH;
    const seen = new Set<string>();
    for (const n of nodes) {
      seen.add(n.id);
      const pn = this.P.get(n.id);
      if (pn) {
        pn.node = n;
        continue;
      }
      const parent = n.parent ? this.P.get(n.parent) : undefined;
      const start = parent ? parent.cur.clone() : new THREE.Vector3();
      this.P.set(n.id, { node: n, cur: start, tgt: start.clone() });
    }
    for (const id of [...this.P.keys()]) if (!seen.has(id)) this.P.delete(id);
    this.rebuildCloud(nodes);
  }

  private rebuildCloud(nodes: BrainNode[]) {
    if (this.cloud) {
      this.scene.remove(this.cloud);
      this.cloud.geometry.dispose();
      (this.cloud.material as THREE.Material).dispose();
      this.cloud = null;
    }
    this.cloudIds = [];
    if (!this.big) return;
    this.cloudIds = nodes.map((n) => n.id);
    const g = new THREE.BufferGeometry();
    g.setAttribute('position', new THREE.BufferAttribute(new Float32Array(nodes.length * 3), 3));
    g.setAttribute('color', new THREE.BufferAttribute(new Float32Array(nodes.length * 3), 3));
    this.cloud = new THREE.Points(g, new THREE.PointsMaterial({ size: 2.2, vertexColors: true, transparent: true, opacity: 0.85, depthWrite: false }));
    this.cloud.frustumCulled = false;
    this.scene.add(this.cloud);
  }

  private syncMeshes(nodes: BrainNode[], featured: Set<string> | null) {
    const want = new Set<string>();
    for (const n of nodes) {
      if (featured && !featured.has(n.id)) continue;
      want.add(n.id);
      const pn = this.P.get(n.id)!;
      const o = this.objs.get(n.id);
      if (o) {
        if (o.node.kind !== n.kind || !!o.node.external !== !!n.external) this.dropObj(n.id);
        else {
          o.node = n;
          continue;
        }
      }
      const concept = isConceptNode(n);
      const mesh = new THREE.Mesh(n.external ? this.geoTorus : concept ? this.geoOcta : this.geoSphere, new THREE.MeshBasicMaterial({ transparent: true, opacity: 1 }));
      mesh.userData.id = n.id;
      const glow = new THREE.Sprite(
        new THREE.SpriteMaterial({
          map: this.GLOW,
          transparent: true,
          depthWrite: false,
          blending: this.p?.theme === 'light' ? THREE.NormalBlending : THREE.AdditiveBlending,
        }),
      );
      this.scene.add(mesh, glow);
      this.objs.set(n.id, { node: n, mesh, glow, cur: pn.cur, tgt: pn.tgt, gs: 0, phase: hash(n.id) % 7 });
    }
    for (const id of [...this.objs.keys()]) if (!want.has(id)) this.dropObj(id);
  }

  private syncConnectors(p: SceneProps) {
    for (const o of this.C.values()) {
      this.scene.remove(o.mesh);
      (o.mesh.material as THREE.Material).dispose();
    }
    this.C.clear();
    if (this.connLines) {
      this.scene.remove(this.connLines);
      this.connLines.geometry.dispose();
      (this.connLines.material as THREE.Material).dispose();
      this.connLines = null;
    }
    const list = p.connectors ?? [];
    if (!list.length) return;
    const members = new Map<string, string[]>();
    this.connEdges = [];
    for (const e of p.edges) {
      if (e.type !== 'connector') continue;
      const [h, m] = e.a.startsWith('H:') ? [e.a, e.b] : [e.b, e.a];
      if (!this.P.has(m)) continue;
      if (!members.has(h)) members.set(h, []);
      members.get(h)!.push(m);
      this.connEdges.push([h, m]);
    }
    for (const c of list) {
      const mesh = new THREE.Mesh(this.geoRing, new THREE.MeshBasicMaterial({ color: CONNECTOR_COLOR, transparent: true, opacity: 0.85 }));
      mesh.userData.id = c.id;
      mesh.scale.setScalar(2.6);
      this.scene.add(mesh);
      const ms = members.get(c.id) ?? [];
      const cur = new THREE.Vector3();
      ms.forEach((m) => cur.add(this.P.get(m)!.cur));
      if (ms.length) cur.multiplyScalar(1 / ms.length);
      this.C.set(c.id, { c, members: ms, mesh, cur });
    }
    const g = new THREE.BufferGeometry();
    g.setAttribute('position', new THREE.BufferAttribute(new Float32Array(this.connEdges.length * 6), 3));
    this.connLines = new THREE.LineSegments(g, new THREE.LineDashedMaterial({ color: CONNECTOR_COLOR, dashSize: 2, gapSize: 2, transparent: true, opacity: 0.45, depthWrite: false }));
    this.connLines.frustumCulled = false;
    this.scene.add(this.connLines);
  }

  private dropObj(id: string) {
    const o = this.objs.get(id);
    if (!o) return;
    this.scene.remove(o.mesh, o.glow);
    (o.mesh.material as THREE.Material).dispose();
    o.glow.material.dispose();
    this.objs.delete(id);
  }

  private rebuildEdges(edges: Edge[], featured: Set<string> | null) {
    if (this.edgeLines) {
      this.scene.remove(this.edgeLines);
      this.edgeLines.geometry.dispose();
      (this.edgeLines.material as THREE.Material).dispose();
    }
    this.edgeList = edges.filter((e) => this.P.has(e.a) && this.P.has(e.b) && (!featured || featured.has(e.a) || featured.has(e.b)));
    const g = new THREE.BufferGeometry();
    g.setAttribute('position', new THREE.BufferAttribute(new Float32Array(this.edgeList.length * 6), 3));
    g.setAttribute('color', new THREE.BufferAttribute(new Float32Array(this.edgeList.length * 6), 3));
    this.edgeLines = new THREE.LineSegments(
      g,
      new THREE.LineBasicMaterial({
        vertexColors: true,
        transparent: true,
        depthWrite: false,
        blending: this.p?.theme === 'light' ? THREE.NormalBlending : THREE.AdditiveBlending,
      }),
    );
    this.scene.add(this.edgeLines);
  }

  /** Flat reading: concepts on a header, memories under the strongest concept, the rest on a shelf. */
  private mapPositions(p: SceneProps): Map<string, Vec3> {
    const featured = new Set<string>();
    for (const n of p.nodes) if (n.activation != null) featured.add(n.id);
    for (const id of p.focus) featured.add(id);
    if (p.selected) featured.add(p.selected);
    const lay = layoutMap(p.nodes, p.edges, p.conceptIds, featured);
    const S = 0.35;
    const out = new Map<string, Vec3>();
    for (const [id, pt] of lay.pos) out.set(id, [pt.x * S, 0, pt.y * S]);
    this.mapNotes = lay.captions.map((c) => ({ t: c.text, p: [c.x * S, 0, c.y * S] as Vec3 }));
    return out;
  }

  private applyLayout(p: SceneProps, instant: boolean) {
    const nodes = p.nodes;
    const local = nodes.filter((n) => !n.external);
    const ext = nodes.filter((n) => n.external);
    const pos = p.view === 'map' ? this.mapPositions(p) : computeLayout(local, p.view, p.conceptIds, p.serverPositions);
    if (p.view !== 'map') {
      const sources = [...new Set(ext.map((n) => n.source ?? n.id.split(':')[0]))].sort();
      for (const n of ext) {
        const sp = p.serverPositions?.[n.id];
        const lit = n.activation !== undefined && n.activation !== null;
        pos.set(n.id, (p.view === 'embed' || p.view === 'tree') && sp ? sp : p.view === 'sphere' && lit ? computeLayout([n], 'sphere', []).get(n.id)! : satellitePos(n, sources));
      }
    }
    for (const n of nodes) {
      const o = this.P.get(n.id);
      const q = pos.get(n.id);
      if (!o || !q) continue;
      o.tgt.set(q[0], q[1], q[2]);
      if (instant && !this.prev.nodes) o.cur.copy(o.tgt);
    }
    if (p.view !== 'map') this.mapNotes = [];
    this.shellTarget = p.view === 'brain' ? PALETTE[p.theme].shellOpacity : 0;
    for (const [k, g] of Object.entries(this.deco)) g.visible = k === p.view;
  }

  private onViewChange(view: ViewName) {
    this.orbit.goal = new THREE.Vector3();
    if (view === 'map') {
      this.orbit.th = 0;
      this.orbit.ph = 0.001;
      this.frameFlat();
      return;
    }
    if (this.prev.view === 'map') {
      this.orbit.th = 0.7;
      this.orbit.ph = 1.2;
    }
    this.orbit.r = view === 'sphere' ? 380 : view === 'time' ? 300 : view === 'tree' ? 360 : view === 'embed' ? 330 : 270;
    if (view === 'time') {
      this.orbit.th = 0;
      this.orbit.ph = 1.45;
    }
  }

  /** Frame the lit context. The shelf stays off to the side until the user zooms out. */
  private frameFlat() {
    const lit: THREE.Vector3[] = [];
    for (const pn of this.P.values()) if (pn.node.activation != null) lit.push(pn.tgt);
    const use = lit.length >= 2 ? lit : [...this.P.values()].map((pn) => pn.tgt);
    if (!use.length) return;
    let minX = Infinity, maxX = -Infinity, minZ = Infinity, maxZ = -Infinity;
    for (const q of use) {
      minX = Math.min(minX, q.x);
      maxX = Math.max(maxX, q.x);
      minZ = Math.min(minZ, q.z);
      maxZ = Math.max(maxZ, q.z);
    }
    this.orbit.target.set((minX + maxX) / 2, 0, (minZ + maxZ) / 2);
    this.orbit.goal = null;
    const aspect = this.host.clientWidth / Math.max(1, this.host.clientHeight);
    const halfW = (maxX - minX) / 2 + 36;
    const halfH = (maxZ - minZ) / 2 + 36;
    this.orbit.r = Math.max(halfH, halfW / Math.max(aspect, 0.25), 40);
  }

  // ------------------------------------------------------------------ painting
  private nodeColor(n: BrainNode): string {
    const p = this.p!;
    const lit = n.activation !== undefined && n.activation !== null;
    if (p.colorMode === 'access') return labelColor(n.labels?.[0]);
    if (p.colorMode === 'depth' && lit) return DEPTH_COLORS[Math.min(n.depth ?? 0, 6)];
    if (n.external) return sourceColor(n.source ?? n.id.split(':')[0]);
    return p.kindColor(n.kind);
  }

  private paint() {
    const p = this.p;
    if (!p) return;
    const hl = new Set(p.highlight);
    const hc = this.hoverConn ? this.C.get(this.hoverConn) : undefined;
    if (hc) hc.members.forEach((m) => hl.add(m));
    for (const o of this.C.values()) {
      const m = o.mesh.material as THREE.MeshBasicMaterial;
      const on = o.c.id === this.hoverConn || o.c.id === p.selected;
      m.color.set(on ? '#3bc9db' : CONNECTOR_COLOR);
      m.opacity = on ? 1 : 0.8;
    }
    const fade = PALETTE[p.theme].fade;
    const light = p.theme === 'light';
    for (const o of this.objs.values()) {
      const n = o.node,
        a = n.activation ?? 0,
        lit = n.activation !== undefined && n.activation !== null,
        w = n.weight,
        m = o.mesh.material as THREE.MeshBasicMaterial;
      const concept = isConceptNode(n);
      const col = this.nodeColor(n);
      if (p.colorMode === 'access' && n.labels?.length) {
        // restricted area: always visible, glowing in the label colour
        m.color.set(col);
        m.opacity = 1;
        m.depthWrite = true;
        o.glow.visible = true;
        o.glow.material.color.set(col);
        o.glow.material.opacity = light ? 0.45 : 0.7;
        o.gs = 11;
      } else if (lit) {
        m.color.set(col);
        m.opacity = 1;
        m.depthWrite = true;
        o.glow.visible = true;
        o.glow.material.color.set(col);
        o.glow.material.opacity = (0.25 + 0.65 * a) * (light ? 0.55 : 1);
        o.gs = (concept ? 15 : 9) * (0.55 + a);
      } else {
        this.tmpC.set(col).lerp(this.tmpC2.set(fade), n.supersededBy ? 0.88 : 0.55);
        m.color.copy(this.tmpC);
        m.opacity = n.supersededBy ? 0.07 : 0.1 + 0.55 * w;
        if (n.external) m.opacity = Math.max(m.opacity, 0.5);
        m.depthWrite = false;
        o.glow.visible = false;
        if (hl.has(n.id)) {
          m.color.set(col);
          m.opacity = 0.95;
          o.glow.visible = true;
          o.glow.material.color.set('#3bc9db');
          o.glow.material.opacity = light ? 0.5 : 0.85;
          o.gs = 9;
        }
      }
      const sc = concept ? 3.1 : n.external ? 2.4 : Math.max(lit ? 1.3 : 0.6, 0.6 + 2.2 * w);
      o.mesh.scale.setScalar(sc);
    }
    if (this.cloud) {
      const col = (this.cloud.geometry.getAttribute('color') as THREE.BufferAttribute).array as Float32Array;
      const bg = this.tmpC2.set(fade);
      const c = new THREE.Color();
      this.cloudIds.forEach((id, i) => {
        const n = this.P.get(id)?.node;
        if (!n) return;
        c.set(this.nodeColor(n));
        const lit = n.activation !== undefined && n.activation !== null;
        if (!lit) c.lerp(bg, n.supersededBy ? 0.88 : 0.75 - 0.4 * n.weight);
        col.set([c.r, c.g, c.b], i * 3);
      });
      (this.cloud.geometry.getAttribute('color') as THREE.BufferAttribute).needsUpdate = true;
    }
    this.paintEdges();
  }

  private paintEdges() {
    const p = this.p;
    if (!p || !this.edgeLines) return;
    const col = (this.edgeLines.geometry.getAttribute('color') as THREE.BufferAttribute).array as Float32Array;
    const TINT: Record<string, [number, number, number]> = {
      similar: [0.15, 0.8, 0.9],
      part: [0.45, 0.9, 0.35],
      seq: [0.45, 0.9, 0.35],
      external: [0.75, 0.45, 1],
    };
    const path = new Set(p.pathEdges.map(([a, b]) => edgeKey(a, b)));
    const tree = new Set((p.treeEdges ?? []).map(([a, b]) => edgeKey(a, b)));
    const light = p.theme === 'light';
    const bg = new THREE.Color(PALETTE[p.theme].bg);
    const out = (r: number, g: number, b: number, i: number) => {
      if (light) {
        // on a light background, interpolate from the background towards the colour
        const k = clamp(Math.max(r, g, b) * 2.6);
        const m = Math.max(r, g, b) || 1;
        const cr = (r / m) * 0.75,
          cg = (g / m) * 0.75,
          cb = (b / m) * 0.75;
        r = bg.r + (cr - bg.r) * k;
        g = bg.g + (cg - bg.g) * k;
        b = bg.b + (cb - bg.b) * k;
      }
      col.set([r, g, b, r, g, b], i * 6);
    };
    this.edgeList.forEach((e, i) => {
      const A = this.P.get(e.a)!.node,
        B = this.P.get(e.b)!.node;
      const la = A.activation,
        lb = B.activation;
      const litA = la !== undefined && la !== null,
        litB = lb !== undefined && lb !== null;
      let r = 0,
        g = 0,
        b = 0;
      const key = edgeKey(e.a, e.b);
      const treeE = A.via === e.b || B.via === e.a;
      if (path.has(key)) {
        r = 1;
        g = 0.85;
        b = 0.25;
      } else if (p.edgesMode === 'none') {
        /* nothing */
      } else if (litA && litB) {
        const k = Math.min(la!, lb!) * (treeE ? 1 : 0.45) * (0.4 + 0.6 * e.w);
        this.tmpC.set(this.nodeColor(A)).lerp(this.tmpC2.set(this.nodeColor(B)), 0.5);
        r = this.tmpC.r * k;
        g = this.tmpC.g * k;
        b = this.tmpC.b * k;
      } else if (p.view === 'tree' && tree.size) {
        const k = tree.has(key) ? 0.3 * (0.4 + 0.6 * e.w) : 0,
          t = TINT[e.type] ?? [0.6, 0.75, 1];
        r = t[0] * k;
        g = t[1] * k;
        b = t[2] * k;
      } else if (p.edgesMode === 'all') {
        let k = (litA || litB ? 0.09 : 0.035) * e.w * (p.view === 'embed' ? 0.5 : 1);
        const t = TINT[e.type];
        if (t) {
          k *= 2.2;
          [r, g, b] = t.map((x) => x * k) as [number, number, number];
        } else {
          r = 0.55 * k;
          g = 0.65 * k;
          b = k;
        }
        if (e.type === 'supersede') {
          r = 0.12 * (p.showSuperseded ? 2 : 0.6);
          g = 0.04;
          b = 0.16 * (p.showSuperseded ? 2 : 0.6);
        }
      }
      if (e.type === 'new' && !path.has(key)) {
        const k = p.edgesMode === 'none' ? 0 : 0.75;
        r = Math.max(r, k);
        g = Math.max(g, 0.6 * k);
        b = Math.max(b, 0.1 * k);
      }
      out(r, g, b, i);
    });
    (this.edgeLines.geometry.getAttribute('color') as THREE.BufferAttribute).needsUpdate = true;
  }

  // ------------------------------------------------------------------ pulses
  private clearPulses() {
    for (const pu of this.pulses) {
      this.scene.remove(pu.sp);
      pu.sp.material.dispose();
    }
    this.pulses = [];
  }
  private setPulses(batch: PulseBatch) {
    this.clearPulses();
    const light = this.p?.theme === 'light';
    for (const it of batch.items) {
      const sp = new THREE.Sprite(
        new THREE.SpriteMaterial({
          map: this.GLOW,
          color: it.color,
          transparent: true,
          depthWrite: false,
          blending: light ? THREE.NormalBlending : THREE.AdditiveBlending,
        }),
      );
      sp.scale.setScalar(it.size ?? 5);
      sp.visible = false;
      this.scene.add(sp);
      this.pulses.push({ sp, from: it.from, to: it.to, t: -it.delay, dur: it.dur, loop: !!it.loop, period: Math.max(batch.period ?? 0, it.dur + 0.2) });
    }
  }

  // ------------------------------------------------------------------ camera + input
  private flat() {
    return this.p?.view === 'map';
  }

  private cam(): THREE.Camera {
    return this.flat() ? this.ortho : this.camera;
  }

  private zoomLimit(r: number) {
    return clamp(r, 25, this.flat() ? 8000 : 900);
  }

  private updCam() {
    const o = this.orbit;
    if (this.flat()) {
      const cam = this.ortho;
      cam.up.set(0, 0, -1);
      cam.position.set(o.target.x, o.target.y + Math.max(o.r, 1), o.target.z);
      cam.lookAt(o.target);
      const aspect = this.host.clientWidth / Math.max(1, this.host.clientHeight);
      const half = Math.max(20, o.r);
      cam.left = -half * aspect;
      cam.right = half * aspect;
      cam.top = half;
      cam.bottom = -half;
      cam.near = 0.1;
      cam.far = 20000;
      cam.updateProjectionMatrix();
      cam.updateMatrix();
      return;
    }
    this.camera.up.set(0, 1, 0);
    const sp = Math.sin(o.ph);
    this.camera.position.set(o.target.x + o.r * sp * Math.sin(o.th), o.target.y + o.r * Math.cos(o.ph), o.target.z + o.r * sp * Math.cos(o.th));
    this.camera.lookAt(o.target);
  }

  private pick(ev: { clientX: number; clientY: number }): string | null {
    const rc = this.renderer.domElement.getBoundingClientRect();
    this.mouse.set(((ev.clientX - rc.left) / rc.width) * 2 - 1, (-(ev.clientY - rc.top) / rc.height) * 2 + 1);
    this.raycaster.setFromCamera(this.mouse, this.cam());
    const meshes = [...this.objs.values()].map((o) => o.mesh);
    if (this.p?.showConnectors) for (const c of this.C.values()) meshes.push(c.mesh);
    const hit = this.raycaster.intersectObjects(meshes)[0];
    if (hit) return hit.object.userData.id as string;
    if (this.cloud) {
      this.raycaster.params.Points = { threshold: 1.6 };
      const h2 = this.raycaster.intersectObject(this.cloud)[0];
      if (h2 && h2.index !== undefined) return this.cloudIds[h2.index] ?? null;
    }
    return null;
  }

  private bindEvents() {
    const cv = this.renderer.domElement;
    cv.tabIndex = 0;
    cv.setAttribute('aria-label', '3D brain view. Drag to rotate, wheel to zoom, click a node to inspect it.');
    cv.addEventListener('contextmenu', (e) => e.preventDefault());
    cv.addEventListener('pointerdown', (e) => {
      this.drag = { x: e.clientX, y: e.clientY, sx: e.clientX, sy: e.clientY, pan: this.flat() || e.button === 2 || e.shiftKey };
      try {
        cv.setPointerCapture(e.pointerId);
      } catch {
        /* ignore */
      }
    });
    cv.addEventListener('pointermove', (e) => {
      if (this.drag) {
        const dx = e.clientX - this.drag.x,
          dy = e.clientY - this.drag.y;
        this.drag.x = e.clientX;
        this.drag.y = e.clientY;
        if (this.drag.pan) {
          const cam = this.cam();
          const right = new THREE.Vector3().setFromMatrixColumn(cam.matrix, 0),
            up = new THREE.Vector3().setFromMatrixColumn(cam.matrix, 1);
          const h = Math.max(1, this.host.clientHeight);
          const k = this.flat() ? (2 * this.orbit.r) / h : this.orbit.r * 0.0016;
          this.orbit.target.addScaledVector(right, -dx * k).addScaledVector(up, dy * k);
          this.orbit.goal = null;
        } else {
          this.orbit.th -= dx * 0.006;
          this.orbit.ph = clamp(this.orbit.ph - dy * 0.006, 0.12, 3.02);
        }
        return;
      }
      const id = this.pick(e);
      this.hover = id;
      const hc = id && id.startsWith('H:') && this.C.has(id) ? id : null;
      if (hc !== this.hoverConn) {
        this.hoverConn = hc;
        this.paint();
      }
      this.showTip(id, e);
    });
    cv.addEventListener('pointerleave', () => {
      this.hover = null;
      this.tipEl.style.display = 'none';
    });
    cv.addEventListener('pointerup', (e) => {
      const moved = this.drag && Math.hypot(e.clientX - this.drag.sx, e.clientY - this.drag.sy) > 5;
      this.drag = null;
      if (moved) return;
      const id = this.pick(e);
      if (id) this.cb.onSelect(id);
    });
    cv.addEventListener('dblclick', (e) => {
      const id = this.pick(e);
      if (id) this.cb.onActivate(id);
    });
    cv.addEventListener(
      'wheel',
      (e) => {
        e.preventDefault();
        this.orbit.r = this.zoomLimit(this.orbit.r * Math.pow(1.0015, e.deltaY));
      },
      { passive: false },
    );
    cv.addEventListener('keydown', (e) => {
      if (this.flat()) {
        const step = (e.shiftKey ? 0.14 : 0.07) * this.orbit.r;
        const right = new THREE.Vector3().setFromMatrixColumn(this.ortho.matrix, 0),
          up = new THREE.Vector3().setFromMatrixColumn(this.ortho.matrix, 1);
        if (e.key === 'ArrowLeft') this.orbit.target.addScaledVector(right, -step);
        else if (e.key === 'ArrowRight') this.orbit.target.addScaledVector(right, step);
        else if (e.key === 'ArrowUp') this.orbit.target.addScaledVector(up, step);
        else if (e.key === 'ArrowDown') this.orbit.target.addScaledVector(up, -step);
        else if (e.key === '+' || e.key === '=') this.orbit.r = this.zoomLimit(this.orbit.r * 0.9);
        else if (e.key === '-') this.orbit.r = this.zoomLimit(this.orbit.r * 1.1);
        else return;
        this.orbit.goal = null;
        e.preventDefault();
        return;
      }
      const step = 0.08;
      if (e.key === 'ArrowLeft') this.orbit.th += step;
      else if (e.key === 'ArrowRight') this.orbit.th -= step;
      else if (e.key === 'ArrowUp') this.orbit.ph = clamp(this.orbit.ph - step, 0.12, 3.02);
      else if (e.key === 'ArrowDown') this.orbit.ph = clamp(this.orbit.ph + step, 0.12, 3.02);
      else if (e.key === '+' || e.key === '=') this.orbit.r = this.zoomLimit(this.orbit.r * 0.9);
      else if (e.key === '-') this.orbit.r = this.zoomLimit(this.orbit.r * 1.1);
      else return;
      e.preventDefault();
    });
  }

  private showTip(id: string | null, e: PointerEvent) {
    const tip = this.tipEl;
    const p = this.p;
    if (!id || !p) {
      tip.style.display = 'none';
      return;
    }
    const co = this.C.get(id);
    if (co) {
      const rc0 = this.renderer.domElement.getBoundingClientRect();
      tip.style.display = 'block';
      tip.style.left = e.clientX - rc0.left + 14 + 'px';
      tip.style.top = e.clientY - rc0.top + 10 + 'px';
      tip.replaceChildren();
      const h = document.createElement('div');
      const b = document.createElement('b');
      b.textContent = `⧉ ${co.c.type} · ${co.c.label}`;
      h.append(b);
      const meta = document.createElement('div');
      meta.className = 'muted small';
      meta.textContent = `${co.c.id}${co.c.key ? ' · key ' + co.c.key : ''} · gain ${co.c.gain} · ${co.c.transparent ? 'transparent' : 'opaque'} · joins ${co.members.length}`;
      tip.append(h, meta);
      return;
    }
    const o = this.P.get(id);
    if (!o) return;
    const n = o.node;
    const rc = this.renderer.domElement.getBoundingClientRect();
    tip.style.display = 'block';
    tip.style.left = e.clientX - rc.left + 14 + 'px';
    tip.style.top = e.clientY - rc.top + 10 + 'px';
    tip.replaceChildren();
    const head = document.createElement('div');
    const badge = document.createElement('span');
    badge.className = 'kind-badge';
    badge.style.background = this.nodeColor(n);
    badge.textContent = n.external ? `${n.source ?? 'ext'} · ${p.kindLabel(n.kind)}` : p.kindLabel(n.kind);
    const idEl = document.createElement('b');
    idEl.className = 'mono';
    idEl.textContent = ' ' + n.id;
    head.append(badge, idEl);
    if (n.supersededBy) head.append(` · superseded → ${n.supersededBy}`);
    const txt = document.createElement('div');
    txt.style.marginTop = '4px';
    txt.textContent = n.text;
    const meta = document.createElement('div');
    meta.className = 'muted small';
    const lit = n.activation !== undefined && n.activation !== null;
    meta.textContent = `w=${n.weight.toFixed(2)} · ${lit ? `act=${n.activation!.toFixed(2)} · depth ${n.depth}` : 'outside context'}`;
    tip.append(head, txt, meta);
  }

  // ------------------------------------------------------------------ labels
  private annotations(): { t: string; p: Vec3 }[] {
    const p = this.p!;
    switch (p.view) {
      case 'brain':
        return Object.entries(LOBES)
          .map(([k, v]) => ({ t: k, p: [v[0] * 1.55, v[1] * 1.35 + 10, v[2] * 1.2] as Vec3 }))
          .concat([{ t: 'concept core', p: [0, 34, 0] }]);
      case 'layers':
        return (
          [
            ['concepts', 80],
            ['decisions · summaries', 40],
            ['facts · files', 0],
            ['tasks · questions · ideas', -40],
            ['notes', -80],
          ] as [string, number][]
        ).map(([t, y]) => ({ t, p: [-135, y, -135] }));
      case 'sphere':
        return [1, 2, 3, 4, 5, 6]
          .filter((d) => d <= p.depth)
          .map((d) => ({ t: 'depth ' + d, p: [d * 30, 3, 0] as Vec3 }))
          .concat([
            { t: 'focus', p: [0, 14, 0] },
            { t: 'memory outside context', p: [0, 190, 0] },
          ]);
      case 'embed':
        return [
          { t: 'PC1', p: [130, 0, 0] },
          { t: 'PC2', p: [0, 108, 0] },
          { t: 'PC3', p: [0, 0, 130] },
          { t: (p.embeddingUse === 'raw' ? 'raw' : 'graph-enriched') + ' space', p: [0, 130, 0] },
        ];
      case 'tree':
        return [
          { t: 'root', p: [0, 104, 0] },
          { t: 'outside the tree', p: [170, -128, 0] },
        ];
      case 'time':
        return (
          [
            [0, 'today'],
            [7, '7 days'],
            [30, '30 days'],
            [100, '100 days'],
            [300, '300 days'],
          ] as [number, string][]
        )
          .map(([a, t]) => ({ t, p: [timeX(a), -52, 62] as Vec3 }))
          .concat([
            { t: '↑ weight', p: [-105, 56, 0] },
            { t: 'concepts (no decay)', p: [120, 60, 0] },
          ]);
      case 'map':
        return this.mapNotes;
    }
  }

  private updLabels() {
    const p = this.p;
    if (!p) return;
    const items: { cls: string; t: string; p: THREE.Vector3 }[] = [];
    const W = this.host.clientWidth,
      H = this.host.clientHeight;
    for (const a of this.annotations()) items.push({ cls: 'ann', t: a.t, p: new THREE.Vector3(...a.p) });
    const lit = [...this.objs.values()]
      .filter((o) => o.node.activation !== undefined && o.node.activation !== null)
      .sort((a, b) => (b.node.activation ?? 0) - (a.node.activation ?? 0))
      .slice(0, 8)
      .map((o) => o.node.id);
    // (on big graphs objs holds only featured nodes, so only lit nodes get labels)
    const show = new Set([...p.focus, ...lit]);
    if (p.selected) show.add(p.selected);
    if (this.hover) show.add(this.hover);
    const sources = new Set<string>();
    for (const o of this.objs.values()) if (o.node.external) sources.add(o.node.source ?? o.node.id.split(':')[0]);
    for (const id of show) {
      const o = this.P.get(id);
      if (!o) continue;
      items.push({
        cls: id === p.selected ? 'sel' : p.focus.includes(id) ? 'seed' : '',
        t: short(isConceptNode(o.node) ? '◆ ' + o.node.text : o.node.text),
        p: o.cur.clone(),
      });
    }
    for (const cid of [this.hoverConn, p.selected]) {
      const co = cid ? this.C.get(cid) : undefined;
      if (co && p.showConnectors) items.push({ cls: 'conn', t: '⧉ ' + co.c.label, p: co.cur.clone() });
    }
    // source satellite captions
    const ext = [...this.objs.values()].filter((o) => o.node.external);
    for (const s of sources) {
      const members = ext.filter((o) => (o.node.source ?? o.node.id.split(':')[0]) === s);
      if (!members.length) continue;
      const c = new THREE.Vector3();
      members.forEach((o) => c.add(o.cur));
      c.multiplyScalar(1 / members.length);
      c.y += 30;
      items.push({ cls: 'ann src', t: 'source ' + s, p: c });
    }
    while (this.labelPool.length < items.length) {
      const d = document.createElement('div');
      this.labelsEl.appendChild(d);
      this.labelPool.push(d);
    }
    this.labelPool.forEach((d, i) => {
      const it = items[i];
      if (!it) {
        d.style.display = 'none';
        return;
      }
      it.p.project(this.cam());
      if (it.p.z > 1) {
        d.style.display = 'none';
        return;
      }
      d.style.display = 'block';
      d.className = 'lbl ' + it.cls;
      if (d.textContent !== it.t) d.textContent = it.t;
      d.style.left = ((it.p.x + 1) / 2) * W + 'px';
      d.style.top = ((1 - it.p.y) / 2) * H + 'px';
    });
  }

  // ------------------------------------------------------------------ loop
  private frame = (now: number) => {
    if (this.disposed) return;
    const dt = Math.min(0.05, (now - this.last) / 1000);
    this.last = now;
    const p = this.p;
    if (p?.autoRotate && p.view !== 'map' && !this.drag) this.orbit.th += dt * 0.08;
    const fog = this.scene.fog as THREE.FogExp2 | null;
    if (fog) fog.density = p?.view === 'map' ? 0 : 0.0022;
    if (this.orbit.goal) {
      this.orbit.target.lerp(this.orbit.goal, 0.07);
      if (this.orbit.target.distanceTo(this.orbit.goal) < 0.3) this.orbit.goal = null;
    }
    this.updCam();
    const t = now / 1000;
    for (const pn of this.P.values()) pn.cur.lerp(pn.tgt, 0.085);
    if (this.cloud) {
      const pos = (this.cloud.geometry.getAttribute('position') as THREE.BufferAttribute).array as Float32Array;
      this.cloudIds.forEach((id, i) => {
        const c = this.P.get(id)?.cur;
        if (c) pos.set([c.x, c.y, c.z], i * 3);
      });
      (this.cloud.geometry.getAttribute('position') as THREE.BufferAttribute).needsUpdate = true;
    }
    for (const o of this.objs.values()) {
      o.mesh.position.copy(o.cur);
      o.glow.position.copy(o.cur);
      if (o.node.external && p?.view !== 'map') o.mesh.rotation.y += dt * 0.6;
      if (o.glow.visible) o.glow.scale.setScalar(o.gs * (1 + 0.09 * Math.sin(t * 2.6 + o.phase)));
    }
    if (this.C.size && this.p?.showConnectors) {
      for (const o of this.C.values()) {
        const tgt = new THREE.Vector3();
        o.members.forEach((m) => tgt.add(this.P.get(m)?.cur ?? tgt));
        if (o.members.length) tgt.multiplyScalar(1 / o.members.length);
        tgt.y += 6;
        o.cur.lerp(tgt, 0.12);
        o.mesh.position.copy(o.cur);
        o.mesh.lookAt(this.cam().position);
      }
      if (this.connLines) {
        const pos = (this.connLines.geometry.getAttribute('position') as THREE.BufferAttribute).array as Float32Array;
        this.connEdges.forEach(([h, m], i) => {
          const A = this.C.get(h)?.cur,
            B = this.P.get(m)?.cur;
          if (A && B) pos.set([A.x, A.y, A.z, B.x, B.y, B.z], i * 6);
        });
        (this.connLines.geometry.getAttribute('position') as THREE.BufferAttribute).needsUpdate = true;
        this.connLines.computeLineDistances();
      }
    }
    const sm = this.shell.material as THREE.PointsMaterial;
    sm.opacity += (this.shellTarget - sm.opacity) * 0.08;
    this.shell.visible = sm.opacity > 0.01;
    if (this.edgeLines) {
      const pos = (this.edgeLines.geometry.getAttribute('position') as THREE.BufferAttribute).array as Float32Array;
      this.edgeList.forEach((e, i) => {
        const A = this.P.get(e.a)?.cur,
          B = this.P.get(e.b)?.cur;
        if (A && B) pos.set([A.x, A.y, A.z, B.x, B.y, B.z], i * 6);
      });
      (this.edgeLines.geometry.getAttribute('position') as THREE.BufferAttribute).needsUpdate = true;
    }
    for (let i = this.pulses.length - 1; i >= 0; i--) {
      const pu = this.pulses[i];
      pu.t += dt;
      if (pu.t < 0) {
        pu.sp.visible = false;
        continue;
      }
      if (pu.t > pu.dur) {
        if (pu.loop) {
          pu.t -= pu.period;
          pu.sp.visible = false;
          continue;
        }
        this.scene.remove(pu.sp);
        pu.sp.material.dispose();
        this.pulses.splice(i, 1);
        continue;
      }
      const A = this.P.get(pu.from),
        B = this.P.get(pu.to);
      if (!A || !B) continue;
      pu.sp.visible = true;
      pu.sp.position.copy(A.cur).lerp(B.cur, pu.t / pu.dur);
    }
    const sel = p?.selected ? (this.objs.get(p.selected) ?? (p.showConnectors ? this.C.get(p.selected) : undefined)) : undefined;
    if (sel) {
      this.selRing.visible = true;
      this.selRing.position.copy(sel.cur);
      this.selRing.scale.setScalar(sel.mesh.scale.x * 3.2 + 3);
    } else this.selRing.visible = false;
    this.renderer.render(this.scene, this.cam());
    this.updLabels();
    this.raf = requestAnimationFrame(this.frame);
  };

  private resize() {
    const w = Math.max(1, this.host.clientWidth),
      h = Math.max(1, this.host.clientHeight);
    this.renderer.setSize(w, h);
    this.camera.aspect = w / h;
    this.camera.updateProjectionMatrix();
  }

  dispose() {
    this.disposed = true;
    cancelAnimationFrame(this.raf);
    this.ro.disconnect();
    this.clearPulses();
    for (const id of [...this.objs.keys()]) this.dropObj(id);
    this.big = false;
    this.rebuildCloud([]);
    this.edgeLines?.geometry.dispose();
    this.renderer.dispose();
    this.renderer.domElement.remove();
    this.labelPool.forEach((d) => d.remove());
  }
}
