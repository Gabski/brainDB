// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

export function KindBadge({ label, color, title }: { label: string; color: string; title?: string }) {
  return (
    <span className="kind-badge" style={{ background: color }} title={title}>
      {label}
    </span>
  );
}
