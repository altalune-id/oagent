import { defineConfig, mergeConfig } from 'vitest/config'
import base from '@oagent/vitest-config/browser'

export default mergeConfig(base, defineConfig({
  test: {
    coverage: {
      provider: 'v8',
      include: ['src/**/*.ts'],
      exclude: ['**/*.test.ts', '**/*.d.ts'],
      reporter: ['text-summary', 'text'],
      thresholds: { lines: 90, statements: 90, functions: 90, branches: 85 },
    },
  },
}))
