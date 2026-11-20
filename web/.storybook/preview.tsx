// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { Preview } from '@storybook/react-vite';
import { useEffect } from 'react';
import '../src/styles/app.css';

const preview: Preview = {
  globalTypes: {
    theme: {
      description: 'UI theme',
      toolbar: {
        title: 'Theme',
        icon: 'mirror',
        items: [
          { value: 'dark', title: 'Dark' },
          { value: 'light', title: 'Light' },
        ],
        dynamicTitle: true,
      },
    },
  },
  initialGlobals: { theme: 'dark' },
  parameters: {
    layout: 'fullscreen',
    controls: { expanded: true },
  },
  decorators: [
    (Story, ctx) => {
      const theme = (ctx.globals.theme as string) ?? 'dark';
      // set synchronously so stores created during this render pick the theme up
      document.documentElement.dataset.theme = theme;
      useEffect(() => {
        document.documentElement.dataset.theme = theme;
        document.body.style.overflow = 'auto';
      }, [theme]);
      return <Story />;
    },
  ],
};
export default preview;
