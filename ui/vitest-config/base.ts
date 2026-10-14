import process from 'node:process';
import { configDefaults, defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    globals: false,
    passWithNoTests: true,
    reporters: process.env.CI ? ['default', 'github-actions'] : ['default'],
    testTimeout: 10_000,
    hookTimeout: 10_000,
    exclude: [...configDefaults.exclude, '**/.nuxt/**', '**/.output/**', '**/storybook-static/**'],
  },
});
