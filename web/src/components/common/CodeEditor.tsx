// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useEffect, useLayoutEffect, useMemo, useRef, useState, type KeyboardEvent } from 'react';
import { tokenize, type Lang, type Token } from '../../model/highlight';

export interface CompletionItem {
  label: string;
  insert?: string;
  kind: 'keyword' | 'id' | 'kind' | 'section' | 'concept' | 'field' | 'template' | 'snippet' | 'var';
  detail?: string;
}

export interface LintMarker {
  line: number; // 1-based
  col: number; // 1-based
  message: string;
}

export interface CodeEditorProps {
  value: string;
  onChange: (v: string) => void;
  lang: Lang;
  /** Ctrl/Cmd+Enter: receives the caret offset. */
  onRun?: (cursor: number) => void;
  /** Ctrl/Cmd+Shift+Enter. */
  onRunAll?: () => void;
  completions?: CompletionItem[];
  markers?: LintMarker[];
  minRows?: number;
  maxHeight?: number | string;
  ariaLabel?: string;
  /** Open the completion popup on mount (stories). */
  initialCompletion?: { prefix: string };
  readOnly?: boolean;
}

const LINE_H = 20;
const PAD = 6;

function splitLines(toks: Token[]): Token[][] {
  const lines: Token[][] = [[]];
  for (const t of toks) {
    const parts = t.t.split('\n');
    parts.forEach((p, i) => {
      if (i > 0) lines.push([]);
      if (p) lines[lines.length - 1].push({ t: p, c: t.c });
    });
  }
  return lines;
}

/**
 * Dependency-free code editor: a transparent <textarea> over a highlighted <pre>, with line numbers,
 * lint markers, autocomplete (Ctrl+Space or typing) and run shortcuts.
 */
export function CodeEditor(props: CodeEditorProps) {
  const { value, onChange, lang, onRun, onRunAll, completions = [], markers = [], minRows = 6, maxHeight, ariaLabel, readOnly } = props;
  const ta = useRef<HTMLTextAreaElement>(null);
  const measure = useRef<HTMLSpanElement>(null);
  const [charW, setCharW] = useState(7.5);
  const [ac, setAc] = useState<{ prefix: string; start: number; items: CompletionItem[]; sel: number; line: number; col: number } | null>(null);

  const lines = useMemo(() => splitLines(tokenize(value, lang)), [value, lang]);
  const markByLine = useMemo(() => {
    const m = new Map<number, LintMarker[]>();
    for (const k of markers) m.set(k.line, [...(m.get(k.line) ?? []), k]);
    return m;
  }, [markers]);

  useLayoutEffect(() => {
    if (measure.current) setCharW(measure.current.getBoundingClientRect().width / 10 || 7.5);
  }, []);

  const filter = (prefix: string): CompletionItem[] => {
    const p = prefix.toLowerCase();
    if (!p) return completions.slice(0, 12);
    const starts = completions.filter((c) => c.label.toLowerCase().startsWith(p) && c.label.toLowerCase() !== p);
    const contains = completions.filter((c) => !c.label.toLowerCase().startsWith(p) && (c.label.toLowerCase().includes(p) || (c.detail ?? '').toLowerCase().includes(p)));
    return [...starts, ...contains].slice(0, 12);
  };

  const openAc = (force: boolean) => {
    const el = ta.current;
    if (!el) return;
    const pos = el.selectionStart;
    const before = value.slice(0, pos);
    const m = before.match(/[\w.$:]+$/);
    const prefix = m ? m[0] : '';
    if (!force && prefix.length < 1) return setAc(null);
    const items = filter(prefix);
    if (!items.length) return setAc(null);
    const lineStart = before.lastIndexOf('\n') + 1;
    setAc({ prefix, start: pos - prefix.length, items, sel: 0, line: before.split('\n').length, col: pos - prefix.length - lineStart });
  };

  useEffect(() => {
    if (!props.initialCompletion) return;
    const prefix = props.initialCompletion.prefix;
    const pos = value.lastIndexOf(prefix) + prefix.length;
    const before = value.slice(0, pos);
    const lineStart = before.lastIndexOf('\n') + 1;
    const items = filter(prefix);
    if (items.length) setAc({ prefix, start: pos - prefix.length, items, sel: 0, line: before.split('\n').length, col: pos - prefix.length - lineStart });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const accept = (it: CompletionItem) => {
    if (!ac) return;
    const ins = it.insert ?? it.label;
    const end = ac.start + ac.prefix.length;
    const next = value.slice(0, ac.start) + ins + value.slice(end);
    onChange(next);
    setAc(null);
    requestAnimationFrame(() => {
      const el = ta.current;
      if (el) {
        el.focus();
        el.selectionStart = el.selectionEnd = ac.start + ins.length;
      }
    });
  };

  const onKey = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    const mod = e.ctrlKey || e.metaKey;
    if (ac) {
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        setAc({ ...ac, sel: (ac.sel + 1) % ac.items.length });
        return;
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault();
        setAc({ ...ac, sel: (ac.sel - 1 + ac.items.length) % ac.items.length });
        return;
      }
      if ((e.key === 'Enter' && !mod) || e.key === 'Tab') {
        e.preventDefault();
        accept(ac.items[ac.sel]);
        return;
      }
      if (e.key === 'Escape') {
        e.preventDefault();
        e.stopPropagation();
        setAc(null);
        return;
      }
    }
    if (mod && e.key === 'Enter') {
      e.preventDefault();
      if (e.shiftKey) onRunAll?.();
      else onRun?.(e.currentTarget.selectionStart);
      return;
    }
    if (mod && e.key === ' ') {
      e.preventDefault();
      openAc(true);
      return;
    }
    if (e.key === 'Tab' && !e.shiftKey) {
      e.preventDefault();
      const el = e.currentTarget,
        s = el.selectionStart,
        en = el.selectionEnd;
      onChange(value.slice(0, s) + '  ' + value.slice(en));
      requestAnimationFrame(() => {
        el.selectionStart = el.selectionEnd = s + 2;
      });
    }
  };

  const rows = Math.max(minRows, lines.length);
  return (
    <div className="editor" style={{ maxHeight, minHeight: rows * LINE_H + PAD * 2 + 2 }}>
      <span ref={measure} aria-hidden style={{ position: 'absolute', visibility: 'hidden', font: '12.5px var(--mono)', whiteSpace: 'pre' }}>
        0123456789
      </span>
      <div className="gutter" aria-hidden>
        {Array.from({ length: rows }, (_, i) => {
          const mk = markByLine.get(i + 1);
          return (
            <div key={i} title={mk?.map((m) => `${m.line}:${m.col} ${m.message}`).join('\n')} style={mk ? { color: 'var(--bad)', fontWeight: 700 } : undefined}>
              {mk ? '● ' : ''}
              {i + 1}
            </div>
          );
        })}
      </div>
      <div className="area" style={{ minWidth: 'max-content', flex: 1 }}>
        <pre aria-hidden>
          {lines.map((ln, i) => {
            const mk = markByLine.get(i + 1);
            return (
              <div key={i} style={{ minHeight: LINE_H, position: 'relative', ...(mk ? { background: 'color-mix(in srgb, var(--bad) 10%, transparent)' } : {}) }}>
                {ln.length ? ln.map((t, j) => (t.c ? <span key={j} className={'tk-' + t.c}>{t.t}</span> : <span key={j}>{t.t}</span>)) : ' '}
                {mk?.map((m, k) => (
                  <span
                    key={'m' + k}
                    style={{ position: 'absolute', left: (m.col - 1) * charW, bottom: 1, width: Math.max(charW * 3, 12), borderBottom: '2px wavy var(--bad)', height: 3 }}
                  />
                ))}
              </div>
            );
          })}
        </pre>
        <textarea
          ref={ta}
          value={value}
          spellCheck={false}
          wrap="off"
          readOnly={readOnly}
          aria-label={ariaLabel ?? 'Code editor'}
          autoCapitalize="off"
          autoCorrect="off"
          onChange={(e) => {
            onChange(e.target.value);
            requestAnimationFrame(() => openAc(false));
          }}
          onKeyDown={onKey}
          onBlur={() => setTimeout(() => setAc(null), 150)}
          style={{ whiteSpace: 'pre', overflow: 'hidden' }}
        />
        {ac && (
          <ul
            role="listbox"
            className="ac-pop"
            style={{ position: 'absolute', left: ac.col * charW + 8, top: ac.line * LINE_H + PAD + 2 }}
          >
            {ac.items.map((it, i) => (
              <li
                key={it.kind + it.label + i}
                role="option"
                aria-selected={i === ac.sel}
                onMouseDown={(e) => {
                  e.preventDefault();
                  accept(it);
                }}
              >
                <span className={'ac-kind ac-' + it.kind}>{it.kind[0].toUpperCase()}</span>
                <span className="mono">{it.label}</span>
                {it.detail && <span className="ac-detail">{it.detail}</span>}
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}
