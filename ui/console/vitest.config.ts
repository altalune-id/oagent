import { fileURLToPath } from 'node:url';
import base from '@oagent/vitest-config/node';
import { defineConfig, mergeConfig } from 'vitest/config';

export default mergeConfig(base, defineConfig({
  // Nuxt sets import.meta.client at build time; the console is a client-only SPA.
  define: { 'import.meta.client': 'true' },
  resolve: {
    alias: {
      '~~': fileURLToPath(new URL('.', import.meta.url)),
      '~': fileURLToPath(new URL('./app', import.meta.url)),
      '@shared/scope': fileURLToPath(new URL('../../scope', import.meta.url)),
      '@shared/apperror': fileURLToPath(new URL('../../apperror', import.meta.url)),
    },
  },
  test: {
    setupFiles: ['./test/nuxt-globals.ts'],
    coverage: {
      provider: 'v8',
      include: ['app/lib/**/*.ts', 'app/stores/**/*.ts', 'app/composables/**/*.ts', 'app/components/**/*.ts'],
      exclude: ['**/*.test.ts', '**/*.stories.ts', '**/*.d.ts', 'app/components/ui/types.ts'],
      reporter: ['text-summary', 'text'],
      thresholds: { lines: 90, statements: 90, functions: 90, branches: 85 },
    },
  },
}));
