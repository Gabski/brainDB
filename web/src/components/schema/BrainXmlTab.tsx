// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useCallback, useEffect, useState } from 'react';
import { useStoreApi } from '../../state/StoreProvider';
import { CodeEditor } from '../common/CodeEditor';
import { ErrorBox, Loading } from '../common/Async';
import { copyText, downloadText } from '../../model/util';
import { Icon } from '../common/Icon';

/** Full brain.xml: edit, apply (PUT), copy, download. */
export function BrainXmlTab() {
  const store = useStoreApi();
  const [xml, setXml] = useState('');
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);
  const load = useCallback(async () => {
    setLoading(true);
    setErr(null);
    try {
      setXml(await store.api.getBrainXml());
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, [store]);
  useEffect(() => {
    load();
  }, [load]);
  return (
    <div className="split-v">
      <div className="toolbar">
        <span className="xs muted">The whole brain: memory, concepts, synapses and context parameters. Edit and apply.</span>
        <span className="spacer" />
        <button className="btn primary" onClick={() => store.putBrainXml(xml)} disabled={loading}>
          Apply XML
        </button>
        <button className="btn" onClick={load}>
          <Icon name="refresh" /> From server
        </button>
        <button className="btn" onClick={async () => (await copyText(xml)) && store.toastMsg('Copied brain.xml')}>
          Copy
        </button>
        <button className="btn" onClick={() => downloadText('brain.xml', xml, 'application/xml')}>
          <Icon name="download" /> brain.xml
        </button>
      </div>
      <div className="tab-scroll">{loading ? <Loading /> : err ? <ErrorBox error={err} onRetry={load} /> : <CodeEditor value={xml} onChange={setXml} lang="xml" minRows={30} ariaLabel="brain.xml editor" />}</div>
    </div>
  );
}
