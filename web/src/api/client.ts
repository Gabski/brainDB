// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type {
  BrainEvent,
  ContextPatch,
  ContextQuery,
  ContextQueryResult,
  EmbeddingPatch,
  EmbedProvider,
  ExecOp,
  ExecResult,
  FillRequest,
  FillResult,
  IngestResult,
  Inspect,
  InspectWhat,
  KindInput,
  LayoutResult,
  QueryFormat,
  QueryResult,
  RemoteHit,
  SearchMode,
  SearchResult,
  Source,
  SourceInput,
  DbInfo,
  ImportReport,
  ImportFile,
  ConfigResult,
  ServerStats,
  Health,
  Whoami,
  Purpose,
  PurposeCheck,
  QuarantineItem,
  RunResult,
  ExplainResult,
  BdqlError,
  Entity,
  EntityRow,
  Pattern,
  State,
  Template,
  VectorResult,
} from './types';
import { getToken } from './token';

export interface SearchQuery {
  query: string;
  mode?: SearchMode;
  k?: number;
  focus?: boolean;
  sources?: string[];
}

export type LiveStatus = 'connecting' | 'live' | 'offline';

/** Everything the UI may ask the brain. All brain logic lives behind this interface. */
export interface ApiClient {
  getState(): Promise<State>;
  setContext(patch: ContextPatch): Promise<State>;
  setEmbedding(patch: EmbeddingPatch): Promise<State>;
  /** The providers this server accepts for `embedding.provider`. */
  embeddingProviders(): Promise<EmbedProvider[]>;
  exec(ops: ExecOp[]): Promise<ExecResult>;
  search(q: SearchQuery): Promise<SearchResult>;
  inspect(q: { id?: string; what: InspectWhat[]; to?: string }): Promise<Inspect>;
  layout(view: 'embed' | 'tree', root?: string): Promise<LayoutResult>;
  vector(id: string): Promise<VectorResult>;
  ingest(files: File[], focus: boolean): Promise<IngestResult>;
  addKind(kind: KindInput): Promise<State>;
  contextText(format: 'xml' | 'md'): Promise<string>;
  getBrainXml(): Promise<string>;
  putBrainXml(xml: string): Promise<State>;
  journal(): Promise<BrainEvent[]>;
  query(query: string | object, format?: QueryFormat): Promise<QueryResult>;
  contextQuery(q: ContextQuery): Promise<ContextQueryResult>;
  fill(req: FillRequest): Promise<FillResult>;
  templates(): Promise<Template[]>;
  sources(): Promise<Source[]>;
  addSource(src: SourceInput): Promise<Source[]>;
  removeSource(name: string): Promise<void>;
  syncSource(name: string): Promise<Source[]>;
  remoteSearch(name: string, q: string): Promise<RemoteHit[]>;
  /** Addendum E: databases, bulk import, braindb.yaml. */
  dbs(): Promise<DbInfo[]>;
  createDb(name: string, config?: string): Promise<DbInfo[] | unknown>;
  deleteDb(name: string): Promise<void>;
  importFiles(files: ImportFile[], opts: { db?: string; config?: string; dryRun?: boolean }): Promise<ImportReport>;
  getConfig(db?: string): Promise<{ yaml: string; result: ConfigResult }>;
  putConfig(yaml: string, opts: { db?: string; dryRun?: boolean }): Promise<ConfigResult>;
  /** Addendum G: BDQL. */
  run(script: string, opts?: { format?: string; dryRun?: boolean }): Promise<RunResult>;
  explain(script: string): Promise<ExplainResult>;
  /** Addendum I: patterns and entities. */
  patterns(): Promise<Pattern[]>;
  testPatterns(text: string, types?: string[]): Promise<Entity[]>;
  entities(q?: { type?: string; value?: string; limit?: number }): Promise<EntityRow[]>;
  /** Enable/disable a pattern (endpoint assumed: POST /api/patterns {name, enabled}). */
  setPatternEnabled(name: string, enabled: boolean): Promise<Pattern[]>;
  health(): Promise<Health>;
  /** Addendum O: purpose and admission. */
  getPurpose(): Promise<Purpose | null>;
  putPurpose(p: Purpose): Promise<Purpose>;
  checkPurpose(q: { text: string; kind?: string; section?: string; fields?: Record<string, string> }): Promise<PurposeCheck>;
  quarantine(): Promise<QuarantineItem[]>;
  admit(ids: string[], section?: string): Promise<unknown>;
  reject(ids: string[], reason: string): Promise<unknown>;
  /** Addendum M: current principal. */
  whoami(): Promise<Whoami>;
  /** Admin preview: X-BrainDB-View-As on every call, `AS USER x;` prefix on BDQL, ?as= on SSE. */
  setViewAs(user: string | null): void;
  getViewAs(): string | null;
  /** Addendum L: convert the on-disk format. Resolves {unsupported:true} when the server cannot (404/405/501). */
  convert(to: 'xml' | 'bin' | 'both'): Promise<{ unsupported?: boolean } & Record<string, unknown>>;
  /** Addendum F: storage stats, snapshot, migrate. */
  stats(): Promise<ServerStats>;
  snapshot(): Promise<unknown>;
  migrate(to: 'file' | 'sqlite'): Promise<unknown>;
  /** Active database: sent as X-BrainDB-Database on every call and ?db= on the SSE URL. */
  setDb(name: string | null): void;
  getDb(): string | null;
  /** Subscribe to SSE events. Returns an unsubscribe function. */
  subscribe(onEvent: (ev: BrainEvent) => void, onStatus: (s: LiveStatus) => void): () => void;
}

export class ApiError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.status = status;
  }
}

async function readError(r: Response): Promise<ApiError> {
  let msg = `${r.status} ${r.statusText}`;
  try {
    const t = await r.text();
    try {
      const j = JSON.parse(t) as { error?: string };
      if (j.error) msg = j.error;
      else if (t) msg = t;
    } catch {
      if (t) msg = t.slice(0, 300);
    }
  } catch {
    /* ignore */
  }
  return new ApiError(msg, r.status);
}

/** Real client over fetch + EventSource. `base` is '' when served by the Go server or proxied by Vite. */
export class HttpApiClient implements ApiClient {
  /** Called when any /api call answers 401 (the UI then shows the token prompt). */
  onUnauthorized: (() => void) | null = null;
  private db: string | null = null;
  private viewAs: string | null = null;

  setViewAs(user: string | null) {
    this.viewAs = user;
  }
  getViewAs() {
    return this.viewAs;
  }
  async getPurpose() {
    try {
      const r = await this.req<Purpose | { purpose?: Purpose | null }>('GET', '/api/purpose');
      return ((r as { purpose?: Purpose }).purpose !== undefined ? (r as { purpose: Purpose }).purpose : (r as Purpose)) ?? null;
    } catch (e) {
      if (e instanceof ApiError && e.status === 404) return null;
      throw e;
    }
  }
  async putPurpose(p: Purpose) {
    const r = await this.req<Purpose | { purpose: Purpose }>('PUT', '/api/purpose', p);
    return (r as { purpose?: Purpose }).purpose ?? (r as Purpose);
  }
  checkPurpose(q: { text: string; kind?: string; section?: string; fields?: Record<string, string> }) {
    return this.req<PurposeCheck>('POST', '/api/purpose/check', q);
  }
  async quarantine() {
    const r = await this.req<unknown>('GET', '/api/quarantine');
    return (Array.isArray(r) ? r : ((r as { items?: QuarantineItem[]; rows?: QuarantineItem[] }).items ?? (r as { rows?: QuarantineItem[] }).rows ?? [])) as QuarantineItem[];
  }
  admit(ids: string[], section?: string) {
    return this.req<unknown>('POST', '/api/quarantine/admit', section ? { ids, section } : { ids });
  }
  reject(ids: string[], reason: string) {
    return this.req<unknown>('POST', '/api/quarantine/reject', { ids, reason });
  }
  whoami() {
    return this.req<Whoami>('GET', '/api/whoami');
  }

  constructor(private base = '') {}

  setDb(name: string | null) {
    this.db = name;
  }
  getDb() {
    return this.db;
  }

  /** fetch with `Authorization: Bearer <token>`; throws ApiError on non-2xx. */
  private async call(path: string, init: RequestInit = {}): Promise<Response> {
    const headers = new Headers(init.headers);
    const tok = getToken();
    if (tok) headers.set('Authorization', 'Bearer ' + tok);
    if (this.db && !headers.has('X-BrainDB-Database')) headers.set('X-BrainDB-Database', this.db);
    if (this.viewAs) headers.set('X-BrainDB-View-As', this.viewAs);
    const r = await fetch(this.base + path, { cache: 'no-store', ...init, headers });
    if (r.status === 401) this.onUnauthorized?.();
    if (!r.ok) throw await readError(r);
    return r;
  }
  private async req<T>(method: string, path: string, body?: unknown): Promise<T> {
    const init: RequestInit = { method };
    if (body !== undefined) {
      init.headers = { 'Content-Type': 'application/json' };
      init.body = JSON.stringify(body);
    }
    return (await (await this.call(path, init)).json()) as T;
  }
  private async text(path: string): Promise<string> {
    return (await this.call(path)).text();
  }

  getState() {
    return this.req<State>('GET', '/api/state');
  }
  setContext(patch: ContextPatch) {
    return this.req<State>('POST', '/api/context', patch);
  }
  async embeddingProviders() {
    const r = await this.req<{ providers: EmbedProvider[] }>('GET', '/api/embedding');
    return r.providers ?? [];
  }

  setEmbedding(patch: EmbeddingPatch) {
    return this.req<State>('POST', '/api/embedding', patch);
  }
  exec(ops: ExecOp[]) {
    return this.req<ExecResult>('POST', '/api/exec', { ops });
  }
  search(q: SearchQuery) {
    return this.req<SearchResult>('POST', '/api/search', q);
  }
  inspect(q: { id?: string; what: InspectWhat[]; to?: string }) {
    const p = new URLSearchParams();
    if (q.id) p.set('id', q.id);
    p.set('what', q.what.join(','));
    if (q.to) p.set('to', q.to);
    return this.req<Inspect>('GET', '/api/inspect?' + p.toString());
  }
  layout(view: 'embed' | 'tree', root?: string) {
    const p = new URLSearchParams({ view });
    if (root) p.set('root', root);
    return this.req<LayoutResult>('GET', '/api/layout?' + p.toString());
  }
  vector(id: string) {
    return this.req<VectorResult>('GET', '/api/vector?id=' + encodeURIComponent(id));
  }
  async ingest(files: File[], focus: boolean) {
    const fd = new FormData();
    files.forEach((f) => fd.append('files[]', f, f.name));
    fd.append('focus', focus ? 'true' : 'false');
    const r = await this.call('/api/ingest', { method: 'POST', body: fd });
    return (await r.json()) as IngestResult;
  }
  addKind(kind: KindInput) {
    return this.req<State>('POST', '/api/kinds', kind);
  }
  contextText(format: 'xml' | 'md') {
    return this.text(format === 'md' ? '/api/context.md' : '/api/context.xml');
  }
  getBrainXml() {
    return this.text('/api/brain.xml');
  }
  async putBrainXml(xml: string) {
    const r = await this.call('/api/brain.xml', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/xml' },
      body: xml,
    });
    return (await r.json()) as State;
  }
  journal() {
    return this.req<BrainEvent[]>('GET', '/api/journal');
  }
  query(query: string | object, format?: QueryFormat) {
    return this.req<QueryResult>('POST', '/api/query', { query, format });
  }
  async contextQuery(q: ContextQuery): Promise<ContextQueryResult> {
    const r = await this.call('/api/context/query', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(q),
    });
    return normaliseContextQuery(await r.text(), r.headers.get('Content-Type') || '', q.format);
  }
  fill(req: FillRequest) {
    return this.req<FillResult>('POST', '/api/fill', req);
  }
  templates() {
    return this.req<Template[]>('GET', '/api/templates');
  }
  sources() {
    return this.req<Source[]>('GET', '/api/sources');
  }
  async run(script: string, opts: { format?: string; dryRun?: boolean } = {}) {
    // A failing script returns {ok:false, error} (possibly with a 4xx): surface it as a RunResult.
    const r = await fetch(this.base + '/api/run', {
      method: 'POST',
      headers: this.authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ script: this.viewAs && !/^\s*AS\s+USER\b/i.test(script) ? `AS USER ${this.viewAs};\n${script}` : script, ...opts }),
    });
    if (r.status === 401) this.onUnauthorized?.();
    const t = await r.text();
    let j: Partial<RunResult> & { error?: BdqlError | string };
    try {
      j = JSON.parse(t);
    } catch {
      throw new ApiError(t || r.statusText, r.status);
    }
    if (typeof j.error === 'string') {
      if (!j.results) return { results: [], status: '', ok: false, error: { statement: 0, line: 0, col: 0, message: j.error } };
    }
    if (!r.ok && !j.results && !j.error) throw new ApiError(t, r.status);
    return { results: j.results ?? [], status: j.status ?? '', ok: j.ok ?? r.ok, ...(j.error && typeof j.error !== 'string' ? { error: j.error } : {}) } as RunResult;
  }
  async explain(script: string): Promise<ExplainResult> {
    const r = await fetch(this.base + '/api/run/explain', {
      method: 'POST',
      headers: this.authHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ script }),
    });
    if (r.status === 401) this.onUnauthorized?.();
    const t = await r.text();
    try {
      const j = JSON.parse(t) as Partial<ExplainResult> & { error?: BdqlError | string };
      const errors = j.errors ?? (j.error ? [typeof j.error === 'string' ? { statement: 0, line: 0, col: 0, message: j.error } : j.error] : []);
      return { ok: j.ok ?? errors.length === 0, errors, plan: j.plan, ast: j.ast };
    } catch {
      throw new ApiError(t || r.statusText, r.status);
    }
  }
  patterns() {
    return this.req<Pattern[]>('GET', '/api/patterns');
  }
  async testPatterns(text: string, types?: string[]) {
    const r = await this.req<unknown>('POST', '/api/patterns/test', types?.length ? { text, types } : { text });
    return (Array.isArray(r) ? r : ((r as { entities?: Entity[] }).entities ?? [])) as Entity[];
  }
  async entities(q: { type?: string; value?: string; limit?: number } = {}) {
    const p = new URLSearchParams();
    if (q.type) p.set('type', q.type);
    if (q.value) p.set('value', q.value);
    if (q.limit) p.set('limit', String(q.limit));
    const r = await this.req<unknown>('GET', '/api/entities' + (p.toString() ? '?' + p.toString() : ''));
    return (Array.isArray(r) ? r : ((r as { rows?: EntityRow[] }).rows ?? [])) as EntityRow[];
  }
  async setPatternEnabled(name: string, enabled: boolean) {
    const r = await this.req<unknown>('POST', '/api/patterns', { name, enabled });
    return Array.isArray(r) ? (r as Pattern[]) : this.patterns();
  }
  async health() {
    const r = await fetch(this.base + '/api/health', { cache: 'no-store' });
    if (!r.ok) throw await readError(r);
    return (await r.json()) as Health;
  }
  async convert(to: 'xml' | 'bin' | 'both') {
    try {
      return await this.req<Record<string, unknown>>('POST', '/api/storage/convert', { to });
    } catch (e) {
      // 501 is what this build returns for a storage backend it does not carry.
      if (e instanceof ApiError && (e.status === 404 || e.status === 405 || e.status === 501)) return { unsupported: true };
      throw e;
    }
  }
  stats() {
    return this.req<ServerStats>('GET', '/api/stats');
  }
  // snapshot writes the brain now; migrate switches the storage backend (Addendum F5).
  snapshot() {
    return this.req<unknown>('POST', '/api/snapshot', {});
  }
  migrate(to: 'file' | 'sqlite') {
    return this.req<unknown>('POST', '/api/migrate', { to });
  }
  dbs() {
    return this.req<DbInfo[]>('GET', '/api/dbs');
  }
  createDb(name: string, config?: string) {
    return this.req<unknown>('POST', '/api/dbs', config ? { name, config } : { name });
  }
  async deleteDb(name: string) {
    await this.call('/api/dbs/' + encodeURIComponent(name) + '?confirm=' + encodeURIComponent(name), { method: 'DELETE' });
  }
  async importFiles(files: ImportFile[], opts: { db?: string; config?: string; dryRun?: boolean }) {
    const fd = new FormData();
    files.forEach((f) => fd.append('files[]', f.file, f.path || f.file.name));
    if (opts.config) fd.append('config', opts.config);
    if (opts.dryRun) fd.append('dryRun', 'true');
    const db = opts.db ?? this.db;
    const r = await this.call('/api/import' + (db ? '?db=' + encodeURIComponent(db) : ''), {
      method: 'POST',
      body: fd,
      headers: db ? { 'X-BrainDB-Database': db } : {},
    });
    return (await r.json()) as ImportReport;
  }
  async getConfig(db?: string) {
    const q = db ? '?db=' + encodeURIComponent(db) : '';
    const r = await this.call('/api/config' + q, db ? { headers: { 'X-BrainDB-Database': db } } : {});
    const t = await r.text();
    try {
      const j = JSON.parse(t) as Partial<ConfigResult> & { yaml?: string; config?: string };
      const yaml = j.yaml ?? j.config ?? '';
      return { yaml, result: { ok: j.ok ?? true, errors: j.errors ?? [], effective: j.effective ?? null } };
    } catch {
      return { yaml: t, result: { ok: true, errors: [], effective: null } };
    }
  }
  async putConfig(yaml: string, opts: { db?: string; dryRun?: boolean }) {
    const p = new URLSearchParams();
    if (opts.db) p.set('db', opts.db);
    if (opts.dryRun) p.set('dryRun', '1');
    const r = await fetch(this.base + '/api/config' + (p.toString() ? '?' + p.toString() : ''), {
      method: 'PUT',
      headers: this.authHeaders({ 'Content-Type': 'application/yaml', ...(opts.db ? { 'X-BrainDB-Database': opts.db } : {}) }),
      body: yaml,
    });
    if (r.status === 401) this.onUnauthorized?.();
    // validation errors come back as {ok:false, errors} (possibly with a 4xx status)
    const t = await r.text();
    try {
      const j = JSON.parse(t) as Partial<ConfigResult> & { error?: string };
      if (j.ok !== undefined || j.errors) return { ok: !!j.ok, errors: j.errors ?? [], effective: j.effective ?? null };
      if (!r.ok) throw new ApiError(j.error ?? t, r.status);
    } catch (e) {
      if (e instanceof ApiError) throw e;
      if (!r.ok) throw new ApiError(t || r.statusText, r.status);
    }
    return { ok: true, errors: [], effective: null };
  }
  private authHeaders(h: Record<string, string>): Record<string, string> {
    const tok = getToken();
    return {
      ...h,
      ...(tok ? { Authorization: 'Bearer ' + tok } : {}),
      ...(this.db && !h['X-BrainDB-Database'] ? { 'X-BrainDB-Database': this.db } : {}),
      ...(this.viewAs ? { 'X-BrainDB-View-As': this.viewAs } : {}),
    };
  }
  async addSource(src: SourceInput) {
    const r = await this.req<unknown>('POST', '/api/sources', src);
    return Array.isArray(r) ? (r as Source[]) : this.sources();
  }
  async removeSource(name: string) {
    await this.call('/api/sources/' + encodeURIComponent(name), { method: 'DELETE' });
  }
  async syncSource(name: string) {
    await this.call('/api/sources/' + encodeURIComponent(name) + '/sync', { method: 'POST' });
    return this.sources();
  }
  async remoteSearch(name: string, q: string) {
    const r = await this.req<unknown>('GET', '/api/sources/' + encodeURIComponent(name) + '/search?q=' + encodeURIComponent(q));
    return normaliseRemoteHits(r, name);
  }

  subscribe(onEvent: (ev: BrainEvent) => void, onStatus: (s: LiveStatus) => void) {
    if (typeof EventSource === 'undefined') {
      onStatus('offline');
      return () => {};
    }
    onStatus('connecting');
    const tok = getToken();
    const qs = new URLSearchParams();
    if (tok) qs.set('token', tok);
    if (this.db) qs.set('db', this.db);
    if (this.viewAs) qs.set('as', this.viewAs);
    const es = new EventSource(this.base + '/api/events' + (qs.toString() ? '?' + qs.toString() : ''));
    es.onopen = () => onStatus('live');
    es.onerror = () => onStatus(es.readyState === EventSource.CLOSED ? 'offline' : 'connecting');
    es.onmessage = (m) => {
      try {
        onEvent(JSON.parse(m.data as string) as BrainEvent);
      } catch {
        /* ignore malformed events */
      }
    };
    return () => es.close();
  }
}

/**
 * The context-query endpoint returns the slice "in the requested format". Accept either a JSON
 * envelope ({output}|{rendered}|{text}) or the rendered document itself.
 */
export function normaliseContextQuery(body: string, contentType: string, format: string): ContextQueryResult {
  if (contentType.includes('json')) {
    try {
      const j = JSON.parse(body) as Record<string, unknown>;
      const out = j.output ?? j.rendered ?? j.text;
      if (typeof out === 'string') return { output: out, format: typeof j.format === 'string' ? j.format : format };
      if (format === 'json') return { output: JSON.stringify(j, null, 2), format };
    } catch {
      /* fall through */
    }
  }
  return { output: body, format };
}

/** Accept `RemoteHit[]`, `{rows}` (SearchResult-like) or `{nodes}` from the remote picker endpoint. */
export function normaliseRemoteHits(r: unknown, source: string): RemoteHit[] {
  const o = r as { rows?: unknown[]; nodes?: unknown[]; hits?: unknown[] };
  const list = (Array.isArray(r) ? r : (o?.rows ?? o?.nodes ?? o?.hits ?? [])) as Record<string, unknown>[];
  return list.map((x) => {
    const id = String(x.id ?? '');
    return {
      id: id.includes(':') && !id.startsWith('A:') ? id : `${source}:${id}`,
      kind: String(x.kind ?? 'fact'),
      text: String(x.text ?? x.label ?? id),
      weight: typeof x.weight === 'number' ? x.weight : typeof x.w === 'number' ? x.w : undefined,
      score: typeof x.score === 'number' ? x.score : undefined,
    };
  });
}
