// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { createContext, useContext, useSyncExternalStore, type ReactNode } from 'react';
import type { BrainStore, UiState } from './store';

const StoreContext = createContext<BrainStore | null>(null);

export function StoreProvider({ store, children }: { store: BrainStore; children: ReactNode }) {
  return <StoreContext.Provider value={store}>{children}</StoreContext.Provider>;
}

export function useStoreApi(): BrainStore {
  const s = useContext(StoreContext);
  if (!s) throw new Error('useStore() outside <StoreProvider>');
  return s;
}

/** Subscribe to a slice of the UI state. Selectors should return stable references. */
export function useStore<T>(selector: (s: UiState) => T): T {
  const store = useStoreApi();
  return useSyncExternalStore(store.subscribe, () => selector(store.get()));
}
