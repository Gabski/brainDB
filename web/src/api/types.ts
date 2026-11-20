// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

// Types of the brainDB HTTP API (docs/SPEC.md "HTTP API" + Addendum A).

export type EdgeType = 'concept' | 'assoc' | 'similar' | 'supersede' | 'part' | 'seq' | 'new' | 'external' | 'connector';

export interface Kind {
  name: string;
  label: string;
  color: string;
  base: number;
  halfLife: number | null;
  /** Share of the base weight that age cannot take away (0.0.2). Older servers omit it. */
  floor?: number | null;
  prefix: string;
  section: string;
  builtin: boolean;
}

export interface FileInfo {
  name: string;
  mime: string;
  size: number;
}

/** A memory or concept. `activation`/`depth`/`via` are present only when the node is lit. */
export interface BrainNode {
  id: string;
  kind: string;
  section: string;
  text: string;
  base: number;
  weight: number;
  age: number;
  pinned: boolean;
  supersededBy?: string;
  parent?: string;
  index?: number;
  file?: FileInfo;
  fields?: Record<string, string>;
  /** Federation (Addendum C): source name and stub flag of a mounted external node. */
  source?: string;
  external?: boolean;
  activation?: number;
  depth?: number;
  via?: string;
  /** Addendum H: connector ids the activation passed through to reach this node. */
  route?: string[];
  /** Addendum I: entities (State only with ?entities=1; always in Inspect item). */
  entities?: Entity[];
  /** Addendum M: access labels (sent only to admins and holders). */
  labels?: string[];
}

export interface Edge {
  a: string;
  b: string;
  w: number;
  type: EdgeType;
}

export interface Params {
  depth: number;
  threshold: number;
  hop: number;
  showSuperseded: boolean;
}

export interface Embedding {
  model: string;
  dim: number;
  alpha: number;
  hops: number;
  use: 'raw' | 'enriched';
  lambda: number;
  coherenceRaw: number;
  coherenceEnriched: number;
}

export interface Stats {
  lit: number;
  memories: number;
  concepts: number;
  tokens: number;
  reach: number;
  avgWeight: number;
  superseded: number;
  newLinks: number;
  byDepth: number[];
  connectors?: number;
  routesUsed?: number;
  connectorEdges?: number;
}

export interface State {
  name: string;
  goal: string;
  step: number;
  params: Params;
  embedding: Embedding;
  kinds: Kind[];
  sections: string[];
  focus: string[];
  goalFocus: string[];
  nodes: BrainNode[];
  edges: Edge[];
  stats: Stats;
  status: string;
  /** Addendum H: structural connector nodes (not in `nodes`). */
  connectors?: Connector[];
}

export type SearchMode = 'hybrid' | 'vector' | 'tree' | 'graph';

export interface SearchRow {
  id: string;
  score: number;
  raw?: number;
  enriched?: number;
  lift?: number;
  vector?: number;
  graph?: number;
  depth?: number;
  via?: string;
  parent?: string | null;
  source?: string;
}

export interface SearchResult {
  mode: SearchMode | string;
  lambda?: number;
  rows: SearchRow[];
  via?: SearchRow[];
  roots?: string[];
}

export interface Suggestion {
  u: string;
  v: string;
  score: number;
  via: string[];
  commonStems: number;
  cos: number;
}

export interface InspectItem {
  node: BrainNode;
  entities?: Entity[];
  decay: number;
  chain: string[];
  xml: string;
}
export interface InspectLink {
  to: string;
  w: number;
  type: string;
}
export interface InspectNear {
  id: string;
  cos: number;
  linked: boolean;
}
export interface InspectTest {
  reached: { id: string; act: number; depth: number }[];
  ablation: { to: string; w: number; type: string; dLit: number; dSum: number; critical: boolean }[];
}
export interface InspectPath {
  path: string[];
  weights: number[];
  strength: number;
}
export interface Discover {
  inner: Suggestion[];
  bridge: Suggestion[];
}

export interface Inspect {
  item?: InspectItem;
  links?: InspectLink[];
  near?: InspectNear[];
  test?: InspectTest;
  suggest?: Suggestion[];
  paths?: InspectPath[];
  discover?: Discover;
  connectors?: ConnectorSuggestion[];
}

export type InspectWhat = 'item' | 'links' | 'near' | 'test' | 'suggest' | 'paths' | 'discover' | 'connectors';

export interface AutoLink {
  a: string;
  b: string;
  cos: number;
  type: 'concept' | 'similar';
}

export interface IngestResult {
  report: string[];
  fresh: string[];
  links: AutoLink[];
  status: string;
}

export interface BrainEvent {
  type: 'changed' | 'hello' | 'import.progress' | string;
  ts: number;
  by: string;
  tool: string;
  summary: string;
  ids: string[];
  /** Addendum E: database the event belongs to. */
  db?: string;
  /** import.progress only */
  done?: number;
  total?: number;
  file?: string;
}

export interface ExecResult {
  changes: string[];
  touched: string[];
  auto: AutoLink[];
  status: string;
}

export type Vec3 = [number, number, number];

export interface LayoutResult {
  positions: Record<string, Vec3>;
  treeEdges?: [string, string][];
  root?: string;
}

export interface VectorResult {
  raw: number[];
  enriched: number[];
  dim: number;
  drift: number;
}

export interface ContextPatch {
  focus?: string[];
  addFocus?: string[];
  removeFocus?: string[];
  depth?: number;
  threshold?: number;
  hop?: number;
  showSuperseded?: boolean;
  goal?: string;
  goalFocus?: boolean;
}

export interface EmbeddingPatch {
  alpha?: number;
  hops?: number;
  use?: 'raw' | 'enriched';
  lambda?: number;
}

export interface KindInput {
  name: string;
  color: string;
  halfLife: number;
  base?: number;
}

/** Exec ops (atomic batch). `@name` references a `tmp` created earlier in the same batch. */
export type ExecOp =
  | {
      op: 'remember';
      kind: string;
      text: string;
      section?: string;
      concepts?: string[];
      links?: { to: string; w?: number }[];
      supersedes?: string;
      pin?: boolean;
      focus?: boolean;
      tmp?: string;
      autolink?: boolean;
      fields?: Record<string, string>;
    }
  | { op: 'concept'; label: string; members?: string[]; related?: string[]; tmp?: string }
  | { op: 'link'; a: string; b: string; w?: number; type?: EdgeType }
  | { op: 'unlink'; a: string; b: string }
  | { op: 'boost' | 'weaken' | 'pin' | 'unpin'; id: string }
  | { op: 'supersede'; old: string; new: string }
  | { op: 'edit'; id: string; text?: string; section?: string; kind?: string; fields?: Record<string, string> }
  | { op: 'forget'; id: string; force?: boolean }
  | { op: 'focus'; ids?: string[]; add?: string[]; remove?: string[] }
  | { op: 'goal'; text: string; focus?: string[] };

// ---- Addendum A: database (query + templates) ----

export type QueryFormat = 'json' | 'yaml' | 'xml' | 'md' | 'csv';
export type ContextFormat = 'xml' | 'md' | 'json' | 'yaml';
export type TemplateFormat = 'auto' | 'json' | 'yaml' | 'xml' | 'md';
export type FillOutput = 'same' | 'json' | 'yaml' | 'xml' | 'md';

export interface QueryResult {
  columns: string[];
  rows: Record<string, unknown>[];
  count: number;
  total: number;
  rendered: string;
}

export interface ContextQuery {
  focus?: string[];
  query?: string;
  depth?: number;
  threshold?: number;
  hop?: number;
  format: ContextFormat;
}

/** Normalised result of POST /api/context/query. */
export interface ContextQueryResult {
  output: string;
  format: string;
}

export interface SlotValue {
  path: string;
  description: string;
  type: string;
  value: string | null;
  source: string | null;
  score: number;
  snippet: string;
}

export interface FillRequest {
  template: string;
  templateFormat?: TemplateFormat;
  output?: FillOutput;
  scope?: 'context' | 'memory';
  withSources?: boolean;
  min?: number;
}

export interface FillResult {
  output: string;
  format: string;
  values: SlotValue[];
  missing: string[];
}

export interface Template {
  name: string;
  path: string;
  format: string;
  content: string;
}

// ---- Addendum C: external sources / federation ----

export interface Source {
  name: string;
  uri: string;
  weight: number;
  mode: string;
  ttl: number;
  federateDepth: number;
  enabled: boolean;
  status: 'ok' | 'error' | 'disabled';
  latencyMs: number;
  nodes: number;
  lastError?: string;
  lastSync?: string;
}

export interface SourceInput {
  name: string;
  uri: string;
  weight?: number;
  mode?: string;
  ttl?: number;
  federateDepth?: number;
  enabled?: boolean;
  tokenEnv?: string;
}

/** A row of the remote picker (GET /api/sources/{name}/search). */
export interface RemoteHit {
  id: string;
  kind: string;
  text: string;
  weight?: number;
  score?: number;
}

// ---- Addendum E: databases, bulk import, configuration ----

export interface DbInfo {
  name: string;
  nodes: number;
  edges: number;
  step: number;
  sizeBytes: number;
  updated: string;
  goal: string;
}

export type ImportStatus = 'created' | 'updated' | 'skipped' | 'error';

export interface ImportFileReport {
  path: string;
  importer: string;
  status: ImportStatus;
  nodes: number;
  edges: number;
  records: number;
  message?: string;
  /** Addendum O: admission decisions in this file. */
  admitted?: number;
  quarantined?: number;
  rejected?: number;
  reasons?: string[];
}

/** Optional detail the server may add for SQL dumps (rendered when present). */
export interface ImportTable {
  name: string;
  file?: string;
  columns: string[];
  rows: number;
  primaryKey?: string;
  foreignKeys?: { column: string; references: string }[];
  concepts?: string[];
}

export interface ImportReport {
  files: ImportFileReport[];
  totals: { files?: number; nodes: number; edges: number; records: number; errors?: number; skipped?: number; [k: string]: number | undefined };
  /** Detected mapping as a braindb.yaml snippet. */
  mapping: string;
  tables?: ImportTable[];
  concepts?: string[];
  dryRun?: boolean;
}

/** One entry of the embedding-provider registry (GET /api/embedding). The server owns this list. */
export interface EmbedProvider {
  name: string;
  label: string;
  /** Request/response format: hash (built in), openai, cohere or gemini. */
  wire: string;
  /** Default endpoint; absent means the provider is self-hosted and `baseUrl` must be configured. */
  baseUrl?: string;
  /** The variable this service conventionally uses — a hint for `apiKeyEnv`, never read by the server. */
  keyEnv?: string;
  needsKey: boolean;
  maxBatch?: number;
  inputType?: string;
  auth?: string;
  model?: string;
  note?: string;
}

export interface ConfigError {
  line: number;
  message: string;
}

export interface ConfigResult {
  ok: boolean;
  errors: ConfigError[];
  /** Effective config (object, or text when the server renders it). */
  effective: unknown;
  yaml?: string;
}

/** A file to import together with its path relative to the dropped folder. */
export interface ImportFile {
  file: File;
  path: string;
}

// ---- Addendum F: storage & performance ----

/** GET /api/stats (same as `braindb stats`). Named ServerStats to avoid clashing with State.stats. */
export interface ServerStats {
  storage: 'file' | 'sqlite' | string;
  redis?: 'ok' | 'error';
  nodes: number;
  edges: number;
  /** Index sizes, cache and latency come from the Redis build only: a plain file backend omits them. */
  indexes?: { name: string; size: number }[];
  cacheHitRate?: number;
  latency?: { op: string; p50: number; p95: number }[];
  /** Database name and current step (plain file backend). */
  db?: string;
  step?: number;
  /** Addendum L: on-disk file format and file sizes. */
  format?: 'xml' | 'bin' | 'both' | string;
  files?: { name: string; bytes: number }[];
}

/** GET /api/health (no token needed). */
export interface Health {
  ok?: boolean;
  version?: string;
  /** false when the server runs a single --brain file and cannot create or delete databases. */
  manageDbs?: boolean;
  storage?: string;
  redis?: 'ok' | 'error';
  format?: string;
  [k: string]: unknown;
}

// ---- Addendum G: BDQL ----

export interface PlanNode {
  op: string;
  detail?: string;
  index?: string;
  rows?: number;
  cache?: 'hit' | 'miss' | string;
  children?: PlanNode[];
}

export interface StatementResult {
  index: number;
  statement: string;
  kind: 'rows' | 'text' | 'changes' | 'plan';
  columns?: string[];
  rows?: Record<string, unknown>[];
  text?: string;
  changes?: string[];
  ids?: string[];
  /** EXPLAIN tree, when the server returns it structured. */
  plan?: PlanNode;
  elapsed_ms: number;
}

export interface BdqlError {
  statement: number;
  line: number;
  col: number;
  message: string;
}

export interface RunResult {
  results: StatementResult[];
  status: string;
  ok: boolean;
  error?: BdqlError;
}

/** POST /api/run/explain: parse-only (lint). */
export interface ExplainResult {
  ok: boolean;
  errors: BdqlError[];
  plan?: unknown;
  ast?: unknown;
}

// ---- Addendum H: connectors ----

export type ConnectorType = 'anchor' | 'bridge' | 'hub' | 'alias' | 'time';

export interface Connector {
  id: string;
  type: ConnectorType | string;
  label: string;
  key?: string;
  gain: number;
  transparent: boolean;
  terms?: string[];
  source?: string;
  degree: number;
}

export interface ConnectorSuggestion {
  type: ConnectorType | string;
  label: string;
  key?: string;
  terms?: string[];
  members: string[];
  score: number;
  evidence: string;
}

// ---- Addendum I: entities and patterns ----

export interface Entity {
  type: string;
  value: string;
  raw: string;
  start: number;
  end: number;
  confidence: number;
  pii: boolean;
  masked?: boolean;
}

export interface Pattern {
  name: string;
  type: string;
  builtin: boolean;
  enabled: boolean;
  regex?: string;
  locales?: string[];
  pii: boolean;
}

/** A row of GET /api/entities. */
export interface EntityRow {
  id?: string;
  node: string;
  type: string;
  value: string;
  raw: string;
  confidence: number;
  pii?: boolean;
}

export interface PatternTestResult {
  text?: string;
  entities: Entity[];
}

// ---- Addendum M: users, roles, grants ----

export interface Whoami {
  name: string;
  /** Alias the server also sends; prefer `name`. */
  user?: string;
  display?: string;
  /** A Go nil slice marshals to null, so every list here can arrive null: read it with `?? []`. */
  roles: string[] | null;
  labels: string[] | null;
  dbs: string[] | null;
  admin?: boolean;
  defaultWriteLabels?: string[];
  /** Grants resolved by the server. A role may hold `review` without saying so in its name. */
  can?: { review?: boolean; purposeOverride?: boolean };
}

// ---- Addendum O: purpose and admission ----

export type AdmissionMode = 'off' | 'advise' | 'enforce';

export interface Purpose {
  name: string;
  description: string;
  inScope: { topics: string[]; concepts: string[]; kinds: string[]; sections: string[] };
  outOfScope: { topics: string[]; entities: string[] };
  examples: { accept: string[]; reject: string[] };
  admission: { mode: AdmissionMode; accept: number; review: number; apply?: string[]; exempt?: string[]; chunks?: 'file' | 'each' };
  /** counters (SHOW PURPOSE) */
  counters?: { admitted: number; quarantined: number; rejected: number };
}

export type AdmissionDecision = 'accept' | 'review' | 'reject' | 'admitted' | 'quarantine' | 'rejected' | string;

export interface PurposeCheck {
  score: number;
  decision: AdmissionDecision;
  pos: number;
  neg: number;
  bonus: number;
  topMatch?: string;
  topConflict?: string;
  rules: string[];
}

export interface QuarantineItem {
  id: string;
  kind: string;
  text: string;
  score: number;
  reasons: string[];
  section?: string;
  /** intended section after admission */
  intendedSection?: string;
  by?: string;
  ts?: number;
}
