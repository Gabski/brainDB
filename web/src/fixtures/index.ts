// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

// Single place where UI fixtures are loaded. Replace the JSON/XML/MD files in ./data with
// output from the real backend (`braindb state` etc.) and every story + the mock client follow.
import type {
  BrainEvent,
  FillResult,
  IngestResult,
  Inspect,
  LayoutResult,
  QueryResult,
  RemoteHit,
  Source,
  DbInfo,
  ImportReport,
  ConfigResult,
  ServerStats,
  RunResult,
  ExplainResult,
  Entity,
  EntityRow,
  Pattern,
  Whoami,
  Purpose,
  PurposeCheck,
  QuarantineItem,
  RunResult as RR,
  SearchMode,
  SearchResult,
  State,
  Template,
  VectorResult,
} from '../api/types';
import stateJson from './data/state.json';
import inspectJson from './data/inspect.json';
import inspectRecordJson from './data/inspect-record.json';
import discoverJson from './data/discover.json';
import hybridJson from './data/search-hybrid.json';
import vectorSearchJson from './data/search-vector.json';
import treeJson from './data/search-tree.json';
import graphJson from './data/search-graph.json';
import layoutEmbedJson from './data/layout-embed.json';
import layoutTreeJson from './data/layout-tree.json';
import vectorJson from './data/vector.json';
import ingestJson from './data/ingest.json';
import journalJson from './data/journal.json';
import queryJson from './data/query.json';
import fillJson from './data/fill.json';
import fillMissingJson from './data/fill-missing.json';
import templatesJson from './data/templates.json';
import sourcesJson from './data/sources.json';
import remoteHitsJson from './data/remote-hits.json';
import dbsJson from './data/dbs.json';
import importReportJson from './data/import-report.json';
import importPartialJson from './data/import-report-partial.json';
import configResultJson from './data/config-result.json';
import configErrorsJson from './data/config-errors.json';
import statsJson from './data/stats.json';
import statsRedisJson from './data/stats-redis.json';
import statsBinJson from './data/stats-bin.json';
import statsFileJson from './data/stats-file.json';
import configYaml from './data/config.yaml?raw';
import configErrorsYaml from './data/config-errors.yaml?raw';
import runMultiJson from './data/run-multi.json';
import runExplainJson from './data/run-explain.json';
import runErrorJson from './data/run-error.json';
import explainErrorJson from './data/explain-error.json';
import connSuggJson from './data/connector-suggestions.json';
import inspectEntitiesJson from './data/inspect-entities.json';
import entitySampleJson from './data/entity-sample.json';
import entitiesJson from './data/entities.json';
import patternsJson from './data/patterns.json';
import patternsTestJson from './data/patterns-test.json';
import whoamiAdminJson from './data/whoami-admin.json';
import whoamiKimJson from './data/whoami-kim.json';
import accessJson from './data/access.json';
import purposeJson from './data/purpose.json';
import purposeCheckJson from './data/purpose-check.json';
import quarantineJson from './data/quarantine.json';
import contextXml from './data/context.xml?raw';
import contextMd from './data/context.md?raw';
import brainXml from './data/brain.xml?raw';

export const fixtureState = stateJson as unknown as State;
export const fixtureInspect = inspectJson as unknown as Inspect;
export const fixtureInspectRecord = inspectRecordJson as unknown as Inspect;
export const fixtureDiscover = discoverJson as unknown as Inspect;
export const fixtureSearch: Record<SearchMode, SearchResult> = {
  hybrid: hybridJson as unknown as SearchResult,
  vector: vectorSearchJson as unknown as SearchResult,
  tree: treeJson as unknown as SearchResult,
  graph: graphJson as unknown as SearchResult,
};
export const fixtureLayoutEmbed = layoutEmbedJson as unknown as LayoutResult;
export const fixtureLayoutTree = layoutTreeJson as unknown as LayoutResult;
export const fixtureVector = vectorJson as unknown as VectorResult;
export const fixtureIngest = ingestJson as unknown as IngestResult;
export const fixtureJournal = journalJson as unknown as BrainEvent[];
export const fixtureQuery = queryJson as unknown as QueryResult;
export const fixtureFill = fillJson as unknown as FillResult;
export const fixtureFillMissing = fillMissingJson as unknown as FillResult;
export const fixtureTemplates = templatesJson as unknown as Template[];
export const fixtureSources = sourcesJson as unknown as Source[];
export const fixtureRemoteHits = remoteHitsJson as unknown as RemoteHit[];
export const fixtureDbs = dbsJson as unknown as DbInfo[];
export const fixtureImportReport = importReportJson as unknown as ImportReport;
export const fixtureImportPartial = importPartialJson as unknown as ImportReport;
export const fixtureConfigResult = configResultJson as unknown as ConfigResult;
export const fixtureConfigErrors = configErrorsJson as unknown as ConfigResult;
export const fixtureConfigYaml = configYaml;
export const fixtureConfigErrorsYaml = configErrorsYaml;
export const fixtureServerStats = statsJson as unknown as ServerStats;
export const fixtureServerStatsRedis = statsRedisJson as unknown as ServerStats;
export const fixtureServerStatsBin = statsBinJson as unknown as ServerStats;
/** What a plain file backend actually returns: no indexes, no latency, no cache hit rate. */
export const fixtureServerStatsFile = statsFileJson as unknown as ServerStats;
export const fixtureRunMulti = runMultiJson as unknown as RunResult;
export const fixtureRunExplain = runExplainJson as unknown as RunResult;
export const fixtureRunError = runErrorJson as unknown as RunResult;
export const fixtureExplainError = explainErrorJson as unknown as ExplainResult;
export const fixtureExplainOk: ExplainResult = { ok: true, errors: [] };
export const fixtureConnectorSuggestions = connSuggJson as unknown as Inspect;
export const fixtureInspectEntities = inspectEntitiesJson as unknown as Inspect;
export const fixtureEntitySamples = entitySampleJson as unknown as { id: string; text: string; entities: Entity[] }[];
export const fixtureEntityRows = entitiesJson as unknown as EntityRow[];
export const fixturePatterns = patternsJson as unknown as Pattern[];
export const fixturePatternTest = patternsTestJson as unknown as { text: string; entities: Entity[] };
export const fixtureWhoamiAdmin = whoamiAdminJson as unknown as Whoami;
export const fixtureWhoamiKim = whoamiKimJson as unknown as Whoami;
type Rows = { columns: string[]; rows: Record<string, unknown>[] };
export const fixtureAccess = accessJson as unknown as { users: Rows; roles: Rows; grants: Rows; labels: Rows; explain: string };
/** A RunResult with a single rows statement (helper for SHOW … fixtures). */
export const rowsResult = (statement: string, r: Rows): RR => ({ ok: true, status: '[ok]', results: [{ index: 0, statement, kind: 'rows', columns: r.columns, rows: r.rows, elapsed_ms: 0.4 }] });
export const fixturePurpose = purposeJson as unknown as Purpose;
export const fixturePurposeChecks = purposeCheckJson as unknown as Record<'accept' | 'review' | 'reject' | 'forbidden', PurposeCheck>;
export const fixtureQuarantine = quarantineJson as unknown as QuarantineItem[];
export const fixtureContextXml = contextXml;
export const fixtureContextMd = contextMd;
export const fixtureBrainXml = brainXml;

/** A State with no focus: nothing is lit. */
export const fixtureStateUnlit: State = {
  ...fixtureState,
  focus: [],
  nodes: fixtureState.nodes.map(({ activation: _a, depth: _d, via: _v, ...n }) => n),
  stats: { ...fixtureState.stats, lit: 0, concepts: 0, tokens: 0, reach: 0, avgWeight: 0, byDepth: [] },
};

/** An empty brain. */
export const fixtureStateEmpty: State = {
  ...fixtureState,
  name: 'empty',
  goal: '',
  focus: [],
  goalFocus: [],
  nodes: [],
  edges: [],
  stats: { lit: 0, memories: 0, concepts: 0, tokens: 0, reach: 0, avgWeight: 0, superseded: 0, newLinks: 0, byDepth: [] },
  status: '[ok] step 0 · empty brain',
};

/** Synthetic big brain (for the >5000-node point-cloud mode). Deterministic. */
export function makeBigState(n = 8000): State {
  const kinds = ['fact', 'decision', 'task', 'question', 'note', 'record', 'chunk'];
  const sections = ['facts', 'decisions', 'tasks', 'questions', 'notes', 'orders', 'files'];
  const concepts = Array.from({ length: 40 }, (_, i) => ({
    id: `A:c${i}`, kind: 'concept', section: 'concepts', text: `Concept ${i}`, base: 0.8, weight: 0.8, age: 0, pinned: false,
  }));
  let seed = 7;
  const rnd = () => ((seed = (seed * 16807) % 2147483647) / 2147483647);
  const nodes = [
    ...concepts,
    ...Array.from({ length: n }, (_, i) => {
      const k = i % kinds.length;
      const lit = i < 60;
      return {
        id: `m${i}`, kind: kinds[k], section: sections[k], text: `Synthetic memory ${i} about concept ${i % 40}`,
        base: 0.5, weight: +(0.05 + rnd() * 0.7).toFixed(2), age: Math.round(rnd() * 300), pinned: false,
        ...(lit ? { activation: +(1 - i / 70).toFixed(2), depth: i < 3 ? 0 : i < 20 ? 1 : 2, via: i < 3 ? undefined : `m${i % 3}` } : {}),
      };
    }),
  ];
  const edges = nodes.slice(concepts.length).flatMap((m, i) => [
    { a: m.id, b: `A:c${i % 40}`, w: 0.7, type: 'concept' as const },
    ...(i > 0 ? [{ a: m.id, b: `m${Math.floor(rnd() * i)}`, w: +(0.2 + rnd() * 0.5).toFixed(2), type: 'assoc' as const }] : []),
  ]);
  return {
    ...fixtureState,
    name: 'big (synthetic)',
    focus: ['m0', 'm1', 'm2'],
    nodes,
    edges,
    stats: { ...fixtureState.stats, lit: 60, memories: n, concepts: 0, tokens: 2100, reach: 2, byDepth: [3, 17, 40] },
  };
}

/** Synthetic query result with many rows (virtualised grid story). */
export function makeBigQuery(rows = 20000) {
  const columns = ['id', 'kind', 'text', 'weight', 'fields.stage', 'fields.value', 'fields.owner'];
  const stages = ['lead', 'qualified', 'proposal', 'negotiation', 'won', 'lost'];
  const owners = ['Kim Lee', 'Ana Ruiz', 'Tom Berg', 'Ola Nowak'];
  return {
    columns,
    rows: Array.from({ length: rows }, (_, i) => ({
      id: `r${100000 + i}`, kind: 'record', text: `Deal ${i}: ${stages[i % 6]} with account ${i % 977}`,
      weight: +((i * 37) % 100 / 100).toFixed(2), 'fields.stage': stages[i % 6], 'fields.value': String(((i * 7919) % 90000) + 1000), 'fields.owner': owners[i % 4],
    })),
    count: rows,
    total: rows,
    rendered: '(rendered output omitted for the synthetic result)',
  };
}
