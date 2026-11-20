// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { StorybookConfig } from '@storybook/react-vite';

const config: StorybookConfig = {
  stories: ['../src/**/*.stories.@(ts|tsx)'],
  addons: [],
  framework: { name: '@storybook/react-vite', options: {} },
  core: { disableTelemetry: true },
  staticDirs: ['../public'],
  async viteFinal(cfg) {
    // Storybook has its own output dir; never let it touch the Go embed folder.
    if (String(cfg.build?.outDir ?? "").includes("internal/server")) throw new Error("storybook outDir points at the Go embed folder");
    return { ...cfg, build: { ...cfg.build, emptyOutDir: false, chunkSizeWarningLimit: 4000 } };
  },
};
export default config;
