// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useEffect, useState } from 'react';
import { useStore, useStoreApi } from '../../state/StoreProvider';
import type { ConnectorSuggestion, SearchMode } from '../../api/types';
import { SearchBar } from './SearchBar';
import { SearchResults } from './SearchResults';
import { NarrowBar, RecentBar } from './NarrowBar';
import { IngestReport } from './IngestReport';
import { SuggestionList } from './SuggestionList';
import { ConnectorSuggestions } from './ConnectorSuggestions';
import { ErrorBox, Loading } from '../common/Async';
import { Icon } from '../common/Icon';

/** Search & discoveries: search results per mode, ingest report, discover inner/bridges, suggested connectors. */
export function SearchTab() {
  const store = useStoreApi();
  const s = useStore((x) => x);
  const [conns, setConns] = useState<{ data?: ConnectorSuggestion[]; loading: boolean; error?: string }>({ loading: false });
  const g = s.index;
  const loadConns = async () => {
    setConns({ loading: true });
    try {
      const r = await store.api.inspect({ what: ['connectors'] });
      setConns({ data: r.connectors ?? [], loading: false });
    } catch (e) {
      setConns({ loading: false, error: e instanceof Error ? e.message : String(e) });
    }
  };
  useEffect(() => {
    if (s.discover.data && !conns.data && !conns.loading) loadConns();
  }, [s.discover.data]); // eslint-disable-line react-hooks/exhaustive-deps
  if (!g) return <Loading />;
  const go = (id: string) => store.select(id, true);
  const nothing = !s.search.result && !s.search.loading && !s.ingest.data && !s.ingest.loading && !s.discover.data && !s.discover.loading && !conns.data;
  return (
    <div className="split-v">
      <div className="toolbar">
        <div className="grow">
          <SearchBar
            query={s.search.query}
            mode={s.search.mode}
            sources={s.sources.data}
            selectedSources={s.search.sources}
            onSearch={(q, m) => store.runSearch(q, m)}
            onMode={(m) => store.setSearch({ mode: m })}
            onSources={(x) => store.setSearch({ sources: x })}
            k={s.search.k}
            onK={(k) => store.setSearch({ k })}
            busy={s.search.loading}
          />
        </div>
        <button className="btn" onClick={() => store.discover()}>
          <Icon name="discover" /> Discover
        </button>
        <button className="btn" onClick={loadConns}>
          <Icon name="connector" /> Suggest connectors
        </button>
      </div>
      {s.search.result && (
        <NarrowBar
          g={g}
          result={s.search.result}
          narrow={s.search.narrow}
          group={s.search.group}
          onNarrow={(p) => store.setNarrow(p)}
          onToggle={(grp, v) => store.toggleNarrow(grp, v)}
          onGroup={(gr) => store.setSearch({ group: gr })}
          onClear={() => store.clearNarrow()}
        />
      )}
      <div className="tab-scroll">
        {nothing && (
          <>
            <div className="empty">Type a query above (hybrid / vector / tree / graph), drop files, or use Discover.</div>
            <RecentBar items={s.searches} onPick={(q, m) => store.runSearch(q, m as SearchMode)} onClear={() => store.clearSearches()} />
          </>
        )}
        {s.ingest.loading && <Loading text="Ingesting files…" />}
        {s.ingest.error && <ErrorBox error={s.ingest.error} />}
        {s.ingest.data && <IngestReport g={g} result={s.ingest.data} onGo={go} />}
        {s.search.loading && <Loading text="Searching…" />}
        {s.search.error && <ErrorBox error={s.search.error} />}
        {s.search.result && (
          <SearchResults g={g} query={s.search.query} result={s.search.result} onGo={go} onFocus={(ids) => store.setFocus(ids)} elapsedMs={s.search.ms} narrow={s.search.narrow} group={s.search.group} />
        )}
        {s.discover.loading && <Loading text="Discovering…" />}
        {s.discover.error && <ErrorBox error={s.discover.error} onRetry={() => store.discover()} />}
        {s.discover.data && (
          <>
            <SuggestionList g={g} title="Missing synapses inside the context" list={s.discover.data.inner} onGo={go} onPreview={(u, v) => store.previewLink(u, v)} onAdd={(u, v, w) => store.addLink(u, v, w)} />
            <SuggestionList g={g} title="Bridges: context ↔ faded memory" list={s.discover.data.bridge} onGo={go} onPreview={(u, v) => store.previewLink(u, v)} onAdd={(u, v, w) => store.addLink(u, v, w)} />
          </>
        )}
        {conns.loading && <Loading text="Looking for connectors…" />}
        {conns.error && <ErrorBox error={conns.error} />}
        {conns.data && <ConnectorSuggestions g={g} list={conns.data} onGo={go} onCreate={(bdql) => store.runBdql(bdql, 'create connector')} />}
      </div>
    </div>
  );
}
