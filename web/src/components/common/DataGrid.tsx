// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useEffect, useMemo, useRef, useState, type KeyboardEvent } from 'react';
import { cellText, MIME, renderRows, type ExportFormat } from '../../model/export';
import { copyText, downloadText } from '../../model/util';

export interface DataGridProps {
  columns: string[];
  rows: Record<string, unknown>[];
  /** Total matching rows (may exceed rows.length). */
  total?: number;
  elapsedMs?: number;
  /** Column holding the node id: row activation selects the node. */
  idColumn?: string;
  onRowActivate?: (row: Record<string, unknown>) => void;
  /** Cells to highlight (e.g. missing slots): returns a class name. */
  cellClass?: (row: Record<string, unknown>, col: string) => string | undefined;
  renderCell?: (row: Record<string, unknown>, col: string) => React.ReactNode;
  exportName?: string;
  height?: number | string;
  onCopied?: (what: string) => void;
}

const ROW_H = 28;
const OVERSCAN = 12;
const isNum = (v: unknown) => typeof v === 'number' || (typeof v === 'string' && /^-?\d+(\.\d+)?$/.test(v));

/**
 * Virtualised results grid: sticky header, sortable + resizable columns, row/cell selection with
 * keyboard navigation, copy cell (Ctrl+C) / row, export json/yaml/xml/md/csv, row count + elapsed ms.
 */
export function DataGrid({ columns, rows, total, elapsedMs, idColumn = 'id', onRowActivate, cellClass, renderCell, exportName = 'result', height, onCopied }: DataGridProps) {
  const [sort, setSort] = useState<{ col: string; dir: 1 | -1 } | null>(null);
  const [widths, setWidths] = useState<Record<string, number>>({});
  const [sel, setSel] = useState<{ r: number; c: number } | null>(null);
  const [scrollTop, setScrollTop] = useState(0);
  const [viewH, setViewH] = useState(400);
  const wrap = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const el = wrap.current;
    if (!el) return;
    const ro = new ResizeObserver(() => setViewH(el.clientHeight || 400));
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const sorted = useMemo(() => {
    if (!sort) return rows;
    const { col, dir } = sort;
    return [...rows].sort((a, b) => {
      const x = a[col],
        y = b[col];
      if (isNum(x) && isNum(y)) return (Number(x) - Number(y)) * dir;
      return cellText(x).localeCompare(cellText(y)) * dir;
    });
  }, [rows, sort]);

  const first = Math.max(0, Math.floor(scrollTop / ROW_H) - OVERSCAN);
  const last = Math.min(sorted.length, Math.ceil((scrollTop + viewH) / ROW_H) + OVERSCAN);
  const visible = sorted.slice(first, last);
  const w = (c: string) => widths[c] ?? (c === 'text' ? 360 : c === idColumn ? 110 : 140);

  const startResize = (c: string, e: React.PointerEvent) => {
    e.preventDefault();
    e.stopPropagation();
    const x0 = e.clientX,
      w0 = w(c);
    const move = (ev: PointerEvent) => setWidths((ws) => ({ ...ws, [c]: Math.max(50, w0 + ev.clientX - x0) }));
    const up = () => {
      window.removeEventListener('pointermove', move);
      window.removeEventListener('pointerup', up);
    };
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', up);
  };

  const copy = async (text: string, what: string) => {
    if (await copyText(text)) onCopied?.(what);
  };

  const ensureVisible = (r: number) => {
    const el = wrap.current;
    if (!el) return;
    const top = r * ROW_H,
      bottom = top + ROW_H + ROW_H; // header
    if (top < el.scrollTop) el.scrollTop = top;
    else if (bottom > el.scrollTop + el.clientHeight) el.scrollTop = bottom - el.clientHeight;
  };

  const onKey = (e: KeyboardEvent) => {
    if (!sorted.length) return;
    const cur = sel ?? { r: 0, c: 0 };
    let { r, c } = cur;
    if (e.key === 'ArrowDown') r = Math.min(sorted.length - 1, r + 1);
    else if (e.key === 'ArrowUp') r = Math.max(0, r - 1);
    else if (e.key === 'ArrowRight') c = Math.min(columns.length - 1, c + 1);
    else if (e.key === 'ArrowLeft') c = Math.max(0, c - 1);
    else if (e.key === 'PageDown') r = Math.min(sorted.length - 1, r + Math.floor(viewH / ROW_H));
    else if (e.key === 'PageUp') r = Math.max(0, r - Math.floor(viewH / ROW_H));
    else if (e.key === 'Home') r = 0;
    else if (e.key === 'End') r = sorted.length - 1;
    else if (e.key === 'Enter') {
      onRowActivate?.(sorted[r]);
      e.preventDefault();
      return;
    } else if ((e.ctrlKey || e.metaKey) && e.key === 'c') {
      copy(e.shiftKey ? renderRows(columns, [sorted[r]], 'csv') : cellText(sorted[r][columns[c]]), e.shiftKey ? 'row' : 'cell');
      e.preventDefault();
      return;
    } else return;
    e.preventDefault();
    setSel({ r, c });
    ensureVisible(r);
  };

  const exp = (f: ExportFormat) => downloadText(`${exportName}.${f}`, renderRows(columns, sorted, f), MIME[f]);

  return (
    <div className="pane" style={{ flex: 1, height }}>
      <div
        className="grid-wrap"
        ref={wrap}
        tabIndex={0}
        role="grid"
        aria-rowcount={sorted.length}
        aria-colcount={columns.length}
        onKeyDown={onKey}
        onScroll={(e) => setScrollTop(e.currentTarget.scrollTop)}
      >
        <table className="grid" style={{ width: 44 + columns.reduce((a, c) => a + w(c), 0) }}>
          <colgroup>
            <col style={{ width: 44 }} />
            {columns.map((c) => (
              <col key={c} style={{ width: w(c) }} />
            ))}
          </colgroup>
          <thead>
            <tr>
              <th className="rownum">#</th>
              {columns.map((c) => (
                <th
                  key={c}
                  scope="col"
                  aria-sort={sort?.col === c ? (sort.dir === 1 ? 'ascending' : 'descending') : 'none'}
                  onClick={() => setSort(sort?.col === c ? (sort.dir === 1 ? { col: c, dir: -1 } : null) : { col: c, dir: 1 })}
                  title={`Sort by ${c}`}
                  style={{ position: 'sticky' }}
                >
                  <span className="th-in">
                    <span className="ellipsis">{c}</span>
                    {sort?.col === c && <span className="sort">{sort.dir === 1 ? '▲' : '▼'}</span>}
                  </span>
                  <span className="resizer" onPointerDown={(e) => startResize(c, e)} onClick={(e) => e.stopPropagation()} />
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {first > 0 && (
              <tr aria-hidden style={{ height: first * ROW_H }}>
                <td colSpan={columns.length + 1} style={{ padding: 0, border: 0 }} />
              </tr>
            )}
            {visible.map((row, i) => {
              const r = first + i;
              return (
                <tr
                  key={r}
                  className={sel?.r === r ? 'sel' : ''}
                  aria-rowindex={r + 1}
                  onClick={() => setSel({ r, c: sel?.c ?? 0 })}
                  onDoubleClick={() => onRowActivate?.(row)}
                >
                  <td className="rownum">{r + 1}</td>
                  {columns.map((c, ci) => {
                    const v = row[c];
                    const cls = [isNum(v) ? 'num' : '', sel?.r === r && sel.c === ci ? 'cell-sel' : '', cellClass?.(row, c) ?? ''].join(' ');
                    return (
                      <td
                        key={c}
                        className={cls}
                        title={cellText(v)}
                        onClick={(e) => {
                          e.stopPropagation();
                          setSel({ r, c: ci });
                          if (c === idColumn && onRowActivate) onRowActivate(row);
                        }}
                      >
                        {renderCell ? renderCell(row, c) : v === null || v === undefined ? <span className="null">null</span> : c === idColumn && onRowActivate ? <span className="link mono" style={{ color: 'var(--accent)' }}>{cellText(v)}</span> : cellText(v)}
                      </td>
                    );
                  })}
                </tr>
              );
            })}
            {last < sorted.length && (
              <tr aria-hidden style={{ height: (sorted.length - last) * ROW_H }}>
                <td colSpan={columns.length + 1} style={{ padding: 0, border: 0 }} />
              </tr>
            )}
          </tbody>
        </table>
        {!rows.length && <div className="empty">No rows.</div>}
      </div>
      <div className="grid-foot">
        <span>
          <b style={{ color: 'var(--fg)' }}>{rows.length.toLocaleString()}</b> row{rows.length === 1 ? '' : 's'}
          {total !== undefined && total !== rows.length ? ` of ${total.toLocaleString()}` : ''}
        </span>
        {elapsedMs !== undefined && <span>{elapsedMs} ms</span>}
        {sel && (
          <>
            <button className="btn sm ghost" onClick={() => copy(cellText(sorted[sel.r]?.[columns[sel.c]]), 'cell')}>
              Copy cell
            </button>
            <button className="btn sm ghost" onClick={() => copy(renderRows(columns, [sorted[sel.r]], 'csv'), 'row')}>
              Copy row
            </button>
          </>
        )}
        <span className="spacer" />
        <span className="xs">Export</span>
        {(['json', 'yaml', 'xml', 'md', 'csv'] as ExportFormat[]).map((f) => (
          <button key={f} className="btn sm" onClick={() => exp(f)} disabled={!rows.length}>
            {f}
          </button>
        ))}
      </div>
    </div>
  );
}
