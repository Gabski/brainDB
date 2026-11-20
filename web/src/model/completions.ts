// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { CompletionItem } from '../components/common/CodeEditor';
import type { Template } from '../api/types';
import { BDQL_KEYWORDS } from './highlight';
import { isConceptNode, type GraphIndex } from './graph';

export const ENTITY_TYPES = ['date', 'datetime', 'duration', 'money', 'percent', 'number_unit', 'email', 'phone', 'url', 'domain', 'person', 'org', 'iban', 'vat', 'card', 'ip', 'version', 'id_code', 'hashtag', 'mention', 'geo', 'address'];

/** Snippet library: examples of every statement family (from the SPEC, adapted to this brain). */
export const BDQL_SNIPPETS: { name: string; group: string; code: string }[] = [
  { group: 'read', name: 'Context for a question', code: 'CONTEXT FOR "guest access policy" DEPTH 2 FORMAT md;' },
  { group: 'read', name: 'Select records', code: "SELECT id, text, fields.wave, fields.users\n  FROM memory\n  WHERE kind = record AND fields.users > 100\n  ORDER BY fields.users DESC\n  LIMIT 10;" },
  { group: 'read', name: 'Near + filters', code: 'SELECT id, kind, text, score FROM memory\n  WHERE NOT superseded AND weight >= 0.2\n  NEAR "token limits" MODE hybrid\n  ORDER BY score DESC LIMIT 8;' },
  { group: 'read', name: 'Group by', code: 'SELECT COUNT(*), kind FROM memory GROUP BY kind;' },
  { group: 'read', name: 'Search (federated)', code: 'SEARCH "conditional access for guests" MODE hybrid LIMIT 5 SOURCES *;' },
  { group: 'read', name: 'Inspect / test / paths', code: 'INSPECT d121 WITH item, links, near;\nTEST d121;\nPATHS FROM q141 TO d125 LIMIT 3;' },
  { group: 'read', name: 'Explain', code: 'EXPLAIN SELECT id, text, score FROM memory@*\n  WHERE kind IN (fact, decision) AND lit\n  NEAR "token limits" ORDER BY score DESC LIMIT 5;' },
  { group: 'read', name: 'Compact query', code: "QUERY 'kind:record users>100 sort:-users limit:10' FORMAT md;" },
  { group: 'write', name: 'Remember + link', code: 'REMEMBER task "Block guest sign-in until CA policy is verified"\n  CONCEPTS "Security" LINK TO k175 WEIGHT 0.8 FOCUS AS T;\nBOOST d125;' },
  { group: 'write', name: 'Insert records', code: "INSERT INTO record (text, wave, users, owner)\n  VALUES ('wave: partners', 'partners', 300, 'Ana Ruiz');" },
  { group: 'write', name: 'Update (dry run)', code: "UPDATE memory SET fields.owner = 'Kim Lee' WHERE kind = record AND fields.owner = 'Ana Ruiz' DRY RUN;" },
  { group: 'write', name: 'Bulk forget (dry run)', code: 'FORGET WHERE kind = note AND age > 90 AND NOT pinned DRY RUN;' },
  { group: 'write', name: 'Focus and goal', code: 'FOCUS d121, d123;\nGOAL "Prepare the SSO login rollout plan" FOCUS d121;' },
  { group: 'docs', name: 'Fill a template', code: "FILL TEMPLATE 'rollout-brief.yaml' OUTPUT json WITH SOURCES MIN 0.2;" },
  { group: 'docs', name: 'Inline template', code: 'FILL $$\nproject: "project name"\ndeadline: "SSO rollout deadline:date"\n$$ AS yaml OUTPUT md;' },
  { group: 'connectors', name: 'Anchor connector', code: "CONNECTOR anchor \"Kim Lee\" KEY 'owner:kim-lee' CONNECT r191, r193, n150 GAIN 0.95 TRANSPARENT AS H;" },
  { group: 'connectors', name: 'Alias / hub', code: "CONNECTOR alias \"MFA\" TERMS '2FA', 'two-factor';\nCONNECTOR hub \"SSO rollout waves\" CONNECT r191, r192, r193;" },
  { group: 'connectors', name: 'Resolve + suggest', code: 'RESOLVE person BY fields.owner MIN 2 DRY RUN;\nSUGGEST CONNECTORS LIMIT 10;\nSELECT id, type, label, key, degree FROM connectors ORDER BY degree DESC;' },
  { group: 'entities', name: 'Entity stats', code: 'SHOW patterns;\nSELECT type, COUNT(*) FROM entities GROUP BY type;' },
  { group: 'entities', name: 'Extract + query', code: 'EXTRACT FROM "Call Kim Aalto (+47 22 33 44 55) before 15 Dec 2026 about €48k";\nSELECT id, text FROM memory WHERE HAS email LIMIT 10;' },
  { group: 'entities', name: 'Custom pattern', code: "PATTERN ADD sku '\\bSKU-\\d{5}\\b' TYPE id_code NORMALIZE upper PII false;\nPATTERN TEST sku 'Order SKU-10422 and SKU-1';" },
  { group: 'purpose', name: 'Purpose and check', code: 'SHOW PURPOSE;\nCHECK "Lunch menu for Friday";\nCHECK "Guest accounts bypass conditional access" AS risk IN risks;' },
  { group: 'purpose', name: 'Review queue', code: "SELECT id, text, purpose.score, purpose.reasons FROM quarantine ORDER BY purpose.score DESC LIMIT 20;\nADMIT n401 IN notes;\nREJECT f402 REASON 'off-topic';" },
  { group: 'purpose', name: 'Override / strict', code: "REMEMBER note \"Offsite logistics for the SSO demo\" OVERRIDE PURPOSE REASON 'needed for the demo plan';\nSTRICT PURPOSE;" },
  { group: 'admin', name: 'Show / sources / kinds', code: "SHOW kinds;\nSHOW sources;\nSOURCE ADD lab 'http://lab:7791' TOKEN ENV LAB_TOKEN WEIGHT 0.7 DEPTH 1;\nKIND ADD customer COLOR '#ff8787' HALF LIFE 90 BASE 0.7 PREFIX u SECTION customers;" },
  { group: 'admin', name: 'Transaction', code: 'BEGIN;\nREMEMBER fact "Pilot wave confirmed" AS P;\nLINK $P TO r191 WEIGHT 0.8;\nCOMMIT;' },
];

const MULTI = ['SHOW PURPOSE', 'FROM quarantine', 'OVERRIDE PURPOSE REASON', 'STRICT PURPOSE', 'ORDER BY', 'GROUP BY', 'DRY RUN', 'HALF LIFE', 'SUGGEST CONNECTORS', 'SHOW patterns', 'SHOW pii', 'SHOW connectors', 'FROM entities', 'TOKEN ENV', 'WITH SOURCES', 'PATTERN ADD', 'PATTERN TEST', 'PATTERN REMOVE', 'FOCUS GOAL', 'SOURCE ADD', 'SOURCE SYNC', 'KIND ADD', 'FILL TEMPLATE'];

/** Autocomplete items for the BDQL editor: keywords, ids, kinds, sections, concepts, fields, templates, variables. */
export function bdqlCompletions(g: GraphIndex | null, templates: Template[] = [], script = ''): CompletionItem[] {
  const out: CompletionItem[] = [];
  for (const k of BDQL_KEYWORDS) out.push({ label: k, kind: 'keyword' });
  for (const k of MULTI) out.push({ label: k, kind: 'keyword' });
  for (const p of ['lit', 'pinned', 'superseded', 'external', 'score', 'connector', 'route', 'memory', 'concepts', 'links', 'connectors', 'entities'])
    out.push({ label: p, kind: 'field', detail: 'pseudo-column / table' });
  for (const c of ['purpose.score', 'purpose.decision', 'purpose.flag', 'purpose.reasons']) out.push({ label: c, kind: 'field', detail: 'admission pseudo-column' });
  out.push({ label: 'quarantine', kind: 'field', detail: 'pseudo-table (reviewers)' });
  for (const t of ENTITY_TYPES) {
    out.push({ label: 'entities.' + t, kind: 'field', detail: 'entity column' });
  }
  for (const m of script.matchAll(/\bAS\s+([A-Za-z_]\w*)|\bLET\s+\$([A-Za-z_]\w*)/gi)) out.push({ label: '$' + (m[1] ?? m[2]), kind: 'var', detail: 'variable' });
  if (g) {
    for (const k of g.kinds.values()) out.push({ label: k.name, kind: 'kind', detail: `kind · half-life ${k.halfLife ?? '∞'}` });
    for (const s of g.state.sections) out.push({ label: s, kind: 'section', detail: 'section' });
    const fields = new Set<string>();
    for (const n of g.state.nodes) {
      if (n.fields) Object.keys(n.fields).forEach((f) => fields.add(f));
      if (isConceptNode(n)) {
        out.push({ label: n.id, kind: 'concept', detail: '◆ ' + n.text });
        out.push({ label: `'${n.text}'`, kind: 'concept', detail: n.id });
      } else out.push({ label: n.id, kind: 'id', detail: n.text });
    }
    for (const f of fields) out.push({ label: 'fields.' + f, kind: 'field', detail: 'record field' });
    for (const c of g.state.connectors ?? []) out.push({ label: c.id, kind: 'id', detail: `⧉ ${c.type} · ${c.label}` });
    for (const src of new Set(g.state.nodes.filter((n) => n.external).map((n) => n.source ?? ''))) if (src) out.push({ label: 'memory@' + src, kind: 'field', detail: 'federated source' });
  }
  for (const t of templates) out.push({ label: `'${t.name}'`, kind: 'template', detail: t.format });
  for (const s of BDQL_SNIPPETS) out.push({ label: s.name, insert: s.code, kind: 'snippet', detail: s.group });
  return out;
}

/** Completions for the compact query syntax (A2). */
export function compactCompletions(g: GraphIndex | null): CompletionItem[] {
  const out: CompletionItem[] = ['kind:', 'section:', 'concept:', 'lit:true', 'pinned:', 'superseded:false', 'w>=', 'age<=', 'near:', 'sort:', 'limit:', 'select:', 'source:'].map((k) => ({ label: k, kind: 'keyword' as const }));
  if (g) {
    for (const k of g.kinds.values()) out.push({ label: 'kind:' + k.name, kind: 'kind' });
    const fields = new Set<string>();
    g.state.nodes.forEach((n) => n.fields && Object.keys(n.fields).forEach((f) => fields.add(f)));
    for (const f of fields) out.push({ label: f, kind: 'field', detail: 'record field' });
  }
  return out;
}
