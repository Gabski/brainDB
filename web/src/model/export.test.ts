// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from 'vitest';
import { cellText, MIME, renderRows, type ExportFormat } from './export';

const COLS = ['id', 'text', 'n'];
const ROWS = [
  { id: 'f201', text: 'plain', n: 1 },
  { id: 'd121', text: 'with "quote", comma\nand newline', n: null },
];

describe('cellText', () => {
  it('renders null and undefined as empty', () => {
    expect(cellText(null)).toBe('');
    expect(cellText(undefined)).toBe('');
  });
  it('JSON-encodes objects and stringifies the rest', () => {
    expect(cellText({ a: 1 })).toBe('{"a":1}');
    expect(cellText([1, 2])).toBe('[1,2]');
    expect(cellText(false)).toBe('false');
    expect(cellText(0)).toBe('0');
  });
});

describe('renderRows', () => {
  it('json: keeps the column order and nulls missing cells', () => {
    const out = JSON.parse(renderRows(COLS, [{ id: 'x' }], 'json')) as Record<string, unknown>[];
    expect(out).toEqual([{ id: 'x', text: null, n: null }]);
    expect(Object.keys(out[0])).toEqual(COLS);
  });

  it('csv: quotes quotes, commas and newlines, and ends with CRLF', () => {
    const csv = renderRows(COLS, ROWS, 'csv');
    const lines = csv.split('\r\n');
    expect(lines[0]).toBe('id,text,n');
    expect(lines[1]).toBe('f201,plain,1');
    expect(lines[2]).toContain('"with ""quote"", comma\nand newline"');
    expect(csv.endsWith('\r\n')).toBe(true);
  });
  it('csv: renders a null cell as empty', () => {
    expect(renderRows(['n'], [{ n: null }], 'csv')).toBe('n\r\n\r\n');
  });

  it('md: escapes pipes and flattens newlines so the table survives', () => {
    const md = renderRows(COLS, ROWS, 'md');
    const lines = md.trimEnd().split('\n');
    expect(lines[1]).toBe('|---|---|---|');
    expect(lines.length).toBe(ROWS.length + 2); // header + separator + one line per row
    expect(lines[3]).toContain('comma and newline'); // the newline inside the cell became a space
    expect(renderRows(['t'], [{ t: 'a|b' }], 'md')).toContain('a\\|b');
  });

  it('xml: escapes attribute values and sanitises column names', () => {
    const xml = renderRows(['a b', '2x', 't'], [{ 'a b': '1', '2x': '2', t: '<a href="x"> & </a>' }], 'xml');
    expect(xml).toContain('a_b="1"');
    expect(xml).toContain('_2x="2"');
    expect(xml).toContain('&lt;a href=&quot;x&quot;&gt; &amp; &lt;/a&gt;');
  });

  it('yaml: quotes what would otherwise parse as another type', () => {
    const y = renderRows(['v'], [{ v: 'true' }, { v: 'plain' }, { v: '12' }, { v: 'two words' }, { v: null }], 'yaml');
    const vals = y.trim().split('\n').map((l) => l.replace('- v: ', ''));
    expect(vals).toEqual(['"true"', 'plain', '"12"', 'two words', 'null']);
  });
  it('yaml: starts each row with a dash and indents the rest', () => {
    const y = renderRows(COLS, [ROWS[0]], 'yaml').trimEnd().split('\n');
    expect(y[0]).toBe('- id: f201');
    expect(y[1]).toBe('  text: plain');
  });

  it('renders empty input without crashing in every format', () => {
    for (const f of ['json', 'yaml', 'xml', 'md', 'csv'] as ExportFormat[]) expect(typeof renderRows(COLS, [], f)).toBe('string');
    expect(JSON.parse(renderRows(COLS, [], 'json'))).toEqual([]);
  });

  it('has a mime type for every format', () => {
    for (const f of ['json', 'yaml', 'xml', 'md', 'csv'] as ExportFormat[]) expect(MIME[f]).toMatch(/\//);
  });
});
