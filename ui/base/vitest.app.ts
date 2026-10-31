import { fileURLToPath } from 'node:url';
import node from '@oagent/vitest-config/node';
import { defineConfig, mergeConfig } from 'vitest/config';
import { baseAliases } from './aliases.ts';

const globals = fileURLToPath(new URL('./test/nuxt-globals.ts', import.meta.url));

export function appVitestConfig(appUrl: string) {
  const at = (path: string) => fileURLToPath(new URL(path, appUrl));
  return mergeConfig(node, defineConfig({
    define: { 'import.meta.client': 'true' },
    resolve: {
      alias: { '~~': at('.'), '~': at('./app'), ...baseAliases },
    },
    test: {
      setupFiles: [globals],
      coverage: {
        provider: 'v8',
        include: ['app/lib/**/*.ts', 'app/stores/**/*.ts', 'app/composables/**/*.ts', 'app/components/**/*.ts'],
        exclude: ['**/*.test.ts', '**/*.stories.ts', '**/*.d.ts'],
        reporter: ['text-summary', 'text'],
        thresholds: { lines: 90, statements: 90, functions: 90, branches: 85 },
      },
    },
  }));
}
