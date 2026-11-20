// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useEffect, useMemo } from 'react';
import type { ApiClient } from '../api/client';
import { ApiProvider } from '../api/ApiProvider';
import { BrainStore, type UiState } from '../state/store';
import { StoreProvider } from '../state/StoreProvider';
import { Workspace } from './Workspace';

/** Root: wires an ApiClient (real or mock) into the store and renders the workspace. */
export function App({ client, initial, autoInit = true }: { client: ApiClient; initial?: Partial<UiState>; autoInit?: boolean }) {
  const store = useMemo(() => new BrainStore(client, initial), [client]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (autoInit) store.init();
    return () => store.dispose();
  }, [store, autoInit]);
  return (
    <ApiProvider client={client}>
      <StoreProvider store={store}>
        <Workspace />
      </StoreProvider>
    </ApiProvider>
  );
}
