// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

// Client-side rendering of grid rows for copy/export.
export type ExportFormat = 'json' | 'yaml' | 'xml' | 'md' | 'csv';

const cell = (v: unknown) => (v === null || v === undefined ? '' : typeof v === 'object' ? JSON.stringify(v) : String(v));

function yamlScalar(v: unknown): string {
  if (v === null || v === undefined) return 'null';
  if (typeof v === 'number' || typeof v === 'boolean') return String(v);
  const s = typeof v === 'object' ? JSON.stringify(v) : String(v);
  return /^[\w./@-][\w ./@:-]*$/.test(s) && !/^(true|false|null|yes|no|-?\d[\d.]*)$/i.test(s) && !s.endsWith(' ') ? s : JSON.stringify(s);
}
const xmlEsc = (s: string) => s.replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c]!);
const xmlName = (s: string) => s.replace(/[^\w.-]/g, '_').replace(/^(\d)/, '_$1');

export function renderRows(columns: string[], rows: Record<string, unknown>[], f: ExportFormat): string {
  switch (f) {
    case 'json':
      return JSON.stringify(rows.map((r) => Object.fromEntries(columns.map((c) => [c, r[c] ?? null]))), null, 2);
    case 'yaml':
      return rows.map((r) => columns.map((c, i) => `${i ? '  ' : '- '}${c}: ${yamlScalar(r[c])}`).join('\n')).join('\n') + '\n';
    case 'xml':
      return '<rows>\n' + rows.map((r) => '  <row ' + columns.map((c) => `${xmlName(c)}="${xmlEsc(cell(r[c]))}"`).join(' ') + '/>').join('\n') + '\n</rows>\n';
    case 'md':
      return (
        [`| ${columns.join(' | ')} |`, `|${columns.map(() => '---').join('|')}|`, ...rows.map((r) => `| ${columns.map((c) => cell(r[c]).replace(/\|/g, '\\|').replace(/\n/g, ' ')).join(' | ')} |`)].join('\n') + '\n'
      );
    case 'csv': {
      const q = (s: string) => (/[",\r\n]/.test(s) ? '"' + s.replace(/"/g, '""') + '"' : s);
      return [columns.map(q).join(','), ...rows.map((r) => columns.map((c) => q(cell(r[c]))).join(','))].join('\r\n') + '\r\n';
    }
  }
}

export const MIME: Record<ExportFormat, string> = { json: 'application/json', yaml: 'application/yaml', xml: 'application/xml', md: 'text/markdown', csv: 'text/csv' };
export const cellText = cell;
