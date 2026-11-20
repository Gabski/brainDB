// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import { addons } from 'storybook/manager-api';
import { create } from 'storybook/theming';

addons.setConfig({
  theme: create({
    base: 'dark',
    brandTitle: 'brainDB by Gabriel Koziestański',
    brandTarget: '_self',
    fontBase: 'system-ui, -apple-system, "Segoe UI", Roboto, sans-serif',
    fontCode: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
  }),
});
