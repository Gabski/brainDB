// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

// API token (server started with BRAINDB_TOKEN). Read from `?token=` on load or from the token
// prompt, kept in sessionStorage. Storage access can throw (private mode, sandboxed iframes).
const KEY = 'braindb.token';
let memToken: string | null = null;

export function initTokenFromUrl(): void {
  try {
    const u = new URL(window.location.href);
    const t = u.searchParams.get('token');
    if (t) {
      setToken(t);
      u.searchParams.delete('token');
      window.history.replaceState(null, '', u.toString());
    }
  } catch {
    /* ignore */
  }
}

export function getToken(): string | null {
  if (memToken) return memToken;
  try {
    memToken = window.sessionStorage.getItem(KEY);
  } catch {
    /* ignore */
  }
  return memToken;
}

export function setToken(t: string | null): void {
  memToken = t || null;
  try {
    if (t) window.sessionStorage.setItem(KEY, t);
    else window.sessionStorage.removeItem(KEY);
  } catch {
    /* ignore */
  }
}
