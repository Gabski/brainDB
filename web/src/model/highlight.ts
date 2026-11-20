// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

// Tiny dependency-free tokenizers for syntax highlighting: BDQL, compact query, JSON, YAML, XML, Markdown.
export type Lang = 'bdql' | 'compact' | 'json' | 'yaml' | 'xml' | 'md' | 'csv' | 'text';
export interface Token {
  t: string;
  c?: string; // css class without "tk-" prefix
}

export const BDQL_KEYWORDS = [
  'USE', 'SHOW', 'CONTEXT', 'FOCUS', 'FOR', 'DEPTH', 'THRESHOLD', 'HOP', 'SOURCES', 'FORMAT', 'SET', 'SELECT', 'FROM', 'WHERE', 'AND', 'OR',
  'NOT', 'IN', 'NEAR', 'MODE', 'ORDER', 'BY', 'ASC', 'DESC', 'LIMIT', 'OFFSET', 'GROUP', 'COUNT', 'SUM', 'AVG', 'MIN', 'MAX', 'SEARCH',
  'INSPECT', 'WITH', 'TEST', 'SUGGEST', 'DISCOVER', 'PATHS', 'TO', 'EXPLAIN', 'REMEMBER', 'CONCEPTS', 'LINK', 'WEIGHT', 'FIELDS',
  'SUPERSEDES', 'PIN', 'UNPIN', 'AS', 'INSERT', 'INTO', 'VALUES', 'UPDATE', 'CONCEPT', 'MEMBERS', 'RELATED', 'TYPE', 'UNLINK', 'BOOST',
  'WEAKEN', 'SUPERSEDE', 'FORGET', 'FORCE', 'DRY', 'RUN', 'ADD', 'REMOVE', 'GOAL', 'FILL', 'TEMPLATE', 'OUTPUT', 'SCOPE', 'INGEST',
  'IMPORT', 'SOURCE', 'SYNC', 'TOKEN', 'ENV', 'KIND', 'COLOR', 'HALF', 'LIFE', 'BASE', 'PREFIX', 'SECTION', 'BEGIN', 'COMMIT', 'ROLLBACK',
  'LET', 'QUERY', 'CONFIRM', 'TRUE', 'FALSE', 'IS', 'NULL',
  // Addendum H (connectors) and I (entities / patterns)
  'CONNECTOR', 'CONNECTORS', 'CONNECT', 'DISCONNECT', 'RESOLVE', 'KEY', 'GAIN', 'TRANSPARENT', 'TERMS', 'EXTRACT', 'PATTERN', 'PATTERNS',
  'HAS', 'BETWEEN', 'NORMALIZE', 'PII', 'ENTITIES',
  // Addendum O (purpose / admission)
  'PURPOSE', 'CHECK', 'ADMIT', 'REJECT', 'OVERRIDE', 'REASON', 'STRICT', 'QUARANTINE',
];
const KW = new Set(BDQL_KEYWORDS);
const PSEUDO = new Set(['quarantine', 'lit', 'pinned', 'superseded', 'external', 'score', 'connector', 'route', 'entities', 'anchor', 'bridge', 'hub', 'alias', 'time', 'type', 'value', 'memory', 'concepts', 'links', 'kinds', 'sections', 'sources', 'dbs', 'stats', 'config', 'templates', 'hybrid', 'vector', 'tree', 'graph', 'md', 'json', 'yaml', 'xml', 'csv', 'context', 'federated']);

function bdql(src: string): Token[] {
  const out: Token[] = [];
  let i = 0;
  const push = (t: string, c?: string) => out.push({ t, c });
  while (i < src.length) {
    const ch = src[i];
    const rest = src.slice(i);
    let m: RegExpMatchArray | null;
    if (rest.startsWith('--')) {
      const e = src.indexOf('\n', i);
      const end = e < 0 ? src.length : e;
      push(src.slice(i, end), 'com');
      i = end;
    } else if (rest.startsWith('$$')) {
      const e = src.indexOf('$$', i + 2);
      const end = e < 0 ? src.length : e + 2;
      push(src.slice(i, end), 'str');
      i = end;
    } else if (ch === "'" || ch === '"') {
      let j = i + 1;
      while (j < src.length && src[j] !== ch) j += src[j] === '\\' ? 2 : 1;
      push(src.slice(i, j + 1), 'str');
      i = j + 1;
    } else if ((m = rest.match(/^\$[A-Za-z_]\w*/))) {
      push(m[0], 'attr');
      i += m[0].length;
    } else if ((m = rest.match(/^-?\d+(\.\d+)?%?/)) && !/[\w:]/.test(src[i - 1] ?? '')) {
      push(m[0], 'num');
      i += m[0].length;
    } else if ((m = rest.match(/^[A-Za-z_][\w]*(?::[\w.-]+)?(?:\.[\w]+)*/))) {
      const w = m[0];
      if (KW.has(w.toUpperCase())) push(w, 'kw');
      else if (/^[A-Za-z]+:/.test(w) || /^[a-z]\d+$/.test(w)) push(w, 'tag');
      else if (PSEUDO.has(w.toLowerCase()) || w.startsWith('fields.') || w.startsWith('entities.') || w.startsWith('purpose.')) push(w, 'key');
      else push(w);
      i += w.length;
    } else if ((m = rest.match(/^(<>|!=|>=|<=|[=<>~*])/))) {
      push(m[0], 'op');
      i += m[0].length;
    } else if ('(),;'.includes(ch)) {
      push(ch, 'punct');
      i++;
    } else {
      push(ch);
      i++;
    }
  }
  return out;
}

function compact(src: string): Token[] {
  const out: Token[] = [];
  const re = /("[^"]*"?|'[^']*'?)|([A-Za-z_.][\w.]*)(:|<=|>=|!=|=|<|>|~)|(\s+)|(\S)/g;
  let m: RegExpExecArray | null;
  let afterOp = false;
  while ((m = re.exec(src))) {
    if (m[1]) out.push({ t: m[1], c: 'str' });
    else if (m[2]) {
      out.push({ t: m[2], c: 'key' }, { t: m[3], c: 'op' });
      afterOp = true;
      continue;
    } else if (m[4]) out.push({ t: m[4] });
    else if (m[5]) {
      // value or bare word: consume the run
      let j = m.index;
      while (j < src.length && !/\s/.test(src[j])) j++;
      const w = src.slice(m.index, j);
      re.lastIndex = j;
      out.push({ t: w, c: afterOp ? (/^-?[\d.]+%?$/.test(w) ? 'num' : 'val') : undefined });
    }
    afterOp = false;
  }
  return out;
}

function json(src: string): Token[] {
  const out: Token[] = [];
  const re = /("(?:[^"\\]|\\.)*")(\s*:)?|(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)|\b(true|false|null)\b|([{}[\],])|(\s+)|(.)/g;
  let m: RegExpExecArray | null;
  while ((m = re.exec(src))) {
    if (m[1]) {
      out.push({ t: m[1], c: m[2] ? 'key' : 'str' });
      if (m[2]) out.push({ t: m[2], c: 'punct' });
    } else if (m[3]) out.push({ t: m[3], c: 'num' });
    else if (m[4]) out.push({ t: m[4], c: 'kw' });
    else if (m[5]) out.push({ t: m[5], c: 'punct' });
    else out.push({ t: m[0] });
  }
  return out;
}

function yaml(src: string): Token[] {
  const out: Token[] = [];
  const lines = src.split('\n');
  lines.forEach((line, li) => {
    const m = line.match(/^(\s*-?\s*)([^:#"'{[\s][^:#]*?)(:)(\s|$)(.*)$/);
    const rest = (s: string) => {
      const ci = s.search(/\s#/);
      const val = ci >= 0 ? s.slice(0, ci) : s.startsWith('#') ? '' : s;
      const com = ci >= 0 ? s.slice(ci) : s.startsWith('#') ? s : '';
      if (val) out.push({ t: val, c: /^\s*-?[\d.]+\s*$/.test(val) ? 'num' : /^\s*(true|false|null)\s*$/.test(val) ? 'kw' : /^\s*[{[]/.test(val) ? undefined : 'str' });
      if (com) out.push({ t: com, c: 'com' });
    };
    if (/^\s*#/.test(line)) out.push({ t: line, c: 'com' });
    else if (m) {
      out.push({ t: m[1], c: 'punct' }, { t: m[2], c: 'key' }, { t: m[3], c: 'punct' }, { t: m[4] });
      rest(m[5]);
    } else {
      const d = line.match(/^(\s*-\s*)(.*)$/);
      if (d) {
        out.push({ t: d[1], c: 'punct' });
        rest(d[2]);
      } else rest(line);
    }
    if (li < lines.length - 1) out.push({ t: '\n' });
  });
  return out;
}

function xml(src: string): Token[] {
  const out: Token[] = [];
  const re = /(<!--[\s\S]*?-->)|(<\/?)([\w:.-]+)|([\w:.-]+)(=)("[^"]*"|'[^']*')|(\/?>)|([^<]+)|(.)/g;
  let m: RegExpExecArray | null;
  while ((m = re.exec(src))) {
    if (m[1]) out.push({ t: m[1], c: 'com' });
    else if (m[2]) out.push({ t: m[2], c: 'punct' }, { t: m[3], c: 'tag' });
    else if (m[4]) out.push({ t: m[4], c: 'attr' }, { t: m[5], c: 'punct' }, { t: m[6], c: 'val' });
    else if (m[7]) out.push({ t: m[7], c: 'punct' });
    else out.push({ t: m[0] });
  }
  return out;
}

function md(src: string): Token[] {
  const out: Token[] = [];
  let inCode = false;
  src.split('\n').forEach((line, i, arr) => {
    if (/^```/.test(line)) {
      inCode = !inCode;
      out.push({ t: line, c: 'com' });
    } else if (inCode) out.push({ t: line, c: 'val' });
    else if (/^#{1,6} /.test(line)) out.push({ t: line, c: 'h' });
    else if (/^>/.test(line)) out.push({ t: line, c: 'com' });
    else if (/^\|/.test(line)) out.push({ t: line, c: 'punct' });
    else {
      const re = /(\*\*[^*]+\*\*)|(\{\{[^}]*\}\})|(`[^`]+`)|(~~[^~]+~~)|([^*{`~]+|.)/g;
      let m: RegExpExecArray | null;
      while ((m = re.exec(line))) {
        if (m[1]) out.push({ t: m[1], c: 'tag' });
        else if (m[2]) out.push({ t: m[2], c: 'attr' });
        else if (m[3]) out.push({ t: m[3], c: 'val' });
        else if (m[4]) out.push({ t: m[4], c: 'com' });
        else out.push({ t: m[0] });
      }
    }
    if (i < arr.length - 1) out.push({ t: '\n' });
  });
  return out;
}

export function tokenize(src: string, lang: Lang): Token[] {
  switch (lang) {
    case 'bdql':
      return bdql(src);
    case 'compact':
      return src.trim().startsWith('{') ? json(src) : compact(src);
    case 'json':
      return json(src);
    case 'yaml':
      return yaml(src);
    case 'xml':
      return xml(src);
    case 'md':
      return md(src);
    default:
      return [{ t: src }];
  }
}

export function guessLang(format: string | undefined, text: string): Lang {
  const f = (format ?? '').toLowerCase();
  if (f === 'json' || f === 'yaml' || f === 'xml' || f === 'md' || f === 'csv') return f as Lang;
  if (f === 'markdown') return 'md';
  const t = text.trimStart();
  if (t.startsWith('<')) return 'xml';
  if (t.startsWith('{') || t.startsWith('[')) return 'json';
  if (t.startsWith('#')) return 'md';
  return 'text';
}

/** Split a BDQL script into statements with their source offsets (respects strings, $$…$$ and comments). */
export function splitStatements(src: string): { text: string; start: number; end: number }[] {
  const out: { text: string; start: number; end: number }[] = [];
  let i = 0,
    start = 0;
  while (i < src.length) {
    const ch = src[i];
    if (src.startsWith('--', i)) {
      const e = src.indexOf('\n', i);
      i = e < 0 ? src.length : e;
    } else if (src.startsWith('$$', i)) {
      const e = src.indexOf('$$', i + 2);
      i = e < 0 ? src.length : e + 2;
    } else if (ch === "'" || ch === '"') {
      let j = i + 1;
      while (j < src.length && src[j] !== ch) j += src[j] === '\\' ? 2 : 1;
      i = j + 1;
    } else if (ch === ';') {
      out.push({ text: src.slice(start, i + 1), start, end: i + 1 });
      start = i + 1;
      i++;
    } else i++;
  }
  if (src.slice(start).replace(/--.*$/gm, '').trim()) out.push({ text: src.slice(start), start, end: src.length });
  return out.filter((s) => s.text.replace(/--.*$/gm, '').trim().replace(/;$/, '').trim());
}

/** The statement under the cursor (or the last one before it). */
export function statementAt(src: string, pos: number): string {
  const st = splitStatements(src);
  const hit = st.find((s) => pos >= s.start && pos <= s.end) ?? [...st].reverse().find((s) => s.start <= pos) ?? st[0];
  return hit ? hit.text.trim() : '';
}
