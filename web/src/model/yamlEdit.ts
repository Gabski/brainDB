// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

/**
 * Minimal, comment-preserving editor for the subset of YAML that `braindb.yaml` uses.
 *
 * It is deliberately not a YAML parser. The settings forms change single scalar keys, and a round trip through a
 * real parser would reformat the file and drop every comment in it — and `braindb.yaml` is mostly comments
 * explaining what the keys do. So this works on the text: it locates a key by path and rewrites that one value.
 *
 * Paths: `name`, `context.depth`, `purpose.admission.mode`, `kinds[name=deal].halfLife`.
 * Supported containers: block maps, single-line flow maps (`context: {depth: 2, hop: 0.85}`) and lists of either.
 * Anything else (a multi-line flow map, a key inside an anchor) throws `YamlEditUnsupported`, and the caller falls
 * back to the raw editor instead of corrupting the file.
 */

export class YamlEditUnsupported extends Error {
  constructor(public path: string, message: string) {
    super(message);
    this.name = 'YamlEditUnsupported';
  }
}

export type Scalar = string | number | boolean;

interface Seg {
  key: string;
  sel?: { field: string; value: string };
}

/** A resolved container: either a range of block lines, or one line holding a flow map. */
type Container = { kind: 'block'; start: number; end: number; indent: number } | { kind: 'flow'; line: number; open: number; close: number };

const KEY_RE = /^(\s*)([A-Za-z0-9_$-]+|"[^"]+"|'[^']+'):(\s|$)/;
const ITEM_RE = /^(\s*)-(\s|$)/;

function parsePath(path: string): Seg[] {
  // split on '.' outside brackets: a selector value can be a file name, as in rules[match=eval-runs.csv]
  const raws: string[] = [];
  let depth = 0,
    start = 0;
  for (let i = 0; i <= path.length; i++) {
    const c = path[i];
    if (c === '[') depth++;
    else if (c === ']') depth--;
    else if ((i === path.length || c === '.') && depth === 0) {
      raws.push(path.slice(start, i));
      start = i + 1;
    }
  }
  return raws.map((raw) => {
    const m = /^([A-Za-z0-9_$-]+)\[([A-Za-z0-9_$-]+)=([^\]]+)\]$/.exec(raw);
    return m ? { key: m[1], sel: { field: m[2], value: m[3] } } : { key: raw };
  });
}

function indentOf(line: string): number {
  return /^\s*/.exec(line)![0].length;
}

function isBlank(line: string): boolean {
  return line.trim() === '' || line.trim().startsWith('#');
}

/** Split a key line into its parts, keeping the trailing comment so a rewrite does not eat it. */
function splitLine(line: string): { indent: string; key: string; value: string; comment: string } | null {
  const m = KEY_RE.exec(line);
  if (!m) return null;
  const head = line.slice(0, m[1].length + m[2].length + 1);
  let rest = line.slice(head.length);
  let comment = '';
  // a '#' inside quotes or braces is not a comment; only take one that follows whitespace at depth 0
  let depth = 0,
    quote = '';
  for (let i = 0; i < rest.length; i++) {
    const c = rest[i];
    if (quote) {
      if (c === quote) quote = '';
      continue;
    }
    if (c === '"' || c === "'") quote = c;
    else if (c === '{' || c === '[') depth++;
    else if (c === '}' || c === ']') depth--;
    else if (c === '#' && depth === 0 && (i === 0 || /\s/.test(rest[i - 1]))) {
      comment = rest.slice(i);
      rest = rest.slice(0, i);
      break;
    }
  }
  return { indent: m[1], key: m[2].replace(/^["']|["']$/g, ''), value: rest.trim(), comment };
}

/** The lines belonging to the value of a key line: everything indented deeper, plus list items at the same indent. */
function blockRange(lines: string[], keyLine: number, keyIndent: number, limit: number): { start: number; end: number; indent: number } {
  let i = keyLine + 1,
    indent = -1;
  for (; i < limit; i++) {
    if (isBlank(lines[i])) continue;
    const ind = indentOf(lines[i]);
    if (ind > keyIndent || (ind === keyIndent && ITEM_RE.test(lines[i]))) {
      if (indent < 0) indent = ind;
      continue;
    }
    break;
  }
  return { start: keyLine + 1, end: i, indent: indent < 0 ? keyIndent + 2 : indent };
}

function findKeyLine(lines: string[], c: Extract<Container, { kind: 'block' }>, key: string): number {
  for (let i = c.start; i < c.end; i++) {
    if (isBlank(lines[i])) continue;
    if (indentOf(lines[i]) !== c.indent) continue;
    const p = splitLine(lines[i]);
    if (p && p.key === key) return i;
  }
  return -1;
}

/** Find the flow-map span `{...}` on one line, starting at `from`. */
function flowSpan(line: string, from: number): { open: number; close: number } | null {
  const open = line.indexOf('{', from);
  if (open < 0) return null;
  let depth = 0,
    quote = '';
  for (let i = open; i < line.length; i++) {
    const c = line[i];
    if (quote) {
      if (c === quote) quote = '';
      continue;
    }
    if (c === '"' || c === "'") quote = c;
    else if (c === '{') depth++;
    else if (c === '}' && --depth === 0) return { open, close: i };
  }
  return null;
}

/** Split a flow-map body into its top-level `key: value` parts. */
function flowParts(body: string): { text: string; from: number }[] {
  let depth = 0,
    quote = '',
    start = 0;
  const parts: { text: string; from: number }[] = [];
  for (let i = 0; i <= body.length; i++) {
    const c = body[i];
    if (i === body.length || (c === ',' && depth === 0 && !quote)) {
      parts.push({ text: body.slice(start, i), from: start });
      start = i + 1;
      continue;
    }
    if (quote) {
      if (c === quote) quote = '';
      continue;
    }
    if (c === '"' || c === "'") quote = c;
    else if (c === '{' || c === '[') depth++;
    else if (c === '}' || c === ']') depth--;
  }
  return parts;
}

/** Read one key out of a flow map body (without the braces). */
function flowGet(body: string, key: string): { value: string; from: number; to: number } | null {
  for (const p of flowParts(body)) {
    const m = /^(\s*)([A-Za-z0-9_$-]+|"[^"]+")\s*:\s*/.exec(p.text);
    if (!m) continue;
    if (m[2].replace(/^"|"$/g, '') !== key) continue;
    return { value: p.text.slice(m[0].length).trim(), from: p.from, to: p.from + p.text.length };
  }
  return null;
}

/** Walk a path down to the container that holds its last segment. Returns null when a step is missing. */
function resolveParent(lines: string[], segs: Seg[], path: string): Container | null {
  let cur: Container = { kind: 'block', start: 0, end: lines.length, indent: 0 };
  for (const seg of segs.slice(0, -1)) {
    if (cur.kind === 'flow') throw new YamlEditUnsupported(path, `"${seg.key}" sits inside a flow map; edit it in Raw YAML`);
    const line = findKeyLine(lines, cur, seg.key);
    if (line < 0) return null;
    const p = splitLine(lines[line])!;
    if (seg.sel) {
      const r = blockRange(lines, line, p.indent.length, cur.end);
      const item = findListItem(lines, r, seg.sel);
      if (!item) return null;
      cur = item;
      continue;
    }
    if (p.value.startsWith('{')) {
      const span = flowSpan(lines[line], p.indent.length);
      if (!span) throw new YamlEditUnsupported(path, `the flow map at "${seg.key}" spans several lines; edit it in Raw YAML`);
      cur = { kind: 'flow', line, open: span.open, close: span.close };
      continue;
    }
    if (p.value !== '') throw new YamlEditUnsupported(path, `"${seg.key}" is a scalar, not a map`);
    const r = blockRange(lines, line, p.indent.length, cur.end);
    for (let i = r.start; i < r.end; i++) {
      if (isBlank(lines[i])) continue;
      if (/^[{[]/.test(lines[i].trim())) throw new YamlEditUnsupported(path, `the collection at "${seg.key}" spans several lines; edit it in Raw YAML`);
      break;
    }
    cur = { kind: 'block', ...r };
  }
  return cur;
}

/** Inside a list, find the item whose `field` equals `value`; works for `- {a: b}` and for block items. */
function findListItem(lines: string[], range: { start: number; end: number; indent: number }, sel: { field: string; value: string }): Container | null {
  for (let i = range.start; i < range.end; i++) {
    if (isBlank(lines[i]) || !ITEM_RE.test(lines[i]) || indentOf(lines[i]) !== range.indent) continue;
    const after = lines[i].indexOf('-') + 1;
    const span = flowSpan(lines[i], after);
    if (span) {
      const got = flowGet(lines[i].slice(span.open + 1, span.close), sel.field);
      if (got && unquote(got.value) === sel.value) return { kind: 'flow', line: i, open: span.open, close: span.close };
      continue;
    }
    // block item: "- key: value" plus the lines indented under it
    const itemIndent = lines[i].indexOf('-') + 2;
    let end = i + 1;
    for (; end < range.end; end++) {
      if (isBlank(lines[end])) continue;
      if (indentOf(lines[end]) < itemIndent || ITEM_RE.test(lines[end])) break;
    }
    const head = splitLine(' '.repeat(itemIndent) + lines[i].slice(itemIndent));
    const c: Extract<Container, { kind: 'block' }> = { kind: 'block', start: i, end, indent: itemIndent };
    const at = head && head.key === sel.field ? i : findKeyLine(lines, c, sel.field);
    if (at >= 0) {
      const p = splitLine(lines[at].slice(0, itemIndent).replace(/-/g, ' ') + lines[at].slice(itemIndent));
      if (p && unquote(p.value) === sel.value) return c;
    }
  }
  return null;
}

/** Strip one layer of quotes. */
export function unquote(v: string): string {
  const t = v.trim();
  return /^".*"$/.test(t) || /^'.*'$/.test(t) ? t.slice(1, -1) : t;
}

/** Render a scalar the way braindb.yaml writes it: bare unless the text needs quoting. */
export function formatScalar(v: Scalar): string {
  if (typeof v === 'number' || typeof v === 'boolean') return String(v);
  const t = v.trim();
  if (t === '') return '""';
  if (/^[A-Za-z0-9_@/.$+-]+$/.test(t) && !/^(y|n|yes|no|on|off|true|false|null|~)$/i.test(t)) return t;
  return JSON.stringify(t);
}

/** Read the raw text of a key, as written. Returns undefined when the key is not in the file. */
export function getYamlValue(text: string, path: string): string | undefined {
  const lines = text.split('\n');
  const segs = parsePath(path);
  let parent: Container | null;
  try {
    parent = resolveParent(lines, segs, path);
  } catch {
    return undefined;
  }
  if (!parent) return undefined;
  const last = segs[segs.length - 1];
  if (parent.kind === 'flow') {
    const got = flowGet(lines[parent.line].slice(parent.open + 1, parent.close), last.key);
    return got ? got.value : undefined;
  }
  const line = findKeyLine(lines, parent, last.key);
  if (line < 0) return undefined;
  const p = splitLine(lines[line])!;
  return p.value === '' ? undefined : p.value;
}

/** Read a key as a string with quotes stripped. */
export function getYamlString(text: string, path: string): string | undefined {
  const v = getYamlValue(text, path);
  return v === undefined ? undefined : unquote(v);
}

/** Read a key as a number, or undefined when absent or not numeric. */
export function getYamlNumber(text: string, path: string): number | undefined {
  const v = getYamlValue(text, path);
  if (v === undefined) return undefined;
  const n = Number(unquote(v));
  return Number.isFinite(n) ? n : undefined;
}

/** Read a key as a boolean, or undefined when absent. */
export function getYamlBool(text: string, path: string): boolean | undefined {
  const v = getYamlValue(text, path);
  if (v === undefined) return undefined;
  const t = unquote(v).toLowerCase();
  return t === 'true' ? true : t === 'false' ? false : undefined;
}

/** Set one scalar, creating the key (and any missing block map above it) when needed. */
export function setYamlValue(text: string, path: string, value: Scalar): string {
  const lines = text.split('\n');
  const segs = parsePath(path);
  const last = segs[segs.length - 1];
  if (last.sel) throw new YamlEditUnsupported(path, 'a list selector must not be the last path segment');
  let parent = resolveParent(lines, segs, path);
  if (!parent) {
    if (segs.length === 1 || segs.some((s) => s.sel)) throw new YamlEditUnsupported(path, 'the parent of this key is missing');
    return setYamlValue(createBlock(lines, segs.slice(0, -1), path).join('\n'), path, value);
  }
  const out = formatScalar(value);
  if (parent.kind === 'flow') {
    const line = lines[parent.line];
    const body = line.slice(parent.open + 1, parent.close);
    const got = flowGet(body, last.key);
    const next = got
      ? body.slice(0, got.to - got.value.length) + out + body.slice(got.to)
      : body.trim() === ''
        ? `${last.key}: ${out}`
        : `${body.replace(/\s+$/, '')}, ${last.key}: ${out}`;
    lines[parent.line] = line.slice(0, parent.open + 1) + next + line.slice(parent.close);
    return lines.join('\n');
  }
  const at = findKeyLine(lines, parent, last.key);
  if (at >= 0) {
    const p = splitLine(lines[at])!;
    lines[at] = `${p.indent}${last.key}: ${out}${p.comment ? ' ' + p.comment.trim() : ''}`;
    return lines.join('\n');
  }
  let insert = parent.end;
  while (insert > parent.start && isBlank(lines[insert - 1])) insert--;
  lines.splice(insert, 0, `${' '.repeat(parent.indent)}${last.key}: ${out}`);
  return lines.join('\n');
}

/** Create the missing block maps for a path prefix, appending them at the end of the nearest existing parent. */
function createBlock(lines: string[], segs: Seg[], path: string): string[] {
  let depth = 0;
  let cur: Container = { kind: 'block', start: 0, end: lines.length, indent: 0 };
  for (; depth < segs.length; depth++) {
    if (cur.kind !== 'block') throw new YamlEditUnsupported(path, 'cannot create a key inside a flow map');
    const line = findKeyLine(lines, cur, segs[depth].key);
    if (line < 0) break;
    const p = splitLine(lines[line])!;
    if (p.value !== '') throw new YamlEditUnsupported(path, `"${segs[depth].key}" is a scalar, not a map`);
    cur = { kind: 'block', ...blockRange(lines, line, p.indent.length, cur.end) };
  }
  const block = cur as Extract<Container, { kind: 'block' }>;
  let insert = block.end;
  while (insert > block.start && isBlank(lines[insert - 1])) insert--;
  const add: string[] = [];
  for (let i = depth; i < segs.length; i++) add.push(`${' '.repeat(block.indent + (i - depth) * 2)}${segs[i].key}:`);
  lines.splice(insert, 0, ...add);
  return lines;
}

/** Remove a key. An empty block map left behind is removed with it, so the file stays valid. */
export function removeYamlKey(text: string, path: string): string {
  const lines = text.split('\n');
  const segs = parsePath(path);
  const last = segs[segs.length - 1];
  const parent = resolveParent(lines, segs, path);
  if (!parent) return text;
  if (parent.kind === 'flow') {
    const line = lines[parent.line];
    const body = line.slice(parent.open + 1, parent.close);
    const kept = flowParts(body)
      .filter((p) => {
        const m = /^(\s*)([A-Za-z0-9_$-]+|"[^"]+")\s*:/.exec(p.text);
        return !m || m[2].replace(/^"|"$/g, '') !== last.key;
      })
      .map((p) => p.text.trim())
      .filter((t) => t !== '');
    lines[parent.line] = line.slice(0, parent.open + 1) + kept.join(', ') + line.slice(parent.close);
    return lines.join('\n');
  }
  // A selector as the last segment removes one list item, not the list: `kinds[name=fact]` takes the
  // `- {name: fact, …}` entry and leaves its neighbours where they are.
  if (last.sel) {
    const line = findKeyLine(lines, parent, last.key);
    if (line < 0) return text;
    const lp = splitLine(lines[line])!;
    if (lp.value !== '') throw new YamlEditUnsupported(path, `"${last.key}" is a flow sequence; edit it in Raw YAML`);
    const r = blockRange(lines, line, lp.indent.length, parent.end);
    const item = findListItem(lines, r, last.sel);
    if (!item) return text;
    const from = item.kind === 'flow' ? item.line : item.start;
    const to = item.kind === 'flow' ? item.line + 1 : item.end;
    lines.splice(from, to - from);
    // an empty list key would parse as null, which the loader rejects: take the key with us
    const after = blockRange(lines, line, lp.indent.length, parent.end - (to - from));
    if (lines.slice(after.start, after.end).every(isBlank)) {
      return removeYamlKey(lines.join('\n'), [...segs.slice(0, -1).map(segText), last.key].join('.'));
    }
    return lines.join('\n');
  }
  const at = findKeyLine(lines, parent, last.key);
  if (at < 0) return text;
  const p = splitLine(lines[at])!;
  const span = p.value === '' ? blockRange(lines, at, p.indent.length, parent.end).end : at + 1;
  lines.splice(at, span - at);
  // a parent block that is now empty would parse as null, which the loader rejects: take it with us
  if (segs.length > 1) {
    const rest = lines.slice(parent.start, parent.end - (span - at)).filter((l) => !isBlank(l));
    if (rest.length === 0) return removeYamlKey(lines.join('\n'), segs.slice(0, -1).map(segText).join('.'));
  }
  return lines.join('\n');
}

function segText(s: Seg): string {
  return s.sel ? `${s.key}[${s.sel.field}=${s.sel.value}]` : s.key;
}

/** List the values of `field` for every item of a list, in file order: `listItems(y, 'kinds', 'name')`. */
export function listItems(text: string, listPath: string, field: string): string[] {
  const lines = text.split('\n');
  const segs = parsePath(listPath);
  const parent = resolveParent(lines, segs, listPath);
  if (!parent || parent.kind !== 'block') return [];
  const line = findKeyLine(lines, parent, segs[segs.length - 1].key);
  if (line < 0) return [];
  const p = splitLine(lines[line])!;
  const r = blockRange(lines, line, p.indent.length, parent.end);
  const out: string[] = [];
  for (let i = r.start; i < r.end; i++) {
    if (isBlank(lines[i]) || !ITEM_RE.test(lines[i]) || indentOf(lines[i]) !== r.indent) continue;
    const after = lines[i].indexOf('-') + 1;
    const span = flowSpan(lines[i], after);
    if (span) {
      const got = flowGet(lines[i].slice(span.open + 1, span.close), field);
      if (got) out.push(unquote(got.value));
      continue;
    }
    const itemIndent = lines[i].indexOf('-') + 2;
    let end = i + 1;
    for (; end < r.end; end++) {
      if (isBlank(lines[end])) continue;
      if (indentOf(lines[end]) < itemIndent || ITEM_RE.test(lines[end])) break;
    }
    const head = splitLine(' '.repeat(itemIndent) + lines[i].slice(itemIndent));
    if (head && head.key === field) {
      out.push(unquote(head.value));
      continue;
    }
    const at = findKeyLine(lines, { kind: 'block', start: i + 1, end, indent: itemIndent }, field);
    if (at >= 0) out.push(unquote(splitLine(lines[at])!.value));
  }
  return out;
}

/**
 * Append a flow-style item to a list, creating the list key when it is missing:
 * `addListItem(y, 'kinds', {name: 'fact', halfLife: 180})` adds `  - {name: fact, halfLife: 180}`.
 */
export function addListItem(text: string, listPath: string, fields: Record<string, Scalar>): string {
  const lines = text.split('\n');
  const segs = parsePath(listPath);
  const item = `- {${Object.entries(fields)
    .map(([k, v]) => `${k}: ${formatScalar(v)}`)
    .join(', ')}}`;
  const parent = resolveParent(lines, segs, listPath);
  const key = segs[segs.length - 1].key;
  if (!parent || parent.kind !== 'block') throw new YamlEditUnsupported(listPath, 'the parent of this list is missing or is a flow map');
  const line = findKeyLine(lines, parent, key);
  if (line < 0) {
    let insert = parent.end;
    while (insert > parent.start && isBlank(lines[insert - 1])) insert--;
    lines.splice(insert, 0, `${' '.repeat(parent.indent)}${key}:`, `${' '.repeat(parent.indent + 2)}${item}`);
    return lines.join('\n');
  }
  const p = splitLine(lines[line])!;
  if (p.value !== '') throw new YamlEditUnsupported(listPath, `"${key}" is already a flow sequence; edit it in Raw YAML`);
  const r = blockRange(lines, line, p.indent.length, parent.end);
  let insert = r.end;
  while (insert > r.start && isBlank(lines[insert - 1])) insert--;
  lines.splice(insert, 0, `${' '.repeat(r.indent)}${item}`);
  return lines.join('\n');
}

/**
 * Ensure a top-level key exists as a block map, so a list or a key can be created under it.
 *
 * `server:` is the case this exists for: a file that never mentioned the server block has no parent for
 * `server.tokens`, and appending the key is the one safe way to create it without reformatting anything else.
 * A key that is already a scalar or a flow map is left alone and reported, because guessing there would
 * rewrite lines the author wrote by hand.
 */
export function ensureBlockMap(text: string, key: string): string {
  const lines = text.split('\n');
  const at = findKeyLine(lines, { kind: 'block', start: 0, end: lines.length, indent: 0 }, key);
  if (at >= 0) {
    const p = splitLine(lines[at])!;
    if (p.value === '') return text;
    throw new YamlEditUnsupported(key, p.value.startsWith('{') ? `"${key}" is a flow map; edit it in Raw YAML` : `"${key}" is a scalar, not a map`);
  }
  const head = text === '' || text.endsWith('\n') ? text : text + '\n';
  return head + `${key}:\n`;
}

/** Line-level diff for the save preview: what the form changed, and nothing else. */
export function diffLines(before: string, after: string): { line: number; before?: string; after?: string }[] {
  const a = before.split('\n'),
    b = after.split('\n');
  const out: { line: number; before?: string; after?: string }[] = [];
  let i = 0,
    j = 0;
  while (i < a.length || j < b.length) {
    if (a[i] === b[j]) {
      i++;
      j++;
      continue;
    }
    if (a[i] !== undefined && b[j] !== undefined && a[i + 1] === b[j + 1]) {
      out.push({ line: j + 1, before: a[i], after: b[j] });
      i++;
      j++;
      continue;
    }
    if (b[j] !== undefined && a[i] === b[j + 1]) {
      out.push({ line: j + 1, after: b[j] });
      j++;
      continue;
    }
    if (a[i] !== undefined && a[i + 1] === b[j]) {
      out.push({ line: j + 1, before: a[i] });
      i++;
      continue;
    }
    out.push({ line: j + 1, before: a[i], after: b[j] });
    i++;
    j++;
  }
  return out;
}
