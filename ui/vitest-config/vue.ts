import { fileURLToPath } from 'node:url';
import vue from '@vitejs/plugin-vue';
import { defineConfig, mergeConfig } from 'vitest/config';

import node from './node.ts';

export default mergeConfig(
  node,
  defineConfig({
    plugins: [vue()],
    test: {
      setupFiles: [fileURLToPath(new URL('./vue-setup.ts', import.meta.url))],
    },
  }),
);
