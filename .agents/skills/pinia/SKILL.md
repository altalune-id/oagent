---
name: pinia
description: Pinia 4 setup stores for oagent's Nuxt console. Use when adding or changing a store in ui/console/app/stores, wiring a store to a Connect client, sharing state between pages or stores, using storeToRefs, writing a store test, or adding a Pinia plugin.
metadata:
  version: "2026.9.25"
  source: Adapted from antfu/skills@feb0f3a (MIT), generated from https://github.com/vuejs/pinia
---

# Pinia

## In oagent

- `ui/console` uses Pinia 4 through `@pinia/nuxt` in a Nuxt 4 SPA (`ssr: false`): no server state, no hydration.
- Write setup stores only: `defineStore('orgs', () => { ... })` with `ref`/`computed` and plain functions; one store per file in `app/stores/`.
- `defineStore`, `ref` and `computed` are auto-imported; stores, components and own composables are imported explicitly (`imports.scan: false`).
- Stores call the Connect clients from `useNuxtApp()` (`$orgClient` and friends) and expose `items`, `loading`, `loaded`, `error` plus actions such as `load()` and `ensureLoaded()`.
- Errors shown to users go through `useErrorMessage()`; ids held in state are public ids, never UUIDs.
- Test each store next to it (`orgs.test.ts`) with `setActivePinia(createPinia())` and mocked clients from `test/nuxt-globals.ts`; `@pinia/testing` is not installed.
- See docs/howto/console-page.md for how a console page, its store and its tests fit together.

Pinia is the official state management library for Vue, designed to be intuitive and type-safe. It supports Options API and setup (Composition API) stores, with first-class TypeScript support and devtools integration. oagent uses setup stores.

> The skill is based on Pinia v4.0.3, generated at 2026-09-25.

## Core References

| Topic | Description | Reference |
|-------|-------------|-----------|
| Stores | Defining stores, state, getters, actions, storeToRefs, subscriptions | [core-stores](references/core-stores.md) |

## Features

### Extensibility

| Topic | Description | Reference |
|-------|-------------|-----------|
| Plugins | Extend stores with custom properties, state, and behavior | [features-plugins](references/features-plugins.md) |

### Composability

| Topic | Description | Reference |
|-------|-------------|-----------|
| Composables | Using Vue composables within stores (VueUse, etc.) | [features-composables](references/features-composables.md) |
| Composing Stores | Store-to-store communication, avoiding circular dependencies | [features-composing-stores](references/features-composing-stores.md) |

## Best Practices

| Topic | Description | Reference |
|-------|-------------|-----------|
| Testing | Unit testing with @pinia/testing, mocking, stubbing | [best-practices-testing](references/best-practices-testing.md) |
| Outside Components | Using stores in navigation guards, plugins, middlewares | [best-practices-outside-component](references/best-practices-outside-component.md) |

## Advanced

| Topic | Description | Reference |
|-------|-------------|-----------|
| Nuxt | Nuxt SPA integration, auto-imports, plugins | [advanced-nuxt](references/advanced-nuxt.md) |
| HMR | Hot module replacement for development | [advanced-hmr](references/advanced-hmr.md) |

## Key Recommendations

- **Use Setup Stores** (oagent uses no option stores)
- **Use `storeToRefs()`** when destructuring state/getters to preserve reactivity
- **Actions can be destructured directly** - they're bound to the store
- **Call stores inside functions** not at module scope
- **Call every `useStore()` before the first `await`** in an action or setup function
- **Test stores directly** with a fresh `createPinia()` per test and mocked API clients
