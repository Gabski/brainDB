// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useCallback, useState, type ReactNode } from 'react';

const KEY = 'braindb.collapsed';

const read = (): Record<string, boolean> => {
  try {
    return JSON.parse(window.localStorage.getItem(KEY) ?? '{}') as Record<string, boolean>;
  } catch {
    return {};
  }
};
const write = (v: Record<string, boolean>) => {
  try {
    window.localStorage.setItem(KEY, JSON.stringify(v));
  } catch {
    /* storage unavailable */
  }
};

/** Open/closed state of one collapsible section, remembered across reloads. */
export function useCollapsed(id: string, defaultOpen = true): [boolean, () => void] {
  const [open, setOpen] = useState(() => read()[id] ?? defaultOpen);
  const toggle = useCallback(() => {
    setOpen((o) => {
      const next = !o;
      write({ ...read(), [id]: next });
      return next;
    });
  }, [id]);
  return [open, toggle];
}

export interface SectionProps {
  /** Stable id: the open/closed state is stored under it. */
  id: string;
  title: ReactNode;
  count?: number | string;
  defaultOpen?: boolean;
  /** Shown in the header, right of the title (does not toggle the section). */
  aside?: ReactNode;
  children: ReactNode;
}

/** A section with a clickable header that remembers whether it was left open. */
export function Section({ id, title, count, defaultOpen = true, aside, children }: SectionProps) {
  const [open, toggle] = useCollapsed(id, defaultOpen);
  return (
    <section className={'sect' + (open ? ' open' : '')}>
      <h3 className="section-h sect-h">
        <button className="sect-btn" aria-expanded={open} onClick={toggle}>
          <span className="caret">{open ? '▾' : '▸'}</span>
          {title}
          {count !== undefined && <span className="count">{count}</span>}
        </button>
        {aside}
      </h3>
      {open && children}
    </section>
  );
}
