---
name: nuxt
description: Nuxt 4 client-only SPA (ssr false) for oagent's ui/console. Use when adding or changing a console page, layout, route or plugin under ui/console/app, editing ui/console/nuxt.config.ts (modules, runtimeConfig, aliases, i18n, devProxy), using Nuxt composables such as useRoute, navigateTo, useHead, useState or useRuntimeConfig, or building the console with nuxt generate for the Go binary.
metadata:
  version: "2026.9.25"
  source: Adapted from antfu/skills@feb0f3a (MIT), generated from https://github.com/nuxt/nuxt
---

# Nuxt

## In oagent

- `ui/console` is a Nuxt 4 SPA: `ssr: false`, built with `nuxt generate` and embedded into the Go binary (`make ui-console`). There is no Nuxt server, no `server/` directory and no server routes; the Go binary is the backend.
- Data comes from typed Connect clients provided by `app/plugins/connect.client.ts` and called from Pinia setup stores, not from `useFetch` against `/api`.
- Auto-imports are narrowed: `components: { dirs: [] }` and `imports: { scan: false }`. Vue, Nuxt and module APIs stay global; import components, stores and own composables from their files.
- Modules: `@nuxt/eslint`, `@nuxt/icon`, `@pinia/nuxt`, `@vueuse/nuxt`, `@nuxtjs/i18n` (`no_prefix` strategy, `en-US` and `id-ID`). Styling is Tailwind CSS 4 + daisyUI 5 through `@tailwindcss/vite`.
- Routes use slugs in the path (`/o/<slug>/...`); every id shown or sent is a public id, never a UUID.
- `runtimeConfig.public` (`apiUrl`, `appVersion`) stays empty at build time; the Go handler fills it in per request.
- See docs/howto/console-page.md for adding a page end to end.

> Based on Nuxt 4.x docs (https://nuxt.com/docs/4.x). oagent pins `nuxt@^4.5`. Upstream material for Nuxt 5, Nitro v3, server routes, SSR and hybrid rendering was removed because the console is client-only.

> **Directory layout:** the `srcDir` is `app/` — Vue app code (`app.vue`, `components/`, `composables/`, `pages/`, `layouts/`, `plugins/`, `stores/`) lives under `app/`, while `nuxt.config.ts`, `i18n/`, `test/` and generated `gen/` stay at the package root. The `~`/`@` aliases point at `app/`; use `~~`/`@@` for the root.

## Core

| Topic | Description | Reference |
|-------|-------------|-----------|
| Directory Structure | `app/` srcDir, aliases, conventions | [core-directory-structure](references/core-directory-structure.md) |
| Configuration | nuxt.config.ts, `$development`/`$production`, runtimeConfig, app.config.ts, Vite options | [core-config](references/core-config.md) |
| CLI Commands | Dev server, generate, prepare, typecheck | [core-cli](references/core-cli.md) |
| Routing | File-based routing, dynamic routes, navigation, middleware, layouts | [core-routing](references/core-routing.md) |
| Data Fetching | useFetch, useAsyncData, $fetch on the client | [core-data-fetching](references/core-data-fetching.md) |
| Modules | Using and writing local Nuxt modules, Nuxt Kit | [core-modules](references/core-modules.md) |
| Deployment | Static generate and go:embed | [core-deployment](references/core-deployment.md) |

## Features

| Topic | Description | Reference |
|-------|-------------|-----------|
| Composables Auto-imports | Vue/Nuxt composables, custom composables, useAnnouncer | [features-composables](references/features-composables.md) |
| Components Auto-imports | Default behavior, lazy components, disabling auto-imports | [features-components-autoimport](references/features-components-autoimport.md) |
| Built-in Components | NuxtLink, NuxtPage, NuxtLayout, NuxtAnnouncer, ClientOnly, and more | [features-components](references/features-components.md) |
| State Management | useState, Pinia integration | [features-state](references/features-state.md) |

## Best Practices

| Topic | Description | Reference |
|-------|-------------|-----------|
| Data Fetching Patterns | Lazy loading, parallel requests, keys, error handling | [best-practices-data-fetching](references/best-practices-data-fetching.md) |

## Advanced

| Topic | Description | Reference |
|-------|-------------|-----------|
| Lifecycle Hooks | Build-time and app runtime hooks | [advanced-hooks](references/advanced-hooks.md) |
