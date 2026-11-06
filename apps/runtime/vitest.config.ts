import node from '@oagent/vitest-config/node';
import { defineConfig, mergeConfig } from 'vitest/config';

export default mergeConfig(node, defineConfig({
  test: {
    coverage: {
      provider: 'v8',
      include: ['src/**/*.ts'],
      exclude: ['src/**/*.test.ts', 'src/test/**'],
      reporter: ['text-summary', 'text'],
      thresholds: { lines: 85, statements: 85, functions: 85, branches: 85 },
    },
  },
}));
