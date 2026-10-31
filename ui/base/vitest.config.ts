import vue from '@oagent/vitest-config/vue';
import { defineConfig, mergeConfig } from 'vitest/config';
import { baseAliases } from './aliases.ts';

export default mergeConfig(vue, defineConfig({
  define: { 'import.meta.client': 'true' },
  resolve: {
    alias: baseAliases,
  },
  test: {
    setupFiles: ['./test/nuxt-globals.ts'],
    coverage: {
      provider: 'v8',
      include: ['app/lib/**/*.ts', 'app/composables/**/*.ts', 'app/components/**/*.ts'],
      exclude: ['**/*.test.ts', '**/*.stories.ts', '**/*.d.ts', 'app/components/ui/types.ts'],
      reporter: ['text-summary', 'text'],
      thresholds: { lines: 90, statements: 90, functions: 90, branches: 85 },
    },
  },
}));
