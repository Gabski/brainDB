// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { useMemo } from 'react';
import { matchRanges } from '../../model/browse';

/** Text with every occurrence of `terms` highlighted. Terms are matched case-insensitively. */
export function HiText({ text, terms }: { text: string; terms: string[] }) {
  const r = useMemo(() => matchRanges(text, terms), [text, terms]);
  if (!r.length) return <>{text}</>;
  const out: React.ReactNode[] = [];
  let pos = 0;
  r.forEach(([a, b], i) => {
    if (a > pos) out.push(text.slice(pos, a));
    out.push(
      <mark className="hl" key={i}>
        {text.slice(a, b)}
      </mark>,
    );
    pos = b;
  });
  if (pos < text.length) out.push(text.slice(pos));
  return <>{out}</>;
}
