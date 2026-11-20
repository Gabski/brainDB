// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useState } from 'react';
import { CodeView } from '../common/CodeView';
import { ErrorBox, Loading } from '../common/Async';
import { copyText, downloadText } from '../../model/util';
import { Icon } from '../common/Icon';

export interface ContextViewerProps {
  format: 'xml' | 'md';
  onFormat: (f: 'xml' | 'md') => void;
  text: string | null;
  loading?: boolean;
  error?: string | null;
  onRefresh?: () => void;
}

/** What the model gets: the lit slice as XML or Markdown, with copy and download text.md. */
export function ContextViewer({ format, onFormat, text, loading, error, onRefresh }: ContextViewerProps) {
  const [copied, setCopied] = useState(false);
  return (
    <div className="pane" style={{ flex: 1 }}>
      <div className="toolbar">
        <div className="seg">
          <button aria-pressed={format === 'xml'} onClick={() => onFormat('xml')}>
            XML
          </button>
          <button aria-pressed={format === 'md'} onClick={() => onFormat('md')}>
            Markdown
          </button>
        </div>
        <button
          className="btn"
          disabled={!text}
          onClick={async () => {
            if (text && (await copyText(text))) {
              setCopied(true);
              setTimeout(() => setCopied(false), 1500);
            }
          }}
        >
          {copied ? <><Icon name="check" /> Copied</> : <><Icon name="copy" /> Copy</>}
        </button>
        <button className="btn" disabled={!text} onClick={() => text && downloadText(format === 'md' ? 'text.md' : 'context.xml', text, format === 'md' ? 'text/markdown' : 'application/xml')}>
          <Icon name="download" /> {format === 'md' ? 'text.md' : 'context.xml'}
        </button>
        {onRefresh && (
          <button className="btn ghost" onClick={onRefresh}>
            <Icon name="refresh" label="Refresh" />
          </button>
        )}
        <span className="spacer" />
        <span className="xs muted">
          {format === 'md' ? 'Context summary: focus, sections with signal path, activation tree, statistics, outdated versions.' : 'What the model gets: only the lit slice, sorted by activation, with refs and concepts.'}
          {text ? ` · ~${Math.ceil(text.length / 3.6)} tok` : ''}
        </span>
      </div>
      <div className="tab-scroll">{loading ? <Loading /> : error ? <ErrorBox error={error} onRetry={onRefresh} /> : text !== null ? <CodeView text={text} lang={format} /> : null}</div>
    </div>
  );
}
