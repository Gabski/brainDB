// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import './styles/app.css';
import { App } from './app/App';
import { HttpApiClient, type ApiClient } from './api/client';
import { initTokenFromUrl } from './api/token';

async function boot() {
  initTokenFromUrl();
  const params = new URLSearchParams(window.location.search);
  let client: ApiClient;
  if (params.has('mock')) {
    // ?mock: run the UI on the bundled fixtures (no backend needed)
    const { createMockClient } = await import('./api/mockClient');
    client = createMockClient({ eventEvery: params.get('mock') === 'live' ? 6000 : 0 });
  } else {
    const http = new HttpApiClient('');
    const db = params.get('db');
    if (db) http.setDb(db);
    client = http;
  }
  try {
    document.documentElement.dataset.theme = localStorage.getItem('braindb.theme')?.includes('light') ? 'light' : 'dark';
  } catch {
    document.documentElement.dataset.theme = 'dark';
  }
  createRoot(document.getElementById('root')!).render(
    <StrictMode>
      <App client={client} />
    </StrictMode>,
  );
}
boot();
