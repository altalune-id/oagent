---
name: vitest
description: Vitest 5 unit testing for oagent's ui/* packages. Use when writing or fixing a *.test.ts next to a console store, composable, component helper or lib file, mocking a Connect client or Nuxt global with vi, raising coverage to meet the ui/console thresholds, or editing a vitest.config.ts or the shared @oagent/vitest-config presets.
metadata:
  version: "2026.9.25"
  source: Adapted from antfu/skills@feb0f3a (MIT), generated from https://github.com/vitest-dev/vitest
---

# Vitest

## In oagent

- Vitest 5 and `@vitest/coverage-v8` come from the workspace `catalog:`; every package extends a preset from `ui/vitest-config` (`node` for pure TS, `browser` for happy-dom).
- `globals: false`: import `describe`, `it`, `expect`, `vi`, `beforeEach` from `'vitest'` in every file.
- Write `it(...)` inside a `describe(...)` block, as the existing tests do; do not switch to top-level `test(...)`.
- Tests sit next to the code (`app/stores/orgs.test.ts`); `ui/console/test/nuxt-globals.ts` stands in for Nuxt auto-imports and Connect clients (`setClient`, `connectError`).
- `ui/console` runs `vitest run --coverage` with thresholds (lines/statements/functions 90, branches 85); a drop below fails the run.
- Store tests call `setActivePinia(createPinia())` in `beforeEach` (see the pinia skill).
- Fixture ids are public ids, never UUIDs. Run all UI tests with `pnpm ui:test`. See docs/howto/console-page.md.


Vitest is a next-generation testing framework powered by Vite. It provides a Jest-compatible API with native ESM, TypeScript, and JSX support out of the box. Vitest shares the same config, transformers, resolvers, and plugins with your Vite app.

**Key Features:**
- Vite-native: Uses Vite's transformation pipeline for fast HMR-like test updates
- Jest-compatible: Drop-in replacement for most Jest test suites
- Smart watch mode: Only reruns affected tests based on module graph
- Native ESM, TypeScript, JSX support without configuration
- Multi-threaded workers for parallel test execution
- Built-in coverage via V8 or Istanbul
- Snapshot testing, mocking, and spy utilities

> The skill is based on Vitest 5.0.1, generated at 2026-09-25.

## Core

| Topic | Description | Reference |
|-------|-------------|-----------|
| Configuration | Vitest and Vite config integration, defineConfig usage | [core-config](references/core-config.md) |
| CLI | Command line interface, commands and options | [core-cli](references/core-cli.md) |
| Test API | test/it function, modifiers like skip, only, concurrent | [core-test-api](references/core-test-api.md) |
| Describe API | describe/suite for grouping tests and nested suites | [core-describe](references/core-describe.md) |
| Expect API | Assertions with toBe, toEqual, matchers and asymmetric matchers | [core-expect](references/core-expect.md) |
| Hooks | beforeEach, afterEach, beforeAll, afterAll, aroundEach | [core-hooks](references/core-hooks.md) |

## Features

| Topic | Description | Reference |
|-------|-------------|-----------|
| Mocking | Mock functions, modules, timers, dates with vi utilities | [features-mocking](references/features-mocking.md) |
| Snapshots | Snapshot testing with toMatchSnapshot and inline snapshots | [features-snapshots](references/features-snapshots.md) |
| Coverage | Code coverage with V8 or Istanbul providers | [features-coverage](references/features-coverage.md) |
| Test Context | Test fixtures, context.expect, test.extend for custom fixtures | [features-context](references/features-context.md) |
| Concurrency | Concurrent tests, parallel execution, sharding | [features-concurrency](references/features-concurrency.md) |
| Filtering | Filter tests by name, file patterns, tags | [features-filtering](references/features-filtering.md) |
| Test Tags | Label tests with tags to filter runs and apply shared options | [features-test-tags](references/features-test-tags.md) |
| Reporters | Built-in reporters, default selection, CI/output config | [features-reporters](references/features-reporters.md) |
| Benchmarking | Write benchmarks with the bench fixture (Tinybench) | [features-benchmarking](references/features-benchmarking.md) |

## Advanced

| Topic | Description | Reference |
|-------|-------------|-----------|
| Vi Utilities | vi helper: mock, spyOn, fake timers, hoisted, waitFor | [advanced-vi](references/advanced-vi.md) |
| Environments | Test environments: node, jsdom, happy-dom, custom | [advanced-environments](references/advanced-environments.md) |
| Type Testing | Type-level testing with expectTypeOf and assertType | [advanced-type-testing](references/advanced-type-testing.md) |
| Projects | Multi-project workspaces, different configs per project | [advanced-projects](references/advanced-projects.md) |
