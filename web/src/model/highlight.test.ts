// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from 'vitest';
import { guessLang, splitStatements, statementAt, tokenize, type Lang } from './highlight';

/** Tokenizers must be loss-free: concatenating the tokens gives the source back. */
const roundtrip = (src: string, lang: Lang) => tokenize(src, lang).map((t) => t.t).join('');

const SAMPLES: Record<string, { lang: Lang; src: string }> = {
  bdql: { lang: 'bdql', src: "-- a comment\nSELECT id, kind FROM memory WHERE kind = fact AND text ~ 'login' LIMIT 10;\n" },
  compact: { lang: 'compact', src: 'kind:record sort:-users limit:20 select:id,kind' },
  json: { lang: 'json', src: '{"a": 1, "b": [true, null, "x"]}' },
  yaml: { lang: 'yaml', src: 'key: value\nlist:\n  - one\n  - two\n# comment\n' },
  xml: { lang: 'xml', src: '<brain name="test"><!-- c --><node id="f1">text</node></brain>' },
  md: { lang: 'md', src: '# Title\n\nSome **bold** text and `code`.\n\n- item\n' },
};

describe('tokenize', () => {
  for (const [name, { lang, src }] of Object.entries(SAMPLES)) {
    it(`${name}: loses no character`, () => {
      expect(roundtrip(src, lang)).toBe(src);
    });
    it(`${name}: classifies at least one token`, () => {
      expect(tokenize(src, lang).some((t) => t.c)).toBe(true);
    });
  }
  it('marks BDQL keywords, strings and comments', () => {
    const t = tokenize("SELECT id FROM memory; -- note\n", 'bdql');
    expect(t.find((x) => x.t === 'SELECT')?.c).toBe('kw');
    expect(t.find((x) => x.t.startsWith('--'))?.c).toBe('com');
    expect(tokenize("REMEMBER 'hello world';", 'bdql').find((x) => x.t === "'hello world'")?.c).toBe('str');
  });
  it('keeps $$…$$ blocks in one string token', () => {
    const t = tokenize('REMEMBER $$multi\nline$$;', 'bdql');
    expect(t.find((x) => x.c === 'str')?.t).toBe('$$multi\nline$$');
  });
  it('treats a JSON body in compact mode as JSON', () => {
    expect(tokenize('{"kind": "fact"}', 'compact').some((t) => t.c === 'key' || t.c === 'tag')).toBe(true);
  });
  it('passes plain text through untouched', () => {
    expect(tokenize('just text', 'text')).toEqual([{ t: 'just text' }]);
  });
  it('handles an empty source', () => {
    for (const lang of Object.values(SAMPLES).map((s) => s.lang)) expect(roundtrip('', lang)).toBe('');
  });
  it('does not hang on an unterminated string or comment', () => {
    expect(roundtrip("SELECT 'unterminated", 'bdql')).toBe("SELECT 'unterminated");
    expect(roundtrip('<!-- unterminated', 'xml')).toBe('<!-- unterminated');
  });
});

describe('guessLang', () => {
  it('trusts an explicit format', () => {
    expect(guessLang('json', 'whatever')).toBe('json');
    expect(guessLang('markdown', 'x')).toBe('md');
    expect(guessLang('CSV', 'x')).toBe('csv');
  });
  it('sniffs the body when the format is unknown', () => {
    expect(guessLang(undefined, '  <brain/>')).toBe('xml');
    expect(guessLang(undefined, '{"a":1}')).toBe('json');
    expect(guessLang(undefined, '[1,2]')).toBe('json');
    expect(guessLang(undefined, '# Title')).toBe('md');
    expect(guessLang(undefined, 'plain')).toBe('text');
  });
});

describe('splitStatements', () => {
  it('splits on semicolons and reports offsets', () => {
    const src = 'SELECT 1; SELECT 2;';
    const st = splitStatements(src);
    expect(st.map((s) => s.text.trim())).toEqual(['SELECT 1;', 'SELECT 2;']);
    expect(src.slice(st[1].start, st[1].end)).toBe(' SELECT 2;');
  });
  it('ignores semicolons inside strings, $$ blocks and comments', () => {
    expect(splitStatements("REMEMBER 'a;b';").length).toBe(1);
    expect(splitStatements('REMEMBER $$a;b$$;').length).toBe(1);
    expect(splitStatements('SELECT 1 -- ; not a split\n;').length).toBe(1);
  });
  it('keeps a trailing statement without a semicolon', () => {
    expect(splitStatements('SELECT 1; SELECT 2').map((s) => s.text.trim())).toEqual(['SELECT 1;', 'SELECT 2']);
  });
  it('drops comment-only and empty input', () => {
    expect(splitStatements('-- just a comment\n')).toEqual([]);
    expect(splitStatements('   ;  ')).toEqual([]);
    expect(splitStatements('')).toEqual([]);
  });
});

describe('statementAt', () => {
  const src = 'SELECT 1;\nSELECT 2;\n';
  it('returns the statement under the cursor', () => {
    expect(statementAt(src, 2)).toBe('SELECT 1;');
    expect(statementAt(src, 12)).toBe('SELECT 2;');
  });
  it('falls back to the last statement before the cursor', () => {
    expect(statementAt(src, src.length)).toBe('SELECT 2;');
  });
  it('returns an empty string when there is nothing to run', () => {
    expect(statementAt('-- only a comment', 5)).toBe('');
  });
});
