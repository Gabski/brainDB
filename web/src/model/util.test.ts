// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from 'vitest';
import { clamp, f2, hash, mulberry32, short, stems, timeAgo } from './util';

describe('clamp', () => {
  it('clamps into [0,1] by default', () => {
    expect(clamp(-1)).toBe(0);
    expect(clamp(2)).toBe(1);
    expect(clamp(0.4)).toBe(0.4);
  });
  it('takes explicit bounds', () => {
    expect(clamp(5, 1, 3)).toBe(3);
  });
});

describe('hash', () => {
  it('is deterministic and differs between inputs', () => {
    expect(hash('abc')).toBe(hash('abc'));
    expect(hash('abc')).not.toBe(hash('abd'));
  });
  it('stays a non-negative 32-bit integer', () => {
    for (const s of ['', 'x', 'a much longer string with spaces', 'ąćęłńóśźż']) {
      const h = hash(s);
      expect(Number.isInteger(h)).toBe(true);
      expect(h).toBeGreaterThanOrEqual(0);
      expect(h).toBeLessThan(2 ** 32);
    }
  });
});

describe('mulberry32', () => {
  it('is reproducible for a seed and stays in [0,1)', () => {
    const a = mulberry32(42);
    const b = mulberry32(42);
    for (let i = 0; i < 20; i++) {
      const x = a();
      expect(x).toBe(b());
      expect(x).toBeGreaterThanOrEqual(0);
      expect(x).toBeLessThan(1);
    }
  });
  it('differs between seeds', () => {
    expect(mulberry32(1)()).not.toBe(mulberry32(2)());
  });
});

describe('short', () => {
  it('leaves text at or under the limit alone', () => {
    expect(short('abc', 3)).toBe('abc');
  });
  it('cuts with an ellipsis and never exceeds the limit', () => {
    expect(short('abcdef', 4)).toBe('abc…');
    expect(short('abcdef', 4).length).toBe(4);
  });
});

describe('f2', () => {
  it('formats two decimals and marks missing values', () => {
    expect(f2(0.456)).toBe('0.46');
    expect(f2(undefined)).toBe('–');
    expect(f2(null)).toBe('–');
    expect(f2(Number.NaN)).toBe('–');
    expect(f2(0)).toBe('0.00');
  });
});

describe('stems', () => {
  it('keeps 5-letter stems of words of 3+ letters and drops short ones', () => {
    const s = stems('Entra login limits ok');
    expect(s.has('login')).toBe(true);
    expect(s.has('limit')).toBe(true);
    expect(s.has('ok')).toBe(false);
  });
  it('strips combining accents, but splits on letters that do not decompose', () => {
    expect(stems('gęślą jaźń').has('gesla')).toBe(true);
    // KNOWN LIMIT: 'ł' and 'ø' have no combining form, so a word is cut there
    // ("zażółć" -> "zazo" + "c"). stems() only feeds the concept hints, so this is cosmetic.
    expect([...stems('zażółć')]).toEqual(['zazo']);
  });
  it('de-duplicates words sharing a stem', () => {
    expect([...stems('limits limited')]).toEqual(['limit']);
  });
});

describe('timeAgo', () => {
  const now = 1_700_000_000_000;
  it('scales from seconds to days', () => {
    expect(timeAgo(now - 5_000, now)).toBe('5s ago');
    expect(timeAgo(now - 120_000, now)).toBe('2m ago');
    expect(timeAgo(now - 7_200_000, now)).toBe('2h ago');
    expect(timeAgo(now - 172_800_000, now)).toBe('2d ago');
  });
  it('never goes negative for a future timestamp', () => {
    expect(timeAgo(now + 10_000, now)).toBe('0s ago');
  });
});
