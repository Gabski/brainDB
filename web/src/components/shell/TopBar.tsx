// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { ReactNode } from 'react';
import type { Theme } from '../graph/sceneShared';
import { Icon } from '../common/Icon';

export function TopBar({ connection, theme, onTheme, onPalette, onAbout, children }: { connection: ReactNode; theme: Theme; onTheme: (t: Theme) => void; onPalette: () => void; onAbout: () => void; children?: ReactNode }) {
  return (
    <header className="topbar">
      <button className="logo btn ghost" onClick={onAbout} title="About brainDB" style={{ height: 30 }}>
        <img src="./logo.svg" alt="" width={20} height={20} /> brainDB
      </button>
      {connection}
      <button className="btn" onClick={onPalette} aria-label="Command palette" style={{ minWidth: 220, justifyContent: 'space-between', color: 'var(--mut)' }}>
        <span>Search commands, nodes…</span> <kbd>Ctrl K</kbd>
      </button>
      <span className="spacer" />
      {children}
      <button className="btn icon" aria-label={'Switch to ' + (theme === 'dark' ? 'light' : 'dark') + ' theme'} title="Toggle theme" onClick={() => onTheme(theme === 'dark' ? 'light' : 'dark')}>
        <Icon name={theme === 'dark' ? 'moon' : 'sun'} />
      </button>
    </header>
  );
}
