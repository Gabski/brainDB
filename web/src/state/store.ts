// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

// App store: one snapshot of the brain (State) plus UI state. Every brain mutation goes through the
// ApiClient; after a mutation (or an SSE "changed" event) the State is refetched.
import type { ApiClient, LiveStatus } from '../api/client';
import { ApiError } from '../api/client';
import type {
  BrainEvent,
  ContextPatch,
  Discover,
  EmbeddingPatch,
  ExecOp,
  IngestResult,
  Inspect,
  InspectPath,
  InspectTest,
  KindInput,
  LayoutResult,
  QueryFormat,
  QueryResult,
  SearchMode,
  SearchResult,
  Source,
  SourceInput,
  State,
  Suggestion,
  Template,
  VectorResult,
  RunResult,
  BdqlError,
  DbInfo,
  Health,
  ServerStats,
  EntityRow,
  Whoami,
  Purpose,
  QuarantineItem,
} from '../api/types';
import { statementAt } from '../model/highlight';
import { setToken } from '../api/token';
import type { ColorMode, EdgesMode, PulseBatch, PulseItem, Theme } from '../components/graph/sceneShared';
import { buildIndex, depthColor, edgeKey, type GraphIndex } from '../model/graph';
import { DEFAULT_BROWSE, type BrowseQuery } from '../model/browse';
import { EMPTY_NARROW, type GroupBy, type ResultNarrow } from '../model/searchView';
import type { ViewName } from '../model/layout';
import { clamp } from '../model/util';

export interface Async<T> {
  data?: T;
  loading: boolean;
  error?: string;
}
const idle = <T,>(): Async<T> => ({ loading: false });

export type TabId = 'query' | 'context' | 'template' | 'graph' | 'browse' | 'search' | 'schema' | 'sources' | 'activity' | 'xml' | 'import' | 'settings' | 'access' | 'review';
export const TAB_LABELS: Record<TabId, string> = {
  query: 'Query',
  context: 'Context',
  template: 'Template',
  graph: 'Graph 3D',
  browse: 'Browse',
  search: 'Search',
  schema: 'Schema',
  sources: 'Sources',
  activity: 'Activity',
  xml: 'Brain XML',
  import: 'Import',
  settings: 'Settings',
  access: 'Access',
  review: 'Review queue',
};

export type ToolOutput =
  | { type: 'test'; id: string; data: InspectTest }
  | { type: 'suggest'; id: string; data: Suggestion[] }
  | { type: 'paths'; from: string; to: string; data: InspectPath[] }
  | { type: 'message'; title: string; text: string; ok?: boolean };

export type PickAction = 'path' | 'link' | 'supersede';
export interface PickMode {
  action: PickAction;
  from: string;
  label: string;
}

export interface HistoryEntry {
  text: string;
  format: QueryFormat;
  ts: number;
  ms?: number;
  rows?: number;
  ok: boolean;
}
export interface RecentSearch {
  query: string;
  mode: SearchMode;
  ts: number;
  rows?: number;
  ms?: number;
}
export interface SavedQuery {
  name: string;
  text: string;
}

export interface AccessRows {
  columns: string[];
  rows: Record<string, unknown>[];
}
export interface AccessData {
  users: AccessRows;
  roles: AccessRows;
  grants: AccessRows;
  labels: AccessRows;
}

export interface UiState {
  brain: State | null;
  index: GraphIndex | null;
  loading: boolean;
  error: string | null;
  revision: number;
  theme: Theme;
  tabs: TabId[];
  activeTab: TabId;
  view: ViewName;
  edgesMode: EdgesMode;
  colorMode: ColorMode;
  autoRotate: boolean;
  /** null = automatic: on in every Graph view except Brain. */
  showConnectors: boolean | null;
  selected: string | null;
  /** Visited-node history for the Inspector's back / forward. */
  nav: { back: string[]; fwd: string[] };
  details: Async<Inspect>;
  vector: Async<VectorResult>;
  tool: ToolOutput | null;
  toolLoading: boolean;
  pickMode: PickMode | null;
  linkWeight: number;
  /** Browse tab: facet filters, sort and free text. */
  browse: BrowseQuery;
  search: {
    query: string;
    mode: SearchMode;
    sources: string[];
    result: SearchResult | null;
    loading: boolean;
    error?: string;
    ms?: number;
    /** How many hits to ask the server for. */
    k: number;
    /** Client-side narrowing of the hits that came back. */
    narrow: ResultNarrow;
    group: GroupBy;
  };
  /** Recent searches, newest first (persisted). */
  searches: RecentSearch[];
  ingest: Async<IngestResult>;
  ingestFocus: boolean;
  discover: Async<Discover>;
  highlight: string[];
  agentHighlight: string[];
  pathEdges: [string, string][];
  pulses: PulseBatch;
  flyTo: { id: string; token: number } | null;
  treeRoot: string | null;
  layouts: { embed?: LayoutResult; tree?: LayoutResult; loading: boolean; error?: string };
  feed: BrainEvent[];
  live: LiveStatus;
  sources: Async<Source[]>;
  templates: Async<Template[]>;
  templateDraft: { content: string; format: string; name?: string; token: number } | null;
  bdql: { script: string; dryRun: boolean; result: RunResult | null; loading: boolean; error?: string; ms?: number; active: number; lint: BdqlError[]; mode: 'bdql' | 'compact' };
  query: { text: string; format: QueryFormat; sources: string[]; result: QueryResult | null; loading: boolean; error?: string; ms?: number };
  history: HistoryEntry[];
  saved: SavedQuery[];
  lastOp: { name: string; ms: number; ok: boolean; ts: number; error?: string } | null;
  needToken: boolean;
  dbs: Async<DbInfo[]>;
  db: string | null;
  health: Health | null;
  serverStats: ServerStats | null;
  /** undefined = not requested, null = loading */
  entityRows: EntityRow[] | null | undefined;
  wizardOpen: boolean;
  importProgress: BrainEvent | null;
  whoami: Whoami | null;
  viewAs: string | null;
  accessOverlay: boolean;
  access: Async<AccessData>;
  purpose: Async<Purpose | null>;
  quarantine: Async<QuarantineItem[]>;
  aboutOpen: boolean;
  paletteOpen: boolean;
  toast: { text: string; ok: boolean; token: number } | null;
}

const SAVED_KEY = 'braindb.savedQueries';
const SEARCHES_KEY = 'braindb.recentSearches';
const HISTORY_KEY = 'braindb.queryHistory';
const THEME_KEY = 'braindb.theme';

function loadLocal<T>(key: string, fallback: T): T {
  try {
    const v = window.localStorage.getItem(key);
    return v ? (JSON.parse(v) as T) : fallback;
  } catch {
    return fallback;
  }
}
function saveLocal(key: string, v: unknown) {
  try {
    window.localStorage.setItem(key, JSON.stringify(v));
  } catch {
    /* storage unavailable */
  }
}

export const DEFAULT_BDQL = `-- BDQL: Ctrl+Enter runs the statement at the cursor, Ctrl+Shift+Enter runs all, Ctrl+Space completes
SELECT id, kind, text, fields.users
  FROM memory
  WHERE kind = record
  ORDER BY fields.users DESC
  LIMIT 10;

CONTEXT FOR "guest access" DEPTH 1 FORMAT md;
`;
export const DEFAULT_QUERY = 'kind:record sort:-users limit:20 select:id,kind,text,weight,fields.wave,fields.users,fields.owner';

export function initialUiState(over: Partial<UiState> = {}): UiState {
  const theme = loadLocal<Theme>(THEME_KEY, 'dark');
  const base: UiState = {
    brain: null,
    index: null,
    loading: false,
    error: null,
    revision: 0,
    theme: theme === 'light' ? 'light' : 'dark',
    tabs: ['graph', 'browse', 'query', 'context', 'template', 'search', 'schema', 'sources', 'activity'],
    activeTab: 'graph',
    view: 'brain',
    edgesMode: 'all',
    colorMode: 'kind',
    autoRotate: true,
    showConnectors: null,
    selected: null,
    nav: { back: [], fwd: [] },
    details: idle(),
    vector: idle(),
    tool: null,
    toolLoading: false,
    pickMode: null,
    linkWeight: 0.6,
    browse: { ...DEFAULT_BROWSE },
    search: { query: '', mode: 'hybrid', sources: [], result: null, loading: false, k: 8, narrow: { ...EMPTY_NARROW }, group: 'none' },
    searches: loadLocal<RecentSearch[]>(SEARCHES_KEY, []),
    ingest: idle(),
    ingestFocus: true,
    discover: idle(),
    highlight: [],
    agentHighlight: [],
    pathEdges: [],
    pulses: { token: 0, items: [] },
    flyTo: null,
    treeRoot: null,
    layouts: { loading: false },
    feed: [],
    live: 'connecting',
    sources: idle(),
    templates: idle(),
    templateDraft: null,
    bdql: { script: DEFAULT_BDQL, dryRun: false, result: null, loading: false, active: 0, lint: [], mode: 'bdql' },
    query: { text: DEFAULT_QUERY, format: 'md', sources: [], result: null, loading: false },
    history: loadLocal<HistoryEntry[]>(HISTORY_KEY, []),
    saved: loadLocal<SavedQuery[]>(SAVED_KEY, []),
    lastOp: null,
    needToken: false,
    dbs: idle(),
    db: null,
    health: null,
    serverStats: null,
    entityRows: undefined,
    wizardOpen: false,
    importProgress: null,
    whoami: null,
    viewAs: null,
    accessOverlay: false,
    access: idle(),
    purpose: idle(),
    quarantine: idle(),
    aboutOpen: false,
    paletteOpen: false,
    toast: null,
  };
  const s = { ...base, ...over };
  if (s.brain && !s.index) s.index = buildIndex(s.brain);
  return s;
}

const errMsg = (e: unknown) => (e instanceof Error ? e.message : String(e));

export class BrainStore {
  private s: UiState;
  private listeners = new Set<() => void>();
  private unsub: (() => void) | null = null;
  private ctxTimer: ReturnType<typeof setTimeout> | null = null;
  private ctxPending: ContextPatch = {};
  private embTimer: ReturnType<typeof setTimeout> | null = null;
  private embPending: EmbeddingPatch = {};
  private pulseToken = 1;
  private refreshSeq = 0;

  constructor(
    public api: ApiClient,
    initial: Partial<UiState> = {},
  ) {
    this.s = initialUiState(initial);
  }

  // ------------------------------------------------------------------ subscription plumbing
  get = () => this.s;
  subscribe = (fn: () => void) => {
    this.listeners.add(fn);
    return () => {
      this.listeners.delete(fn);
    };
  };
  set(patch: Partial<UiState> | ((s: UiState) => Partial<UiState>)) {
    const p = typeof patch === 'function' ? patch(this.s) : patch;
    this.s = { ...this.s, ...p };
    this.listeners.forEach((l) => l());
  }

  /** Wraps an API call: records latency in the status bar, shows errors as a toast, flags 401. */
  private async op<T>(name: string, fn: () => Promise<T>, quiet = false): Promise<T | undefined> {
    const t0 = performance.now();
    try {
      const r = await fn();
      this.set({ lastOp: { name, ms: Math.round(performance.now() - t0), ok: true, ts: Date.now() } });
      return r;
    } catch (e) {
      const msg = errMsg(e);
      const unauthorized = e instanceof ApiError && e.status === 401;
      const denied = /access denied/i.test(msg) || (e instanceof ApiError && e.status === 403);
      this.set({
        lastOp: { name, ms: Math.round(performance.now() - t0), ok: false, ts: Date.now(), error: msg },
        ...(unauthorized ? { needToken: true } : {}),
        ...(quiet || unauthorized ? {} : { toast: { text: denied ? `Write denied (${name}): ${msg}` : `${name}: ${msg}`, ok: false, token: Date.now() } }),
        ...(denied ? { tool: { type: 'message' as const, title: 'Write denied', text: msg + '\nYou can read this item but your grants do not allow changing it.', ok: false } } : {}),
      });
      return undefined;
    }
  }

  // ------------------------------------------------------------------ lifecycle
  async init() {
    this.set({ loading: true, error: null });
    const st = await this.op('state', () => this.api.getState(), true);
    if (st) this.setBrain(st);
    else this.set({ loading: false, error: this.s.lastOp?.error ?? 'cannot load state' });
    this.api
      .journal()
      .then((j) => this.set({ feed: [...j].reverse().slice(0, 200) }))
      .catch(() => {});
    this.loadSources();
    this.loadTemplates();
    this.loadDbs();
    this.api
      .whoami()
      .then((w) => {
        this.set({ whoami: w });
        if (isReviewer(w)) this.loadQuarantine();
      })
      .catch(() => {});
    this.loadPurpose();
    this.api
      .health()
      .then((h) => this.set({ health: h }))
      .catch(() => {});
    this.api
      .stats()
      .then((x) => this.set({ serverStats: x }))
      .catch(() => {});
    this.connectLive();
  }

  async loadDbs() {
    const r = await this.op('dbs', () => this.api.dbs(), true);
    const cur = this.api.getDb();
    this.set({ dbs: r ? { data: r, loading: false } : { loading: false, error: this.s.lastOp?.error }, db: cur ?? r?.[0]?.name ?? this.s.db });
  }
  /** Switch the active database: every call then carries X-BrainDB-Database / ?db=. */
  async switchDb(name: string) {
    this.api.setDb(name);
    this.set({ db: name, selected: null, nav: { back: [], fwd: [] }, tool: null, search: { ...this.s.search, result: null }, bdql: { ...this.s.bdql, result: null }, entityRows: undefined, layouts: { loading: false } });
    await this.init();
  }
  async loadPurpose() {
    this.set((s) => ({ purpose: { ...s.purpose, loading: true } }));
    const r = await this.op('purpose', () => this.api.getPurpose(), true);
    this.set((s) => ({ purpose: r !== undefined ? { data: r, loading: false } : { data: s.purpose.data, loading: false, error: s.lastOp?.error } }));
  }
  async savePurpose(p: Purpose) {
    const r = await this.op('save purpose', () => this.api.putPurpose(p));
    if (r) {
      this.set({ purpose: { data: r, loading: false } });
      this.toastMsg('Purpose saved');
    }
  }
  async loadQuarantine() {
    this.set((s) => ({ quarantine: { ...s.quarantine, loading: true } }));
    const r = await this.op('quarantine', () => this.api.quarantine(), true);
    this.set((s) => ({ quarantine: r ? { data: r, loading: false } : { data: s.quarantine.data, loading: false, error: s.lastOp?.error } }));
  }
  async admitItems(ids: string[], section?: string) {
    const r = await this.op('admit', () => this.api.admit(ids, section));
    if (r !== undefined) {
      this.toastMsg(`Admitted ${ids.join(', ')}${section ? ' into ' + section : ''}`);
      await Promise.all([this.loadQuarantine(), this.refresh()]);
    }
  }
  async rejectItems(ids: string[], reason: string) {
    const r = await this.op('reject', () => this.api.reject(ids, reason));
    if (r !== undefined) {
      this.toastMsg(`Rejected ${ids.join(', ')}: ${reason}`);
      await this.loadQuarantine();
    }
  }
  /** Admin preview of another user's view (null = back to self). */
  async setViewAs(user: string | null) {
    this.api.setViewAs(user);
    this.set({ viewAs: user, selected: null, nav: { back: [], fwd: [] }, tool: null, accessOverlay: false, entityRows: undefined });
    await this.init();
  }
  /** Admin: users, roles, grants and labels through BDQL SHOW statements. */
  async loadAccess() {
    this.set({ access: { ...this.s.access, loading: true, error: undefined } });
    try {
      const pick = async (what: string): Promise<AccessRows> => {
        const r = await this.api.run(`SHOW ${what};`);
        if (r.error) throw new Error(r.error.message);
        const x = r.results.find((y) => y.kind === 'rows');
        return { columns: x?.columns ?? [], rows: x?.rows ?? [] };
      };
      const [users, roles, grants, labels] = await Promise.all(['users', 'roles', 'grants', 'labels'].map(pick));
      this.set({ access: { data: { users, roles, grants, labels }, loading: false } });
    } catch (e) {
      this.set({ access: { loading: false, error: errMsg(e) } });
    }
  }
  async loadEntities() {
    this.set({ entityRows: null });
    const r = await this.op('entities', () => this.api.entities({ limit: 2000 }), true);
    this.set({ entityRows: r ?? [] });
  }

  connectLive() {
    this.unsub?.();
    this.unsub = this.api.subscribe(
      (ev) => this.onEvent(ev),
      (live) => this.set({ live }),
    );
  }

  dispose() {
    this.unsub?.();
    this.unsub = null;
  }

  private setBrain(st: State) {
    const sel = this.s.selected && st.nodes.some((n) => n.id === this.s.selected) ? this.s.selected : null;
    this.set((s) => ({ brain: st, index: buildIndex(st), loading: false, error: null, revision: s.revision + 1, selected: sel }));
    if (this.s.view === 'embed' || this.s.view === 'tree') this.loadLayout(this.s.view);
    if (sel) this.loadDetails(sel, false);
  }

  async refresh() {
    const seq = ++this.refreshSeq;
    const st = await this.op('state', () => this.api.getState(), true);
    if (st && seq === this.refreshSeq) this.setBrain(st);
  }

  private onEvent(ev: BrainEvent) {
    if (ev.type === 'hello') return;
    if (ev.type === 'import.progress') {
      this.set((s) => ({ feed: [ev, ...s.feed].slice(0, 200), importProgress: ev }));
      return;
    }
    this.set((s) => ({ feed: [ev, ...s.feed].slice(0, 200) }));
    this.refresh().then(() => {
      if (ev.by === 'ui') return;
      const idx = this.s.index;
      const ids = (ev.ids ?? []).filter((id) => idx?.nodes.has(id));
      if (!ids.length) return;
      const items: PulseItem[] = [];
      ids.slice(0, 12).forEach((id, i) => {
        const via = idx!.nodes.get(id)?.via;
        if (via) items.push({ from: via, to: id, delay: i * 0.15, dur: 0.6, color: '#3bc9db', size: 6 });
      });
      this.set({ agentHighlight: ids, pulses: { token: this.pulseToken++, items } });
    });
  }

  // ------------------------------------------------------------------ token
  async submitToken(token: string) {
    setToken(token);
    this.set({ needToken: false });
    await this.init();
  }

  // ------------------------------------------------------------------ UI-only setters
  setTheme(theme: Theme) {
    saveLocal(THEME_KEY, theme);
    this.set({ theme });
  }
  openTab(tab: TabId) {
    this.set((s) => ({ tabs: s.tabs.includes(tab) ? s.tabs : [...s.tabs, tab], activeTab: tab }));
  }
  closeTab(tab: TabId) {
    this.set((s) => {
      const tabs = s.tabs.filter((t) => t !== tab);
      const i = s.tabs.indexOf(tab);
      return { tabs, activeTab: s.activeTab === tab ? (tabs[Math.max(0, i - 1)] ?? tabs[0] ?? 'graph') : s.activeTab };
    });
  }
  setView(view: ViewName) {
    this.set({ view });
    if (view === 'embed' || view === 'tree') this.loadLayout(view);
  }
  setDisplay(p: Partial<Pick<UiState, 'edgesMode' | 'colorMode' | 'autoRotate' | 'linkWeight' | 'ingestFocus' | 'showConnectors'>>) {
    this.set(p);
  }
  /** Browse tab filters; loads the entity index the first time an entity facet is used. */
  setBrowse(p: Partial<BrowseQuery>) {
    this.set((s) => ({ browse: { ...s.browse, ...p } }));
    if (p.entity && this.s.entityRows === undefined) void this.loadEntities();
  }
  setPalette(open: boolean) {
    this.set({ paletteOpen: open });
  }
  dismissToast() {
    this.set({ toast: null });
  }
  toastMsg(text: string, ok = true) {
    this.set({ toast: { text, ok, token: Date.now() } });
  }

  /** Esc: cancel pick mode, clear path/highlights/pulses. */
  cancel() {
    this.set({ pickMode: null, pathEdges: [], highlight: [], agentHighlight: [], pulses: { token: this.pulseToken++, items: [] } });
  }

  // ------------------------------------------------------------------ selection + inspector
  select(id: string | null, fly = false) {
    const pm = this.s.pickMode;
    if (pm && id) {
      this.set({ pickMode: null });
      this.completePick(pm, id);
      return;
    }
    const changed = id !== this.s.selected;
    const prev = this.s.selected;
    if (changed && prev && this.navMode === 'push') this.set((x) => ({ nav: { back: [...x.nav.back, prev].slice(-50), fwd: [] } }));
    this.set({
      selected: id,
      ...(changed ? { tool: null, pathEdges: [], vector: idle<VectorResult>() } : {}),
      ...(fly && id ? { flyTo: { id, token: Date.now() } } : {}),
    });
    if (changed) this.set({ pulses: { token: this.pulseToken++, items: [] } });
    if (id) this.loadDetails(id, true);
  }

  /** 'push' records history, 'replay' does not (used by back / forward themselves). */
  private navMode: 'push' | 'replay' = 'push';

  /** Step back to the previously inspected node. */
  navBack() {
    const prev = this.s.nav.back[this.s.nav.back.length - 1];
    if (!prev) return;
    const cur = this.s.selected;
    this.set((x) => ({ nav: { back: x.nav.back.slice(0, -1), fwd: cur ? [...x.nav.fwd, cur] : x.nav.fwd } }));
    this.navMode = 'replay';
    this.select(prev, true);
    this.navMode = 'push';
  }
  /** Step forward again after a back. */
  navForward() {
    const next = this.s.nav.fwd[this.s.nav.fwd.length - 1];
    if (!next) return;
    const cur = this.s.selected;
    this.set((x) => ({ nav: { back: cur ? [...x.nav.back, cur] : x.nav.back, fwd: x.nav.fwd.slice(0, -1) } }));
    this.navMode = 'replay';
    this.select(next, true);
    this.navMode = 'push';
  }

  async loadDetails(id: string, withVector: boolean) {
    this.set((s) => ({ details: { ...s.details, loading: true, error: undefined } }));
    const r = await this.op('inspect', () => this.api.inspect({ id, what: ['item', 'links', 'near'] }), true);
    if (this.s.selected !== id) return;
    this.set({ details: r ? { data: r, loading: false } : { loading: false, error: this.s.lastOp?.error } });
    if (withVector) {
      this.set({ vector: { loading: true } });
      const v = await this.op('vector', () => this.api.vector(id), true);
      if (this.s.selected === id) this.set({ vector: v ? { data: v, loading: false } : { loading: false, error: this.s.lastOp?.error } });
    }
  }

  // ------------------------------------------------------------------ context
  /** Debounced POST /api/context (sliders). */
  patchContext(p: ContextPatch, debounce = 0) {
    this.ctxPending = { ...this.ctxPending, ...p };
    if (this.s.brain) {
      // optimistic params so sliders feel immediate
      const params = { ...this.s.brain.params };
      if (p.depth !== undefined) params.depth = p.depth;
      if (p.threshold !== undefined) params.threshold = p.threshold;
      if (p.hop !== undefined) params.hop = p.hop;
      if (p.showSuperseded !== undefined) params.showSuperseded = p.showSuperseded;
      this.set({ brain: { ...this.s.brain, params } });
    }
    if (this.ctxTimer) clearTimeout(this.ctxTimer);
    this.ctxTimer = setTimeout(() => this.flushContext(), debounce);
  }
  private async flushContext() {
    const p = this.ctxPending;
    this.ctxPending = {};
    const st = await this.op('context', () => this.api.setContext(p));
    if (st) this.setBrain(st);
  }
  setFocus(ids: string[]) {
    this.patchContext({ focus: ids });
  }
  addFocus(id: string) {
    this.patchContext({ addFocus: [id] });
  }
  removeFocus(id: string) {
    this.patchContext({ removeFocus: [id] });
  }
  focusGoal() {
    this.patchContext({ goalFocus: true });
  }
  setGoal(goal: string) {
    this.patchContext({ goal });
  }

  // ------------------------------------------------------------------ embedding
  patchEmbedding(p: EmbeddingPatch, debounce = 250) {
    this.embPending = { ...this.embPending, ...p };
    if (this.s.brain) this.set({ brain: { ...this.s.brain, embedding: { ...this.s.brain.embedding, ...p } } });
    if (this.embTimer) clearTimeout(this.embTimer);
    this.embTimer = setTimeout(async () => {
      const q = this.embPending;
      this.embPending = {};
      const st = await this.op('embedding', () => this.api.setEmbedding(q));
      if (st) this.setBrain(st);
    }, debounce);
  }

  // ------------------------------------------------------------------ exec
  async exec(ops: ExecOp[], label?: string) {
    if (this.s.viewAs) {
      this.toastMsg(`Writes are disabled while viewing as ${this.s.viewAs}.`, false);
      return undefined;
    }
    const r = await this.op(label ?? 'exec ' + ops.map((o) => o.op).join('+'), () => this.api.exec(ops));
    if (r) await this.refresh();
    return r;
  }
  boost(id: string) {
    return this.exec([{ op: 'boost', id }]);
  }
  weaken(id: string) {
    return this.exec([{ op: 'weaken', id }]);
  }
  togglePin(id: string) {
    const n = this.s.index?.nodes.get(id);
    return this.exec([{ op: n?.pinned ? 'unpin' : 'pin', id }]);
  }
  forget(id: string) {
    return this.exec([{ op: 'forget', id }]).then((r) => {
      if (r && this.s.selected === id) this.select(null);
      return r;
    });
  }
  async addLink(a: string, b: string, w: number, type: 'new' | 'similar' | 'external' = 'new') {
    const r = await this.exec([{ op: 'link', a, b, w: +clamp(w).toFixed(2), type }], 'link');
    if (r) this.toastMsg(`Linked ${a} ↔ ${b} (w ${w.toFixed(2)})`);
    return r;
  }
  /** Change an edge weight by d (unlink + link in one atomic batch so it can go down too). */
  async adjustEdge(a: string, b: string, d: number) {
    const e = this.s.brain?.edges.find((x) => edgeKey(x.a, x.b) === edgeKey(a, b));
    if (!e) return;
    const w = +clamp(e.w + d, 0.05, 1).toFixed(2);
    await this.exec(
      [
        { op: 'unlink', a, b },
        { op: 'link', a, b, w, type: e.type },
      ],
      'edge weight',
    );
    if (this.s.tool?.type === 'test') this.testLinks(this.s.tool.id);
  }
  async removeEdge(a: string, b: string) {
    await this.exec([{ op: 'unlink', a, b }], 'unlink');
    if (this.s.tool?.type === 'test') this.testLinks(this.s.tool.id);
  }
  async remember(kind: string, text: string, concepts: string[], fields?: Record<string, string>) {
    const r = await this.exec([{ op: 'remember', kind, text, concepts, focus: true, autolink: true, tmp: 'new', ...(fields ? { fields } : {}) }], 'remember');
    if (r) {
      const id = r.touched.find((t) => !t.startsWith('A:')) ?? r.touched[0];
      this.set({ tool: { type: 'message', title: 'Memory added', text: `${r.changes.join('\n')}\nLinked to the chosen concepts and up to 3 best matches; added to focus.`, ok: true } });
      if (id) this.select(id, true);
    }
    return r;
  }
  async addKind(k: KindInput) {
    const st = await this.op('add kind', () => this.api.addKind(k));
    if (st) {
      this.setBrain(st);
      this.toastMsg(`Kind "${k.name}" added`);
    }
    return st;
  }

  // ------------------------------------------------------------------ pick mode (path to / link to / supersede by)
  startPick(action: PickAction, from: string) {
    const label = action === 'path' ? `Find paths from ${from}` : action === 'link' ? `New link from ${from}` : `Newer version that supersedes ${from}`;
    this.set({ pickMode: { action, from, label } });
  }
  private completePick(pm: PickMode, target: string) {
    if (target === pm.from) return;
    if (pm.action === 'path') this.showPaths(pm.from, target);
    else if (pm.action === 'link') this.addLink(pm.from, target, this.s.linkWeight);
    else this.exec([{ op: 'supersede', old: pm.from, new: target }], 'supersede');
  }

  // ------------------------------------------------------------------ analysis tools
  async testLinks(id: string) {
    this.set({ toolLoading: true });
    const r = await this.op('test links', () => this.api.inspect({ id, what: ['test'] }));
    this.set({ toolLoading: false });
    if (!r?.test) return;
    // replay: pulses from the node outwards, one hop per depth level
    const idx = this.s.index;
    const reached = new Map(r.test.reached.map((x) => [x.id, x]));
    reached.set(id, { id, act: 1, depth: 0 });
    const items: PulseItem[] = [];
    for (const x of r.test.reached) {
      const parent = (idx?.adj.get(x.id) ?? [])
        .map((e) => (e.a === x.id ? e.b : e.a))
        .filter((v) => reached.get(v)?.depth === x.depth - 1)
        .sort((a, b) => (reached.get(b)?.act ?? 0) - (reached.get(a)?.act ?? 0))[0];
      if (parent) items.push({ from: parent, to: x.id, delay: (x.depth - 1) * 0.55, dur: 0.55, color: depthColor(x.depth), size: 5 });
    }
    this.set({ tool: { type: 'test', id, data: r.test }, pulses: { token: this.pulseToken++, items } });
  }
  async suggest(id: string) {
    this.set({ toolLoading: true });
    const r = await this.op('suggest', () => this.api.inspect({ id, what: ['suggest'] }));
    this.set({ toolLoading: false });
    if (r) this.set({ tool: { type: 'suggest', id, data: r.suggest ?? [] } });
  }
  async showPaths(from: string, to: string) {
    this.set({ toolLoading: true });
    const r = await this.op('paths', () => this.api.inspect({ id: from, what: ['paths'], to }));
    this.set({ toolLoading: false });
    if (!r) return;
    const data = r.paths ?? [];
    this.set({ tool: { type: 'paths', from, to, data }, selected: from });
    if (data[0]) this.highlightPath(data[0].path);
  }
  highlightPath(path: string[]) {
    const edges: [string, string][] = [];
    const items: PulseItem[] = [];
    for (let i = 0; i < path.length - 1; i++) {
      edges.push([path[i], path[i + 1]]);
      items.push({ from: path[i], to: path[i + 1], delay: i * 0.45, dur: 0.45, color: '#ffd43b', loop: true, size: 7 });
    }
    this.set({ pathEdges: edges, pulses: { token: this.pulseToken++, items, period: (path.length - 1) * 0.45 + 0.8 } });
  }
  previewLink(u: string, v: string) {
    this.highlightPath([u, v]);
    this.set({ flyTo: { id: u, token: Date.now() } });
  }
  async discover() {
    this.set({ discover: { loading: true } });
    this.openTab('search');
    const r = await this.op('discover', () => this.api.inspect({ what: ['discover'] }));
    this.set({ discover: r?.discover ? { data: r.discover, loading: false } : { loading: false, error: this.s.lastOp?.error } });
  }
  setTreeRoot(id: string) {
    this.set({ treeRoot: id, view: 'tree', activeTab: 'graph' });
    this.loadLayout('tree');
  }
  findSimilar(id: string) {
    const n = this.s.index?.nodes.get(id);
    if (!n) return;
    this.runSearch(n.text.slice(0, 200), 'vector');
  }

  // ------------------------------------------------------------------ layouts (embed / tree come from the server)
  async loadLayout(view: 'embed' | 'tree') {
    this.set((s) => ({ layouts: { ...s.layouts, loading: true, error: undefined } }));
    const root = view === 'tree' ? (this.s.treeRoot ?? this.s.brain?.focus[0] ?? undefined) : undefined;
    const r = await this.op('layout', () => this.api.layout(view, root), true);
    this.set((s) => ({
      layouts: r ? { ...s.layouts, [view]: r, loading: false } : { ...s.layouts, loading: false, error: s.lastOp?.error },
      ...(r && view === 'tree' && r.root ? { treeRoot: r.root } : {}),
    }));
  }

  // ------------------------------------------------------------------ search
  setSearch(p: Partial<UiState['search']>) {
    this.set((s) => ({ search: { ...s.search, ...p } }));
  }
  /** Narrow the hits that came back (client-side only, no new request). */
  setNarrow(p: Partial<ResultNarrow>) {
    this.set((s) => ({ search: { ...s.search, narrow: { ...s.search.narrow, ...p } } }));
  }
  toggleNarrow(group: 'kinds' | 'sections' | 'sources', value: string) {
    const cur = this.s.search.narrow[group];
    this.setNarrow({ [group]: cur.includes(value) ? cur.filter((x) => x !== value) : [...cur, value] } as Partial<ResultNarrow>);
  }
  clearNarrow() {
    this.set((s) => ({ search: { ...s.search, narrow: { ...EMPTY_NARROW } } }));
  }
  /** Keeps the last 20 searches (de-duplicated by query + mode). */
  private rememberSearch(e: RecentSearch) {
    const list = [e, ...this.s.searches.filter((x) => x.query !== e.query || x.mode !== e.mode)].slice(0, 20);
    saveLocal(SEARCHES_KEY, list);
    this.set({ searches: list });
  }
  clearSearches() {
    saveLocal(SEARCHES_KEY, []);
    this.set({ searches: [] });
  }
  async runSearch(query?: string, mode?: SearchMode) {
    const q = (query ?? this.s.search.query).trim();
    if (!q) return;
    const m = mode ?? this.s.search.mode;
    this.set((s) => ({ search: { ...s.search, query: q, mode: m, loading: true, error: undefined, narrow: { ...EMPTY_NARROW } } }));
    this.openTab('search');
    const t0 = performance.now();
    const sources = this.s.search.sources;
    const r = await this.op('search', () => this.api.search({ query: q, mode: m, k: this.s.search.k, ...(sources.length ? { sources } : {}) }));
    const ms = Math.round(performance.now() - t0);
    if (!r) {
      this.set((s) => ({ search: { ...s.search, loading: false, error: s.lastOp?.error, result: null } }));
      return;
    }
    const highlight = r.rows.map((x) => x.id);
    let pathEdges: [string, string][] = [];
    const items: PulseItem[] = [];
    if (m === 'tree') {
      pathEdges = r.rows.filter((x) => x.parent).map((x) => [x.parent!, x.id]);
      r.rows.forEach((x) => {
        if (x.parent) items.push({ from: x.parent, to: x.id, delay: ((x.depth ?? 1) - 1) * 0.55, dur: 0.55, color: depthColor(x.depth ?? 1), size: 5 });
      });
    }
    this.set((s) => ({ search: { ...s.search, loading: false, result: r, ms }, highlight, pathEdges, pulses: { token: this.pulseToken++, items } }));
    this.rememberSearch({ query: q, mode: m, ts: Date.now(), rows: r.rows.length + (r.via?.length ?? 0), ms });
  }

  // ------------------------------------------------------------------ ingest
  async ingest(files: File[]) {
    if (!files.length) return;
    this.set({ ingest: { loading: true } });
    this.openTab('search');
    const r = await this.op('ingest', () => this.api.ingest(files, this.s.ingestFocus));
    this.set({ ingest: r ? { data: r, loading: false } : { loading: false, error: this.s.lastOp?.error } });
    if (r) {
      this.set({ agentHighlight: r.fresh });
      await this.refresh();
    }
  }

  // ------------------------------------------------------------------ brain xml
  async putBrainXml(xml: string) {
    const st = await this.op('apply brain.xml', () => this.api.putBrainXml(xml));
    if (st) {
      this.setBrain(st);
      this.toastMsg('brain.xml applied');
    }
    return !!st;
  }

  // ------------------------------------------------------------------ raw query (Addendum A)
  setQuery(p: Partial<UiState['query']>) {
    this.set((s) => ({ query: { ...s.query, ...p } }));
  }
  async runQuery(text?: string) {
    const q = text ?? this.s.query.text;
    if (!q.trim()) return;
    const format = this.s.query.format;
    this.set((s) => ({ query: { ...s.query, text: q, loading: true, error: undefined } }));
    this.openTab('query');
    let payload: string | object = q;
    const trimmed = q.trim();
    if (trimmed.startsWith('{')) {
      try {
        payload = JSON.parse(trimmed) as object;
      } catch (e) {
        this.set((s) => ({ query: { ...s.query, loading: false, error: 'Invalid JSON: ' + errMsg(e) } }));
        return;
      }
    }
    const srcs = this.s.query.sources;
    if (srcs.length && typeof payload === 'string') payload = `${payload} source:${srcs.join(',')}`;
    else if (srcs.length && typeof payload === 'object' && !('from' in payload)) payload = { ...payload, from: `memory@${srcs.join(',')}` };
    const t0 = performance.now();
    const r = await this.op('query', () => this.api.query(payload, format), true);
    const ms = Math.round(performance.now() - t0);
    const entry: HistoryEntry = { text: q, format, ts: Date.now(), ms, rows: r?.count, ok: !!r };
    const history = [entry, ...this.s.history.filter((h) => h.text !== q)].slice(0, 50);
    saveLocal(HISTORY_KEY, history);
    this.set((s) => ({ history, query: { ...s.query, loading: false, result: r ?? null, error: r ? undefined : s.lastOp?.error, ms } }));
  }
  // ------------------------------------------------------------------ BDQL (Addendum G)
  setBdql(p: Partial<UiState['bdql']>) {
    this.set((s) => ({ bdql: { ...s.bdql, ...p } }));
  }
  private lintTimer: ReturnType<typeof setTimeout> | null = null;
  /** Debounced parse-only lint via POST /api/run/explain. */
  lintBdql(script: string, debounce = 400) {
    if (this.lintTimer) clearTimeout(this.lintTimer);
    this.lintTimer = setTimeout(async () => {
      try {
        const r = await this.api.explain(script);
        if (this.s.bdql.script === script) this.setBdql({ lint: r.errors });
      } catch {
        /* lint is best-effort */
      }
    }, debounce);
  }
  /** Run the statement at `cursor`, or the whole script when cursor is undefined. */
  async runBdqlEditor(cursor?: number) {
    const full = this.s.bdql.script;
    const script = cursor === undefined ? full : statementAt(full, cursor);
    if (!script.trim()) return;
    this.setBdql({ loading: true, error: undefined });
    this.openTab('query');
    const t0 = performance.now();
    const r = await this.op('run', () => this.api.run(script, { dryRun: this.s.bdql.dryRun, format: this.s.query.format }), true);
    const ms = Math.round(performance.now() - t0);
    const entry: HistoryEntry = { text: script, format: this.s.query.format, ts: Date.now(), ms, rows: r?.results.length, ok: !!r?.ok };
    const history = [entry, ...this.s.history.filter((h) => h.text !== script)].slice(0, 50);
    saveLocal(HISTORY_KEY, history);
    this.set((s) => ({ history, bdql: { ...s.bdql, loading: false, result: r ?? null, error: r ? undefined : s.lastOp?.error, ms, active: 0, lint: r?.error ? [r.error] : s.bdql.lint } }));
    const writes = r?.results.some((x) => x.kind === 'changes');
    if (writes && !this.s.bdql.dryRun) await this.refresh();
    if (r) {
      const ids = r.results.flatMap((x) => x.ids ?? []);
      if (ids.length) this.set({ agentHighlight: ids });
    }
  }
  saveQuery(name: string, text: string) {
    const saved = [{ name, text }, ...this.s.saved.filter((x) => x.name !== name)];
    saveLocal(SAVED_KEY, saved);
    this.set({ saved });
  }
  deleteSaved(name: string) {
    const saved = this.s.saved.filter((x) => x.name !== name);
    saveLocal(SAVED_KEY, saved);
    this.set({ saved });
  }
  clearHistory() {
    saveLocal(HISTORY_KEY, []);
    this.set({ history: [] });
  }

  // ------------------------------------------------------------------ templates
  async loadTemplates() {
    this.set({ templates: { loading: true } });
    const r = await this.op('templates', () => this.api.templates(), true);
    this.set({ templates: r ? { data: r, loading: false } : { loading: false, error: this.s.lastOp?.error } });
  }
  openTemplate(t: Template) {
    this.set({ templateDraft: { content: t.content, format: t.format, name: t.name, token: Date.now() } });
    this.openTab('template');
  }

  // ------------------------------------------------------------------ sources (Addendum C)
  async loadSources() {
    this.set((s) => ({ sources: { ...s.sources, loading: true } }));
    const r = await this.op('sources', () => this.api.sources(), true);
    this.set((s) => ({ sources: r ? { data: r, loading: false } : { data: s.sources.data, loading: false, error: s.lastOp?.error } }));
  }
  async addSource(src: SourceInput) {
    const r = await this.op('add source', () => this.api.addSource(src));
    if (r) {
      this.set({ sources: { data: r, loading: false } });
      this.toastMsg(`Source "${src.name}" saved`);
      await this.refresh();
    }
    return !!r;
  }
  async removeSource(name: string) {
    const r = await this.op('remove source', () => this.api.removeSource(name).then(() => true));
    if (r) {
      await this.loadSources();
      await this.refresh();
    }
  }
  async syncSource(name: string) {
    const r = await this.op('sync ' + name, () => this.api.syncSource(name));
    if (r) {
      this.set({ sources: { data: r, loading: false } });
      await this.refresh();
    }
  }
  /** Run a BDQL script for an action (connector actions etc.), then refresh. */
  async runBdql(script: string, label = 'run') {
    if (this.s.viewAs) {
      this.toastMsg(`Writes are disabled while viewing as ${this.s.viewAs}.`, false);
      return undefined;
    }
    const r = await this.op(label, () => this.api.run(script));
    if (!r) return undefined;
    if (!r.ok || r.error) {
      const e = r.error;
      const denied = e && /access denied/i.test(e.message);
      this.toastMsg(e ? `${denied ? 'Write denied — ' : ''}${label}: statement ${e.statement}, line ${e.line}:${e.col}: ${e.message}` : `${label} failed`, false);
      if (denied) this.set({ tool: { type: 'message', title: 'Write denied', text: e!.message, ok: false } });
      return r;
    }
    const changes = r.results.flatMap((x) => x.changes ?? []);
    this.toastMsg(changes.length ? changes.slice(0, 6).join('\n') : r.status || 'ok');
    await this.refresh();
    return r;
  }
  async linkExternal(local: string, remoteId: string, w: number) {
    return this.addLink(local, remoteId, w, 'external');
  }
}

export const connectorsVisible = (s: Pick<UiState, 'showConnectors' | 'view'>) => s.showConnectors ?? s.view !== 'brain';

/** Reviewers: admins or holders of a review role/grant (Addendum O). */
export const isReviewer = (w: Whoami | null | undefined) =>
  !!w && (!!w.admin || w.can?.review === true || (w.can === undefined && (w.roles ?? []).some((r) => /review/i.test(r))));
