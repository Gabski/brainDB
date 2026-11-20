// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

// Small deterministic helpers (seeded PRNG, FNV-1a hash, formatting, clipboard/download).

export const clamp = (x: number, a = 0, b = 1) => Math.max(a, Math.min(b, x));

export function mulberry32(seed: number): () => number {
  let a = seed;
  return () => {
    a |= 0;
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

/** FNV-1a over UTF-16 code units. */
export function hash(s: string): number {
  let h = 2166136261;
  for (const c of String(s)) {
    h ^= c.charCodeAt(0);
    h = Math.imul(h, 16777619);
  }
  return h >>> 0;
}

export const gauss = (r: () => number) => (r() + r() + r() - 1.5) / 0.87;

export const short = (s: string, n = 34) => (s.length > n ? s.slice(0, n - 1) + '…' : s);

const deacc = (s: string) => s.normalize('NFD').replace(/[̀-ͯ]/g, '');

/** 5-letter word stems (used only for the add-memory concept hints). */
export const stems = (s: string) =>
  new Set(
    deacc(String(s).toLowerCase())
      .split(/[^a-z0-9]+/)
      .filter((w) => w.length >= 3)
      .map((w) => w.slice(0, 5)),
  );

export const f2 = (x: number | undefined | null) => (x === undefined || x === null || Number.isNaN(x) ? '–' : x.toFixed(2));

export function downloadText(name: string, text: string, type = 'text/plain') {
  const a = document.createElement('a');
  a.href = URL.createObjectURL(new Blob([text], { type }));
  a.download = name;
  a.click();
  setTimeout(() => URL.revokeObjectURL(a.href), 1000);
}

export async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    return false;
  }
}

export function timeAgo(ts: number, now = Date.now()): string {
  const s = Math.max(0, Math.round((now - ts) / 1000));
  if (s < 60) return s + 's ago';
  if (s < 3600) return Math.round(s / 60) + 'm ago';
  if (s < 86400) return Math.round(s / 3600) + 'h ago';
  return Math.round(s / 86400) + 'd ago';
}
