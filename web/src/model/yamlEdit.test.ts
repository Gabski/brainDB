// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from 'vitest';
import { addListItem, diffLines, ensureBlockMap, getYamlBool, getYamlNumber, getYamlString, getYamlValue, listItems, removeYamlKey, setYamlValue, YamlEditUnsupported } from './yamlEdit';

// The shapes that actually occur in the shipped braindb.yaml files.
const Y = `# brainDB configuration for the braindb-self example.
name: braindb-self
goal: Ship 0.0.3 with the paraphrase question answered
context: {depth: 2, threshold: 0.3, hop: 0.85, showSuperseded: false, focus: [q180, d151]}
embedding:
  provider: hash           # offline and free
  alpha: 0.5
  use: enriched
budget: {warnTokens: 3000}
kinds:
  - {name: addendum, label: spec addendum, halfLife: 365, floor: 0.85, prefix: a}
  - {name: command, label: CLI command, halfLife: 180, floor: 0.7, prefix: c}
sections: [facts, decisions, spec]
import:
  keepRaw: false
  rules:
    - match: "eval-runs.csv"
      importer: csv
      kind: benchmark
purpose:
  name: brainDB project brain
  admission:
    mode: advise
    accept: 0.35
`;

describe('getYamlValue', () => {
  it('reads top-level, block and flow-map keys', () => {
    expect(getYamlString(Y, 'name')).toBe('braindb-self');
    expect(getYamlNumber(Y, 'context.depth')).toBe(2);
    expect(getYamlNumber(Y, 'context.threshold')).toBe(0.3);
    expect(getYamlBool(Y, 'context.showSuperseded')).toBe(false);
    expect(getYamlString(Y, 'embedding.provider')).toBe('hash');
    expect(getYamlNumber(Y, 'budget.warnTokens')).toBe(3000);
    expect(getYamlString(Y, 'purpose.admission.mode')).toBe('advise');
    expect(getYamlNumber(Y, 'purpose.admission.accept')).toBe(0.35);
  });
  it('reads a field of a selected list item', () => {
    expect(getYamlNumber(Y, 'kinds[name=command].halfLife')).toBe(180);
    expect(getYamlString(Y, 'kinds[name=addendum].prefix')).toBe('a');
    expect(getYamlString(Y, 'import.rules[match=eval-runs.csv].kind')).toBe('benchmark');
  });
  it('returns undefined for keys that are not set', () => {
    expect(getYamlValue(Y, 'embedding.model')).toBeUndefined();
    expect(getYamlValue(Y, 'kinds[name=nope].halfLife')).toBeUndefined();
    expect(getYamlValue(Y, 'access.enabled')).toBeUndefined();
  });
});

describe('setYamlValue', () => {
  it('rewrites a flow-map value in place and leaves the rest of the line alone', () => {
    const out = setYamlValue(Y, 'context.threshold', 0.25);
    expect(out).toContain('context: {depth: 2, threshold: 0.25, hop: 0.85, showSuperseded: false, focus: [q180, d151]}');
    expect(getYamlNumber(out, 'context.threshold')).toBe(0.25);
  });
  it('adds a key to a flow map that does not have it', () => {
    const out = setYamlValue(Y, 'budget.hardTokens', 8000);
    expect(out).toContain('budget: {warnTokens: 3000, hardTokens: 8000}');
  });
  it('keeps the trailing comment of a block key', () => {
    const out = setYamlValue(Y, 'embedding.provider', 'openai');
    expect(out).toContain('provider: openai           # offline and free'.replace(/ {11}/, ' '));
    expect(getYamlString(out, 'embedding.provider')).toBe('openai');
  });
  it('appends a missing key to an existing block at its own indent', () => {
    const out = setYamlValue(Y, 'embedding.model', 'openai/text-embedding-3-small');
    expect(out).toMatch(/\n {2}model: openai\/text-embedding-3-small\n/);
    expect(getYamlString(out, 'embedding.model')).toBe('openai/text-embedding-3-small');
  });
  it('creates the missing block map above a new key', () => {
    const out = setYamlValue(Y, 'access.enabled', true);
    expect(getYamlBool(out, 'access.enabled')).toBe(true);
    expect(out).toContain('access:');
  });
  it('edits one item of a list without touching its neighbour', () => {
    const out = setYamlValue(Y, 'kinds[name=command].floor', 0.55);
    expect(out).toContain('{name: command, label: CLI command, halfLife: 180, floor: 0.55, prefix: c}');
    expect(getYamlNumber(out, 'kinds[name=addendum].floor')).toBe(0.85);
  });
  it('edits a block-style list item', () => {
    const out = setYamlValue(Y, 'import.rules[match=eval-runs.csv].kind', 'record');
    expect(getYamlString(out, 'import.rules[match=eval-runs.csv].kind')).toBe('record');
    expect(out).toContain('importer: csv');
  });
  it('quotes a string only when it needs quoting', () => {
    expect(setYamlValue(Y, 'name', 'self-brain')).toContain('name: self-brain');
    expect(setYamlValue(Y, 'name', 'a: b')).toContain('name: "a: b"');
    expect(setYamlValue(Y, 'name', 'no')).toContain('name: "no"');
  });
  it('preserves every other line', () => {
    const out = setYamlValue(Y, 'context.depth', 3);
    const a = Y.split('\n'),
      b = out.split('\n');
    expect(b.length).toBe(a.length);
    expect(b.filter((l, i) => l !== a[i])).toHaveLength(1);
  });
  it('refuses rather than corrupting what it cannot parse', () => {
    const multi = 'context:\n  {depth: 2,\n   hop: 0.85}\n';
    expect(() => setYamlValue(multi, 'context.depth', 3)).toThrow(YamlEditUnsupported);
  });
});

describe('removeYamlKey', () => {
  it('removes a flow-map key', () => {
    const out = removeYamlKey(Y, 'context.showSuperseded');
    expect(out).toContain('context: {depth: 2, threshold: 0.3, hop: 0.85, focus: [q180, d151]}');
  });
  it('removes a block key', () => {
    expect(getYamlValue(removeYamlKey(Y, 'embedding.alpha'), 'embedding.alpha')).toBeUndefined();
  });
  it('takes an emptied parent block with it, because an empty map parses as null', () => {
    let out = removeYamlKey(Y, 'purpose.admission.mode');
    out = removeYamlKey(out, 'purpose.admission.accept');
    expect(out).not.toContain('admission:');
    expect(out).toContain('name: brainDB project brain');
  });
  it('is a no-op for a key that is not there', () => {
    expect(removeYamlKey(Y, 'embedding.model')).toBe(Y);
  });
});

describe('listItems', () => {
  it('lists flow-style and block-style items', () => {
    expect(listItems(Y, 'kinds', 'name')).toEqual(['addendum', 'command']);
    expect(listItems(Y, 'import.rules', 'match')).toEqual(['eval-runs.csv']);
    expect(listItems(Y, 'nope', 'name')).toEqual([]);
  });
});

describe('diffLines', () => {
  it('reports only the changed line', () => {
    const d = diffLines(Y, setYamlValue(Y, 'context.depth', 3));
    expect(d).toHaveLength(1);
    expect(d[0].before).toContain('depth: 2');
    expect(d[0].after).toContain('depth: 3');
  });
  it('reports an inserted line as an addition', () => {
    const d = diffLines(Y, setYamlValue(Y, 'embedding.model', 'm'));
    expect(d).toHaveLength(1);
    expect(d[0].before).toBeUndefined();
    expect(d[0].after).toContain('model: m');
  });
});

describe('addListItem', () => {
  it('appends an item at the indent the list already uses', () => {
    const out = addListItem(Y, 'kinds', { name: 'fact', halfLife: 180, floor: 0.7 });
    expect(out).toContain('  - {name: fact, halfLife: 180, floor: 0.7}');
    expect(listItems(out, 'kinds', 'name')).toEqual(['addendum', 'command', 'fact']);
    expect(getYamlNumber(out, 'kinds[name=fact].floor')).toBe(0.7);
  });
  it('creates the list when it is missing', () => {
    const out = addListItem('name: x\n', 'kinds', { name: 'fact', halfLife: 180 });
    expect(out).toContain('kinds:\n  - {name: fact, halfLife: 180}');
  });
  it('appends inside a nested block without disturbing what follows', () => {
    const out = addListItem(Y, 'import.rules', { match: '*.md', importer: 'markdown' });
    expect(listItems(out, 'import.rules', 'match')).toEqual(['eval-runs.csv', '*.md']);
    expect(getYamlString(out, 'purpose.admission.mode')).toBe('advise');
  });
});

describe('ensureBlockMap', () => {
  it('leaves an existing block map alone', () => {
    const y = 'name: x\nserver:\n  tokenEnv: BRAINDB_TOKEN\n';
    expect(ensureBlockMap(y, 'server')).toBe(y);
  });
  it('appends the key when it is missing, so a list can be created under it', () => {
    const out = ensureBlockMap('name: x\n', 'server');
    expect(out).toBe('name: x\nserver:\n');
  });
  it('adds the missing newline before appending', () => {
    expect(ensureBlockMap('name: x', 'server')).toBe('name: x\nserver:\n');
  });
  it('refuses a flow map instead of rewriting a hand-written line', () => {
    expect(() => ensureBlockMap('server: {addr: "127.0.0.1:7791"}\n', 'server')).toThrow(YamlEditUnsupported);
  });
  it('refuses a scalar', () => {
    expect(() => ensureBlockMap('server: off\n', 'server')).toThrow(YamlEditUnsupported);
  });
  it('ignores a key of the same name nested deeper', () => {
    const out = ensureBlockMap('sources:\n  - {name: lab, server: x}\n', 'server');
    expect(out).toBe('sources:\n  - {name: lab, server: x}\nserver:\n');
  });
});

// The Settings form adds API and MCP tokens as entries naming an environment variable. The value never
// enters the file, so what the form writes must be exactly a name, a variable and a scope.
describe('server tokens, as the settings form writes them', () => {
  it('creates the server block and the list on a file that has neither', () => {
    const out = addListItem(ensureBlockMap('name: scratch\n', 'server'), 'server.tokens', { name: 'bot', tokenEnv: 'BRAINDB_TOKEN_BOT', scope: 'read' });
    expect(out).toContain('server:\n  tokens:\n    - {name: bot, tokenEnv: BRAINDB_TOKEN_BOT, scope: read}');
    expect(listItems(out, 'server.tokens', 'name')).toEqual(['bot']);
    expect(getYamlString(out, 'server.tokens[name=bot].tokenEnv')).toBe('BRAINDB_TOKEN_BOT');
    expect(getYamlString(out, 'server.tokens[name=bot].scope')).toBe('read');
  });
  it('appends to an existing list and keeps the admin variable', () => {
    const y = 'name: x\nserver:\n  tokenEnv: BRAINDB_TOKEN\n  tokens:\n    - {name: bot, tokenEnv: BRAINDB_TOKEN_BOT, scope: read}\n';
    const out = addListItem(y, 'server.tokens', { name: 'ci', tokenEnv: 'BRAINDB_TOKEN_CI', scope: 'write' });
    expect(listItems(out, 'server.tokens', 'name')).toEqual(['bot', 'ci']);
    expect(getYamlString(out, 'server.tokenEnv')).toBe('BRAINDB_TOKEN');
  });
  it('edits one entry without touching its neighbour', () => {
    const y = 'server:\n  tokens:\n    - {name: bot, tokenEnv: A, scope: read}\n    - {name: ci, tokenEnv: B, scope: read}\n';
    const out = setYamlValue(y, 'server.tokens[name=ci].scope', 'admin');
    expect(getYamlString(out, 'server.tokens[name=ci].scope')).toBe('admin');
    expect(getYamlString(out, 'server.tokens[name=bot].scope')).toBe('read');
  });
  it('removes one entry and leaves the rest of the block', () => {
    const y = 'server:\n  tokenEnv: BRAINDB_TOKEN\n  tokens:\n    - {name: bot, tokenEnv: A, scope: read}\n    - {name: ci, tokenEnv: B, scope: read}\n';
    const out = removeYamlKey(y, 'server.tokens[name=bot]');
    expect(listItems(out, 'server.tokens', 'name')).toEqual(['ci']);
    expect(getYamlString(out, 'server.tokenEnv')).toBe('BRAINDB_TOKEN');
  });
});

// A selector as the last path segment removes one item. Removing the list instead would delete every other
// entry with it, which is what the Settings tables call on every trash button.
describe('removeYamlKey with a list selector', () => {
  it('removes one flow item and keeps its neighbours', () => {
    const out = removeYamlKey(Y, 'kinds[name=addendum]');
    expect(listItems(out, 'kinds', 'name')).toEqual(['command']);
    expect(getYamlString(out, 'kinds[name=command].label')).toBe('CLI command');
  });
  it('removes the last item and the rest of the file survives', () => {
    const out = removeYamlKey(Y, 'kinds[name=command]');
    expect(listItems(out, 'kinds', 'name')).toEqual(['addendum']);
    expect(getYamlString(out, 'purpose.admission.mode')).toBe('advise');
  });
  it('removes a block item with its indented lines', () => {
    const out = removeYamlKey(Y, 'import.rules[match=eval-runs.csv]');
    expect(listItems(out, 'import.rules', 'match')).toEqual([]);
    expect(getYamlBool(out, 'import.keepRaw')).toBe(false);
  });
  it('takes the list key with it when the last item goes, so the key does not parse as null', () => {
    const y = 'name: x\nserver:\n  tokenEnv: T\n  tokens:\n    - {name: bot, tokenEnv: A, scope: read}\n';
    const out = removeYamlKey(y, 'server.tokens[name=bot]');
    expect(out).not.toContain('tokens:');
    expect(getYamlString(out, 'server.tokenEnv')).toBe('T');
  });
  it('leaves the file alone when no item matches', () => {
    expect(removeYamlKey(Y, 'kinds[name=nope]')).toBe(Y);
  });
});
