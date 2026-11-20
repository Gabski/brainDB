// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { defineConfig } from 'vitest/config';

// Unit tests cover the pure modules under src/model (filters, tokenizers, export rendering).
// Components are covered by the Storybook stories instead.
export default defineConfig({
  test: {
    include: ['src/**/*.test.ts'],
    environment: 'node',
    reporters: 'dot',
    coverage: { include: ['src/model/**'], reporter: ['text-summary'] },
  },
});
