// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { TAB_LABELS, type TabId } from '../../state/store';
import { Icon } from '../common/Icon';

/** Closable workspace tabs (arrow keys move between tabs, Delete closes). */
export function TabBar({ tabs, active, onActivate, onClose, onAdd }: { tabs: TabId[]; active: TabId; onActivate: (t: TabId) => void; onClose: (t: TabId) => void; onAdd?: () => void }) {
  return (
    <div className="tabbar" role="tablist" aria-label="Workspace tabs">
      {tabs.map((t, i) => (
        <div
          key={t}
          role="tab"
          tabIndex={t === active ? 0 : -1}
          aria-selected={t === active}
          className="tab"
          onClick={() => onActivate(t)}
          onAuxClick={(e) => e.button === 1 && onClose(t)}
          onKeyDown={(e) => {
            if (e.key === 'ArrowRight') onActivate(tabs[(i + 1) % tabs.length]);
            else if (e.key === 'ArrowLeft') onActivate(tabs[(i - 1 + tabs.length) % tabs.length]);
            else if (e.key === 'Delete') onClose(t);
          }}
        >
          {TAB_LABELS[t]}
          <span
            className="close"
            role="button"
            aria-label={'Close ' + TAB_LABELS[t]}
            onClick={(e) => {
              e.stopPropagation();
              onClose(t);
            }}
          >
            ×
          </span>
        </div>
      ))}
      {onAdd && (
        <button className="add" aria-label="Open a tab" title="Open a tab (Ctrl+K)" onClick={onAdd}>
          <Icon name="add" />
        </button>
      )}
    </div>
  );
}
