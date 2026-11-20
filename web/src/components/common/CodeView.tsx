// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useMemo } from 'react';
import { tokenize, type Lang } from '../../model/highlight';

/** Read-only syntax-highlighted text. */
export function CodeView({ text, lang, maxHeight, className }: { text: string; lang: Lang; maxHeight?: number | string; className?: string }) {
  const toks = useMemo(() => (text.length > 400_000 ? [{ t: text }] : tokenize(text, lang)), [text, lang]);
  return (
    <pre className={'code ' + (className ?? '')} style={maxHeight ? { maxHeight } : undefined} tabIndex={0}>
      {toks.map((t, i) => (t.c ? <span key={i} className={'tk-' + t.c}>{t.t}</span> : t.t))}
    </pre>
  );
}
