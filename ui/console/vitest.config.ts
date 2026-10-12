import { fileURLToPath } from 'node:url'
import { defineConfig, mergeConfig } from 'vitest/config'
import base from '@oagent/vitest-config/node'

export default mergeConfig(base, defineConfig({
  resolve: {
    alias: {
      '@shared/apperror': fileURLToPath(new URL('../../apperror', import.meta.url)),
    },
  },
}))
