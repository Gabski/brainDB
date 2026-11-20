// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

// Story helpers: a mock-backed store/ApiProvider, a padded frame, and fixture indexes.
import { useEffect, useMemo, type ReactNode } from 'react';
import { action } from 'storybook/actions';
import { ApiProvider } from '../api/ApiProvider';
import { createMockClient, type MockOptions } from '../api/mockClient';
import { BrainStore, type UiState } from '../state/store';
import { StoreProvider } from '../state/StoreProvider';
import { buildIndex } from '../model/graph';
import { fixtureState } from '../fixtures';

export const g = buildIndex(fixtureState);
export const log = action('api');
export const noop = () => {};

/** Renders children inside a mock-client store. `init` loads state/journal/sources like the app. */
export function MockStore({ children, mock, initial, init = true, onStore }: { children: ReactNode; mock?: MockOptions; initial?: Partial<UiState>; init?: boolean; onStore?: (s: BrainStore) => void }) {
  const store = useMemo(() => {
    const client = createMockClient({ log: (m, ...a) => log(m, ...a), ...mock });
    const s = new BrainStore(client, { brain: mock?.state ?? fixtureState, theme: (document.documentElement.dataset.theme as 'dark' | 'light') ?? 'dark', ...initial });
    onStore?.(s);
    return s;
  }, []); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (init) store.init();
    return () => store.dispose();
  }, [store, init]);
  return (
    <ApiProvider client={store.api}>
      <StoreProvider store={store}>{children}</StoreProvider>
    </ApiProvider>
  );
}

/** Padded panel frame for presentational components. */
export function Frame({ children, width = 340, height, pad = 12 }: { children: ReactNode; width?: number | string; height?: number | string; pad?: number }) {
  return (
    <div style={{ background: 'var(--panel)', color: 'var(--fg)', padding: pad, width, height, minHeight: '100vh', boxSizing: 'border-box', overflow: 'auto' }}>{children}</div>
  );
}

/** Full-height flex frame for tabs. */
export function TabFrame({ children, height = '100vh' }: { children: ReactNode; height?: number | string }) {
  return <div style={{ height, display: 'flex', flexDirection: 'column', background: 'var(--bg)', color: 'var(--fg)' }}>{children}</div>;
}
