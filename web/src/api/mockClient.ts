// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { ApiClient, LiveStatus } from './client';
import { ApiError } from './client';
import type { BrainEvent, ContextQuery, Entity, Pattern, QueryFormat, SearchMode, ServerStats, State } from './types';
import {
  fixtureBrainXml,
  fixtureContextMd,
  fixtureContextXml,
  fixtureDiscover,
  fixtureFill,
  fixtureFillMissing,
  fixtureIngest,
  fixtureInspect,
  fixtureInspectRecord,
  fixtureJournal,
  fixtureLayoutEmbed,
  fixtureLayoutTree,
  fixtureQuery,
  fixtureRemoteHits,
  fixtureSources,
  fixtureDbs,
  fixtureImportReport,
  fixtureImportPartial,
  fixtureConfigResult,
  fixtureConfigErrors,
  fixtureConfigYaml,
  fixtureServerStats,
  fixtureConnectorSuggestions,
  fixtureWhoamiAdmin,
  fixturePurpose,
  fixturePurposeChecks,
  fixtureQuarantine,
  fixtureWhoamiKim,
  fixtureAccess,
  rowsResult,
  fixtureInspectEntities,
  fixtureEntityRows,
  fixturePatterns,
  fixturePatternTest,
  fixtureRunMulti,
  fixtureRunExplain,
  fixtureRunError,
  fixtureExplainError,
  fixtureExplainOk,
  fixtureSearch,
  fixtureState,
  fixtureTemplates,
  fixtureVector,
} from '../fixtures';

export interface MockOptions {
  /** Called for every API call (Storybook passes `action('api')`). */
  log?: (method: string, ...args: unknown[]) => void;
  /** Artificial latency in ms. */
  delay?: number;
  /** `true`: every call fails. A string: only that method fails. */
  fail?: boolean | string;
  /** Calls never resolve (loading state). */
  hang?: boolean;
  /** Initial state (defaults to the fixture). */
  state?: State;
  /** Every fill returns the variant with missing slots. */
  fillMissing?: boolean;
  /** Import returns the report with errors. */
  importPartial?: boolean;
  /** Config validation returns errors. */
  configErrors?: boolean;
  /** Principal returned by /api/whoami (default admin). */
  user?: 'admin' | 'kim';
  /** No purpose configured (filter off). */
  noPurpose?: boolean;
  /** Write statements fail with "access denied". */
  writeDenied?: boolean;
  /** Storage stats returned by /api/stats. */
  stats?: ServerStats;
  /** /api/storage/convert answers 404. */
  convertUnsupported?: boolean;
  /** Emit a fake agent event every N ms on the SSE stream (0 = never). */
  eventEvery?: number;
}

/** In-memory client over the fixtures. Mutations only log; queries return fixtures. */
export function createMockClient(opts: MockOptions = {}): ApiClient {
  const log = opts.log ?? (() => {});
  let state = opts.state ?? fixtureState;
  let sources = fixtureSources;
  let dbs = fixtureDbs;
  let viewAs: string | null = null;
  let purpose = opts.noPurpose ? null : fixturePurpose;
  let quarantine = fixtureQuarantine;
  let patterns = fixturePatterns;
  let stats = opts.stats ?? fixtureServerStats;
  let activeDb: string | null = fixtureDbs[0]?.name ?? null;
  let progressCb: ((ev: BrainEvent) => void) | null = null;
  const call = <T>(method: string, args: unknown[], value: () => T): Promise<T> => {
    log(method, ...args);
    if (opts.hang) return new Promise<T>(() => {});
    if (opts.fail === true || opts.fail === method)
      return Promise.reject(new ApiError(`mock: ${method} failed (op#1 link: no such item x999)`, 400));
    return new Promise<T>((res) => setTimeout(() => res(value()), opts.delay ?? 60));
  };
  const withStatus = (s: State) => s;
  /** Non-admin principal: drop nodes whose labels are not all held (and their edges), as the server would. */
  const visible = (st: State): State => {
    const who = viewAs ? fixtureWhoamiKim : opts.user === 'kim' ? fixtureWhoamiKim : null;
    if (!who) return st;
    const held = new Set(who.labels);
    const nodes = st.nodes.filter((n) => (n.labels ?? []).every((l) => held.has(l))).map((n) => ({ ...n, labels: n.labels?.filter((l) => held.has(l)) }));
    const ids = new Set(nodes.map((n) => n.id));
    const edges = st.edges.filter((e) => (ids.has(e.a) || e.a.startsWith('H:')) && (ids.has(e.b) || e.b.startsWith('H:')));
    const mem = nodes.filter((n) => n.kind !== 'concept' && !n.external);
    return { ...st, nodes, edges, focus: st.focus.filter((f) => ids.has(f)), stats: { ...st.stats, memories: mem.length, lit: mem.filter((n) => n.activation !== undefined).length } };
  };

  return {
    getState: () => call('getState', [], () => visible(state)),
    setContext: (p) =>
      call('setContext', [p], () => {
        state = withStatus({
          ...state,
          params: {
            depth: p.depth ?? state.params.depth,
            threshold: p.threshold ?? state.params.threshold,
            hop: p.hop ?? state.params.hop,
            showSuperseded: p.showSuperseded ?? state.params.showSuperseded,
          },
          focus: p.focus ?? (p.goalFocus ? state.goalFocus : [...new Set([...state.focus, ...(p.addFocus ?? [])])].filter((x) => !(p.removeFocus ?? []).includes(x))),
          goal: p.goal ?? state.goal,
        });
        return state;
      }),
    embeddingProviders: () =>
      call('embeddingProviders', [], () => [
        { name: 'hash', label: 'Built-in hash-256 (offline, lexical)', wire: 'hash', needsKey: false, note: 'offline, deterministic and free, but it matches word shapes, not meaning' },
        { name: 'openai', label: 'OpenAI', wire: 'openai', baseUrl: 'https://api.openai.com/v1', keyEnv: 'OPENAI_API_KEY', needsKey: true, maxBatch: 2048, model: 'text-embedding-3-small' },
        { name: 'openrouter', label: 'OpenRouter', wire: 'openai', baseUrl: 'https://openrouter.ai/api/v1', keyEnv: 'OPENROUTER_API_KEY', needsKey: true, maxBatch: 512, model: 'openai/text-embedding-3-small' },
        { name: 'cohere', label: 'Cohere', wire: 'cohere', baseUrl: 'https://api.cohere.com', keyEnv: 'COHERE_API_KEY', needsKey: true, maxBatch: 96, inputType: 'search_document', model: 'embed-v4.0' },
        { name: 'gemini', label: 'Google Gemini', wire: 'gemini', baseUrl: 'https://generativelanguage.googleapis.com/v1beta', keyEnv: 'GEMINI_API_KEY', needsKey: true, maxBatch: 100, inputType: 'RETRIEVAL_DOCUMENT', auth: 'google', model: 'text-embedding-004' },
        { name: 'ollama', label: 'Ollama (local)', wire: 'openai', baseUrl: 'http://127.0.0.1:11434/v1', needsKey: false, maxBatch: 64, model: 'nomic-embed-text' },
      ]),
    setEmbedding: (p) =>
      call('setEmbedding', [p], () => {
        state = { ...state, embedding: { ...state.embedding, ...p } };
        return state;
      }),
    exec: (ops) =>
      (opts.writeDenied ? Promise.reject(new ApiError('access denied: write on ' + ((ops[0] as { id?: string; a?: string }).id ?? (ops[0] as { a?: string }).a ?? 'node') + ' requires write grant for label "security"', 403)) : null) ??
      call('exec', [ops], () => ({
        changes: ops.map((o) => `${o.op} ok`),
        touched: [],
        auto: [],
        status: state.status,
      })),
    search: (q) => call('search', [q], () => fixtureSearch[(q.mode ?? 'hybrid') as SearchMode] ?? fixtureSearch.hybrid),
    inspect: (q) =>
      call('inspect', [q], () => {
        if (q.what.includes('discover')) return fixtureDiscover;
        if (q.what.includes('connectors')) return fixtureConnectorSuggestions;
        const base =
          q.id === 'n154' ? fixtureInspectEntities : q.id && state.nodes.find((n) => n.id === q.id)?.kind === 'record' ? fixtureInspectRecord : fixtureInspect;
        const node = state.nodes.find((n) => n.id === q.id);
        const out: typeof base = {};
        for (const w of q.what) {
          if (w === 'item' && base.item) out.item = node ? { ...base.item, node: { ...node, entities: base.item.node.id === node.id ? base.item.node.entities : undefined } } : base.item;
          else if (w !== 'item' && w !== 'discover') (out as Record<string, unknown>)[w] = (fixtureInspect as Record<string, unknown>)[w] ?? (base as Record<string, unknown>)[w];
        }
        if (q.what.includes('links') && base.links) out.links = base.links;
        if (q.what.includes('near') && base.near) out.near = base.near;
        return out;
      }),
    layout: (view, root) => call('layout', [view, root], () => (view === 'tree' ? fixtureLayoutTree : fixtureLayoutEmbed)),
    vector: (id) => call('vector', [id], () => fixtureVector),
    ingest: (files, focus) => call('ingest', [files.map((f) => f.name), focus], () => fixtureIngest),
    addKind: (k) =>
      call('addKind', [k], () => {
        state = {
          ...state,
          kinds: [
            ...state.kinds,
            { name: k.name, label: k.name, color: k.color, base: k.base ?? 0.5, halfLife: k.halfLife, prefix: k.name[0] ?? 'k', section: k.name, builtin: false },
          ],
        };
        return state;
      }),
    contextText: (f) => call('contextText', [f], () => (f === 'md' ? fixtureContextMd : fixtureContextXml)),
    getBrainXml: () => call('getBrainXml', [], () => fixtureBrainXml),
    putBrainXml: (xml) => call('putBrainXml', [xml.length + ' chars'], () => state),
    journal: () => call('journal', [], () => fixtureJournal),
    query: (q, format?: QueryFormat) => call('query', [q, format], () => fixtureQuery),
    contextQuery: (q: ContextQuery) =>
      call('contextQuery', [q], () => ({
        output:
          q.format === 'md'
            ? fixtureContextMd
            : q.format === 'xml'
              ? fixtureContextXml
              : q.format === 'json'
                ? JSON.stringify({ goal: state.goal, focus: state.focus, items: state.nodes.filter((n) => n.activation !== undefined).slice(0, 6) }, null, 2)
                : `goal: ${state.goal}\nfocus:\n${state.focus.map((f) => `  - ${f}`).join('\n')}\n`,
        format: q.format,
      })),
    fill: (req) => call('fill', [req], () => (opts.fillMissing ? fixtureFillMissing : fixtureFill)),
    templates: () => call('templates', [], () => fixtureTemplates),
    sources: () => call('sources', [], () => sources),
    addSource: (src) =>
      call('addSource', [src], () => {
        sources = [
          ...sources.filter((s) => s.name !== src.name),
          { name: src.name, uri: src.uri, weight: src.weight ?? 0.7, mode: src.mode ?? 'live', ttl: src.ttl ?? 300, federateDepth: src.federateDepth ?? 1, enabled: src.enabled ?? true, status: 'ok', latencyMs: 12, nodes: 0 },
        ];
        return sources;
      }),
    removeSource: (name) =>
      call('removeSource', [name], () => {
        sources = sources.filter((s) => s.name !== name);
      }),
    syncSource: (name) => call('syncSource', [name], () => sources),
    remoteSearch: (name, q) => call('remoteSearch', [name, q], () => fixtureRemoteHits.map((h) => ({ ...h, id: name + ':' + h.id.split(':').pop() }))),
    dbs: () => call('dbs', [], () => dbs),
    createDb: (name, config) =>
      call('createDb', [name, config], () => {
        dbs = [...dbs, { name, nodes: 0, edges: 0, step: 0, sizeBytes: 0, updated: new Date().toISOString(), goal: '' }];
        return dbs;
      }),
    examples: () =>
      call('examples', [], () => [
        { name: 'sales-company', description: 'Tessaline sales memory', kind: 'seed' },
        { name: 'research-lab', description: 'Electrolyte lab memory', kind: 'seed' },
        { name: 'import-demo', description: 'A raw pile turned into a database', kind: 'import' },
      ]),
    installExample: (name, example) =>
      call('installExample', [name, example], () => {
        dbs = [...dbs, { name, nodes: 12, edges: 4, step: 1, sizeBytes: 4096, updated: new Date().toISOString(), goal: example }];
        return { name, example };
      }),
    deleteDb: (name) =>
      call('deleteDb', [name], () => {
        dbs = dbs.filter((d) => d.name !== name);
      }),
    importFiles: (files, o) =>
      call('importFiles', [files.map((f) => f.path), o], () => {
        const rep = opts.importPartial ? fixtureImportPartial : fixtureImportReport;
        if (!o.dryRun && progressCb) {
          const total = rep.files.length;
          rep.files.forEach((f, i) => setTimeout(() => progressCb?.({ type: 'import.progress', ts: Date.now(), by: 'ui', tool: 'import', summary: f.path, ids: [], db: o.db, done: i + 1, total, file: f.path }), 150 * (i + 1)));
        }
        return { ...rep, dryRun: !!o.dryRun };
      }),
    getConfig: (db) => call('getConfig', [db], () => ({ yaml: fixtureConfigYaml, result: fixtureConfigResult })),
    putConfig: (yaml, o) => call('putConfig', [yaml.length + ' chars', o], () => (opts.configErrors || /enrich\b|kindz/.test(yaml) ? fixtureConfigErrors : fixtureConfigResult)),
    setDb: (name) => {
      log('setDb', name);
      activeDb = name;
    },
    getDb: () => activeDb,
    run: (script, o) =>
      call('run', [script, o], () => {
        const sh = script.match(/SHOW\s+(users|roles|grants|labels)/i);
        if (sh) return rowsResult(sh[0], fixtureAccess[sh[1].toLowerCase() as 'users']);
        if (/EXPLAIN\s+ACCESS/i.test(script))
          return { ok: true, status: '[ok]', results: [{ index: 0, statement: script.trim(), kind: 'text' as const, text: fixtureAccess.explain, elapsed_ms: 0.2 }] };
        if (opts.writeDenied && /\b(REMEMBER|LINK|BOOST|WEAKEN|PIN|UPDATE|FORGET|SUPERSEDE|GRANT|LABEL|CONNECT)\b/i.test(script))
          return { ok: false, status: '[error] nothing applied', results: [], error: { statement: 1, line: 1, col: 1, message: 'access denied: write on k175 requires write grant for label "security"' } };
        if (/\b(GRANT|REVOKE|DENY|CREATE|ALTER|DROP)\b/i.test(script))
          return { ok: true, status: '[ok] access config saved', results: [{ index: 0, statement: script.trim(), kind: 'changes' as const, changes: ['access: ' + script.trim()], ids: [], elapsed_ms: 1.1 }] };
        if (/selekt/i.test(script)) return fixtureRunError;
        if (/^\s*explain/im.test(script) && !/;\s*\S/.test(script.trim().replace(/;\s*$/, ''))) return fixtureRunExplain;
        const n = script.split(';').filter((x) => x.replace(/--.*$/gm, '').trim()).length;
        return { ...fixtureRunMulti, results: fixtureRunMulti.results.slice(0, Math.max(1, Math.min(n, fixtureRunMulti.results.length))) };
      }),
    explain: (script) => call('explain', [script.length + ' chars'], () => (/selekt|\btsk\b/i.test(script) ? fixtureExplainError : fixtureExplainOk)),
    patterns: () => call('patterns', [], () => patterns),
    testPatterns: (text, types) => call('testPatterns', [text, types], () => mockExtract(text, patterns)),
    entities: (q) => call('entities', [q], () => fixtureEntityRows.filter((r) => (!q?.type || r.type === q.type) && (!q?.value || r.value === q.value))),
    setPatternEnabled: (name, enabled) =>
      call('setPatternEnabled', [name, enabled], () => {
        patterns = patterns.map((p) => (p.name === name ? { ...p, enabled } : p));
        return patterns;
      }),
    health: () => call('health', [], () => ({ ok: true, version: '0.9.0-dev', storage: stats.storage, format: stats.format })),
    convert: (to) =>
      call('convert', [to], () => {
        if (opts.convertUnsupported) return { unsupported: true };
        stats = { ...stats, format: to };
        return { ok: true, to };
      }),
    stats: () => call('stats', [], () => stats),
    getPurpose: () => call('getPurpose', [], () => purpose),
    putPurpose: (p) =>
      call('putPurpose', [p], () => {
        purpose = p;
        return p;
      }),
    checkPurpose: (q) =>
      call('checkPurpose', [q], () => {
        const t = q.text.toLowerCase();
        if (!t.trim()) return { ...fixturePurposeChecks.reject, score: 0, rules: ['empty text → rejected'] };
        if (/\b\d{4}[ -]?\d{4}[ -]?\d{4}[ -]?\d{4}\b/.test(t)) return fixturePurposeChecks.forbidden;
        if (/lunch|menu|cat|football|pizza|movie/.test(t)) return fixturePurposeChecks.reject;
        if (/sso|entra|mfa|oidc|guest|rollout|wave|deadline|token|login|security|risk/.test(t)) return fixturePurposeChecks.accept;
        return fixturePurposeChecks.review;
      }),
    quarantine: () => call('quarantine', [], () => quarantine),
    admit: (ids, section) =>
      call('admit', [ids, section], () => {
        quarantine = quarantine.filter((q) => !ids.includes(q.id));
        return { ok: true };
      }),
    reject: (ids, reason) =>
      call('reject', [ids, reason], () => {
        quarantine = quarantine.filter((q) => !ids.includes(q.id));
        return { ok: true };
      }),
    whoami: () => call('whoami', [], () => (viewAs ? { ...fixtureWhoamiKim, name: viewAs } : opts.user === 'kim' ? fixtureWhoamiKim : fixtureWhoamiAdmin)),
    setViewAs: (u) => {
      log('setViewAs', u);
      viewAs = u;
    },
    getViewAs: () => viewAs,
    snapshot: () => call('snapshot', [], () => ({ ok: true })),
    migrate: (to) => call('migrate', [to], () => ({ ok: true, to })),
    subscribe: (onEvent: (ev: BrainEvent) => void, onStatus: (s: LiveStatus) => void) => {
      onStatus(opts.fail === true ? 'offline' : 'live');
      progressCb = onEvent;
      if (!opts.eventEvery) return () => {};
      let i = 0;
      const t = setInterval(() => {
        const src = fixtureJournal[i++ % fixtureJournal.length];
        onEvent({ ...src, ts: Date.now() });
      }, opts.eventEvery);
      return () => clearInterval(t);
    },
  };
}

/** Very small client-side extractor so the pattern tester story reacts to typing. */
function mockExtract(text: string, patterns: Pattern[]): Entity[] {
  if (text === fixturePatternTest.text) return fixturePatternTest.entities;
  const on = new Set(patterns.filter((p) => p.enabled).map((p) => p.name));
  const rules: [string, RegExp, (m: string) => string, boolean?][] = [
    ['date', /\b\d{4}-\d{2}-\d{2}\b/g, (m) => m],
    ['money', /[€$£]\s?\d[\d,. ]*\d|\b\d[\d,. ]*\s?(?:EUR|USD|PLN|NOK)\b/g, (m) => m.replace(/\s/g, '')],
    ['percent', /\b\d+(?:\.\d+)?\s?%/g, (m) => String(parseFloat(m) / 100)],
    ['email', /\b[\w.+-]+@[\w-]+(?:\.[\w-]+)+\b/g, (m) => m.toLowerCase(), true],
    ['phone', /\+\d{2}(?:[ -]?\d{2,3}){3,4}/g, (m) => m.replace(/[ -]/g, ''), true],
    ['url', /\bhttps?:\/\/[^\s,)]+/g, (m) => m],
    ['id_code', /\b(?:SKU|INV|R)-\d+(?:-\d+)?\b|#\d{3,}/g, (m) => m.toUpperCase()],
    ['person', /\b(?:Dr\.\s|Ms\.\s|Mr\.\s)?[A-Z][a-z]+\s[A-Z][a-z]+\b/g, (m) => m, true],
    ['version', /\bv\d+\.\d+(?:\.\d+)?\b/g, (m) => m.slice(1)],
  ];
  const out: Entity[] = [];
  for (const [type, re, norm, pii] of rules) {
    if (!on.has(type)) continue;
    for (const m of text.matchAll(re)) {
      const start = m.index ?? 0;
      if (out.some((e) => start < e.end && start + m[0].length > e.start)) continue;
      out.push({ type, raw: m[0], value: norm(m[0]), start, end: start + m[0].length, confidence: 0.9, pii: !!pii });
    }
  }
  for (const p of patterns.filter((x) => !x.builtin && x.enabled && x.regex)) {
    try {
      for (const m of text.matchAll(new RegExp(p.regex!, 'g'))) {
        const start = m.index ?? 0;
        if (!out.some((e) => start < e.end && start + m[0].length > e.start)) out.push({ type: p.type, raw: m[0], value: m[0].toUpperCase(), start, end: start + m[0].length, confidence: 1, pii: p.pii });
      }
    } catch {
      /* invalid custom regex */
    }
  }
  return out.sort((a, b) => a.start - b.start);
}
