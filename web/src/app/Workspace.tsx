// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { lazy, Suspense, useEffect, useMemo } from 'react';
import { useStore, useStoreApi } from '../state/StoreProvider';
import { TAB_LABELS, type TabId } from '../state/store';
import { VIEWS } from '../model/layout';
import { isConceptNode } from '../model/graph';
import { TopBar } from '../components/shell/TopBar';
import { ConnectionSelector } from '../components/shell/ConnectionSelector';
import { Navigator } from '../components/shell/Navigator';
import { TabBar } from '../components/shell/TabBar';
import { StatusBar } from '../components/shell/StatusBar';
import { CommandPalette, type Command } from '../components/shell/CommandPalette';
import { TokenPrompt } from '../components/shell/TokenPrompt';
import { AboutDialog } from '../components/shell/AboutDialog';
import { Inspector } from '../components/inspector/Inspector';
import { Toast } from '../components/common/Toast';
import { Loading } from '../components/common/Async';
import { QueryTab } from '../components/query/QueryTab';
import { ContextTab } from '../components/context/ContextTab';
import { TemplateTab } from '../components/template/TemplateTab';
import { SearchTab } from '../components/search/SearchTab';
import { BrowseTab } from '../components/browse/BrowseTab';
import { SchemaTab } from '../components/schema/SchemaTab';
import { SourcesTab } from '../components/sources/SourcesTab';
import { AgentFeed } from '../components/activity/AgentFeed';
import { BrainXmlTab } from '../components/schema/BrainXmlTab';
import { ImportTab, SettingsTab, NewDbWizardConnected } from '../components/db/DbTabs';
import { AccessTab } from '../components/access/AccessTab';
import { ReviewTab } from '../components/purpose/ReviewQueue';
import { isReviewer } from '../state/store';
import { UserMenu, ViewAsBanner, ViewAsSelector } from '../components/access/AccessUi';
import { ErrorBoundary } from '../components/common/ErrorBoundary';

// three.js is the heaviest dependency: load the Graph tab lazily
const GraphTab = lazy(() => import('../components/graph/GraphTab').then((m) => ({ default: m.GraphTab })));

function TabContent({ tab }: { tab: TabId }) {
  const store = useStoreApi();
  const feed = useStore((s) => s.feed);
  const live = useStore((s) => s.live);
  const g = useStore((s) => s.index);
  switch (tab) {
    case 'graph':
      return (
        <Suspense fallback={<Loading text="Loading 3D…" />}>
          <GraphTab />
        </Suspense>
      );
    case 'query':
      return <QueryTab />;
    case 'context':
      return <ContextTab />;
    case 'template':
      return <TemplateTab />;
    case 'browse':
      return <BrowseTab />;
    case 'search':
      return <SearchTab />;
    case 'schema':
      return <SchemaTab />;
    case 'sources':
      return <SourcesTab />;
    case 'activity':
      return <AgentFeed feed={feed} live={live} g={g} onGo={(id) => store.select(id, true)} />;
    case 'xml':
      return <BrainXmlTab />;
    case 'import':
      return <ImportTab />;
    case 'settings':
      return <SettingsTab />;
    case 'access':
      return <AccessTab />;
    case 'review':
      return <ReviewTab />;
  }
}

/** The full application shell: top bar, Navigator, tabbed workspace, Inspector, status bar. */
export function Workspace() {
  const store = useStoreApi();
  const s = useStore((x) => x);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        store.setPalette(!store.get().paletteOpen);
      } else if (e.altKey && e.key === 'ArrowLeft') {
        e.preventDefault();
        store.navBack();
      } else if (e.altKey && e.key === 'ArrowRight') {
        e.preventDefault();
        store.navForward();
      } else if (e.key === 'Escape' && !store.get().paletteOpen) store.cancel();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [store]);

  useEffect(() => {
    document.documentElement.dataset.theme = s.theme;
  }, [s.theme]);

  // admins get the Access tab and the users list for "View as…"
  const isAdmin = !!s.whoami?.admin;
  useEffect(() => {
    if (!isAdmin) return;
    if (!store.get().tabs.includes('access')) store.set((x) => ({ tabs: [...x.tabs, 'access'] }));
    if (!store.get().access.data && !store.get().access.loading) store.loadAccess();
  }, [isAdmin, store]);
  const reviewer = isReviewer(s.whoami);
  useEffect(() => {
    if (reviewer && !store.get().tabs.includes('review')) store.set((x) => ({ tabs: [...x.tabs, 'review'] }));
  }, [reviewer, store]);
  const viewAsUsers = (s.access.data?.users.rows ?? []).map((r) => String(r.name)).filter((n) => n !== s.whoami?.name);

  const commands = useMemo<Command[]>(() => {
    const out: Command[] = [];
    out.push({ id: 'run', group: 'Query', label: 'Run query (BDQL)', hint: 'Ctrl+Shift+Enter', run: () => store.runBdqlEditor() });
    for (const t of (Object.keys(TAB_LABELS) as TabId[]).filter((t) => (t !== 'access' || isAdmin) && (t !== 'review' || reviewer))) out.push({ id: 'tab:' + t, group: 'Open tab', label: TAB_LABELS[t], run: () => store.openTab(t) });
    for (const v of VIEWS) out.push({ id: 'view:' + v.id, group: 'View', label: 'Graph view: ' + v.label, run: () => (store.openTab('graph'), store.setView(v.id)) });
    out.push({ id: 'focus:goal', group: 'Focus', label: 'Focus = goal', run: () => store.focusGoal() });
    out.push({ id: 'focus:clear', group: 'Focus', label: 'Clear focus', run: () => store.setFocus([]) });
    out.push({ id: 'discover', group: 'Analyse', label: 'Discover new links in context', run: () => store.discover() });
    out.push({ id: 'source:add', group: 'Sources', label: 'Add source…', run: () => store.openTab('sources') });
    if (s.health?.manageDbs !== false) out.push({ id: 'db:new', group: 'Database', label: 'New database…', run: () => store.set({ wizardOpen: true }) });
    out.push({ id: 'theme', group: 'Settings', label: 'Toggle light/dark theme', run: () => store.setTheme(store.get().theme === 'dark' ? 'light' : 'dark') });
    out.push({ id: 'about', group: 'Help', label: 'About brainDB', run: () => store.set({ aboutOpen: true }) });
    for (const d of s.dbs.data ?? []) out.push({ id: 'db:' + d.name, group: 'Database', label: 'Switch to ' + d.name, run: () => store.switchDb(d.name) });
    for (const n of s.brain?.nodes ?? []) {
      out.push({ id: 'go:' + n.id, group: 'Go to', label: (isConceptNode(n) ? '◆ ' : '') + n.text, hint: n.id, run: () => store.select(n.id, true) });
      out.push({ id: 'f:' + n.id, group: 'Set focus', label: n.text, hint: n.id, run: () => (store.setFocus([n.id]), store.select(n.id, true)) });
    }
    for (const c of s.brain?.connectors ?? []) out.push({ id: 'go:' + c.id, group: 'Go to', label: '⧉ ' + c.label, hint: c.id, run: () => store.select(c.id) });
    return out;
  }, [store, s.brain, s.dbs.data, isAdmin, reviewer, s.health?.manageDbs]);

  return (
    <div className={'app' + (s.viewAs ? ' viewing-as' : '')}>
      <TopBar
        theme={s.theme}
        onTheme={(t) => store.setTheme(t)}
        onPalette={() => store.setPalette(true)}
        onAbout={() => store.set({ aboutOpen: true })}
        connection={
          <ConnectionSelector
            current={s.db ?? s.brain?.name ?? 'local'}
            dbs={s.dbs.data}
            sources={s.sources.data}
            live={s.live}
            onSwitchDb={(n) => store.switchDb(n)}
            onNewDb={() => store.set({ wizardOpen: true })}
            canManageDbs={s.health?.manageDbs !== false}
            onSource={() => store.openTab('sources')}
            onAddSource={() => store.openTab('sources')}
          />
        }
      >
        {/* The top bar must survive a bad field: a crash here used to unmount the whole app. */}
        <ErrorBoundary resetKey={s.whoami?.name ?? ''} label="User menu">
          {(isAdmin || s.viewAs) && <ViewAsSelector users={viewAsUsers.length ? viewAsUsers : s.viewAs ? [s.viewAs] : []} value={s.viewAs} onChange={(u) => store.setViewAs(u)} />}
          <UserMenu who={s.whoami} onSwitchToken={() => store.set({ needToken: true })} />
        </ErrorBoundary>
      </TopBar>
      {s.viewAs && <ViewAsBanner user={s.viewAs} onExit={() => store.setViewAs(null)} />}
      <Navigator
        g={s.index}
        selected={s.selected}
        sources={s.sources.data}
        templates={s.templates.data}
        saved={s.saved}
        history={s.history}
        entities={s.entityRows}
        onLoadEntities={() => store.loadEntities()}
        onSelect={(id) => store.select(id, true)}
        onKind={(k) => {
          store.setBdql({ mode: 'bdql', script: `SELECT id, kind, section, text, weight, activation\n  FROM memory\n  WHERE kind = ${k}\n  ORDER BY weight DESC\n  LIMIT 100;\n` });
          store.runBdqlEditor();
        }}
        onSource={() => store.openTab('sources')}
        onTemplate={(t) => store.openTemplate(t)}
        onQuery={(text) => {
          store.setBdql({ mode: 'bdql', script: text });
          store.openTab('query');
        }}
        onDeleteSaved={(n) => store.deleteSaved(n)}
      />
      <main className="workspace">
        <TabBar tabs={s.tabs} active={s.activeTab} onActivate={(t) => store.openTab(t)} onClose={(t) => store.closeTab(t)} onAdd={() => store.setPalette(true)} />
        <div className="tab-body" role="tabpanel" aria-label={TAB_LABELS[s.activeTab]}>
          <ErrorBoundary resetKey={s.activeTab} label={TAB_LABELS[s.activeTab]}>
            {s.tabs.length ? <TabContent tab={s.activeTab} /> : <div className="empty">All tabs closed. Press Ctrl+K to open one.</div>}
          </ErrorBoundary>
        </div>
      </main>
      <aside className="inspector" aria-label="Inspector">
        <Inspector />
      </aside>
      <StatusBar db={s.db} connection={s.brain?.name ?? 'local'} live={s.live} brain={s.brain} lastOp={s.lastOp} storage={s.serverStats?.storage ?? s.health?.storage} format={s.serverStats?.format ?? s.health?.format} quarantine={reviewer ? s.quarantine.data?.length : undefined} onQuarantine={() => store.openTab('review')} />
      {s.paletteOpen && <CommandPalette commands={commands} onClose={() => store.setPalette(false)} />}
      {s.needToken && <TokenPrompt onSubmit={(t) => store.submitToken(t)} onCancel={s.brain ? () => store.set({ needToken: false }) : undefined} />}
      {s.aboutOpen && <AboutDialog version={s.health?.version} onClose={() => store.set({ aboutOpen: false })} />}
      {s.wizardOpen && <NewDbWizardConnected />}
      {s.toast && <Toast key={s.toast.token} text={s.toast.text} ok={s.toast.ok} onClose={() => store.dismissToast()} />}
    </div>
  );
}
