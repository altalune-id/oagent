import { defineConfig, mergeConfig } from 'vitest/config';

import base from './base.ts';

export default mergeConfig(
  base,
  defineConfig({
    test: {
      environment: 'node',
    },
  }),
);
