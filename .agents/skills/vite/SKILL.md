---
name: vite
description: Vite 8 (Rolldown) config, plugin API, asset handling and builds for oagent's ui/* packages. Use when editing ui/mcp/vite.config.ts or ui/web/vite.config.ts, adding a Vite plugin, using import.meta.glob/import.meta.env or ?raw/?url imports, changing build output that the Go binary embeds, or migrating config from Rollup/esbuild options to Rolldown/Oxc.
metadata:
  version: "2026.9.25"
  source: Adapted from antfu/skills@feb0f3a (MIT), generated from https://github.com/vitejs/vite
---

# Vite

## In oagent

- Vite 8 (Rolldown + Oxc) comes from the pnpm 10 workspace `catalog:` in `pnpm-workspace.yaml`; reference it as `"vite": "catalog:"`, never a pinned copy per package.
- `ui/mcp` is a Vite + Vue single-file SPA (`vite-plugin-singlefile`, `unplugin-auto-import` for Vue APIs); `pnpm ui:mcp` builds it.
- `ui/web` uses Vite only to emit one ES module (`build.lib`, one entry) into `internal/web/static`; its CSS comes from the Tailwind CLI, not Vite.
- `ui/console` is Nuxt 4; change its Vite settings through `vite:` in `nuxt.config.ts` (see the nuxt skill), not a `vite.config.ts`.
- No SSR and no published libraries: every build is client code embedded in the Go binary.
- Styling is Tailwind CSS 4 + daisyUI 5 (`@tailwindcss/vite` in the console), not UnoCSS.
- See docs/howto/README.md for the build and embed workflow.

> Based on Vite v8.3.1 (Rolldown-powered), generated at 2026-09-25. Vite 8 uses the Rolldown bundler and the Oxc transformer instead of esbuild + Rollup.

Vite is a frontend build tool with a fast dev server (native ESM + HMR) and optimized production builds.

## Preferences

- Use TypeScript: prefer `vite.config.ts`
- Always use ESM, avoid CommonJS

## Core

| Topic | Description | Reference |
|-------|-------------|-----------|
| Configuration | `vite.config.ts`, `defineConfig`, conditional configs, `loadEnv` | [core-config](references/core-config.md) |
| Features | `import.meta.glob`, asset queries (`?raw`, `?url`), `import.meta.env`, HMR API | [core-features](references/core-features.md) |
| Plugin API | Vite-specific hooks, virtual modules, plugin ordering | [core-plugin-api](references/core-plugin-api.md) |

## Build

| Topic | Description | Reference |
|-------|-------------|-----------|
| Build | oagent's single-file SPA and single-entry builds, JavaScript API | [build](references/build.md) |

## Advanced

| Topic | Description | Reference |
|-------|-------------|-----------|
| Rolldown Migration | Vite 8 changes: Rolldown bundler, Oxc transformer, config migration | [rolldown-migration](references/rolldown-migration.md) |

## Quick Reference

### CLI Commands

```bash
vite              # Start dev server
vite build        # Production build
vite preview      # Preview production build
```

In oagent run them through the package scripts, for example `pnpm --filter @oagent/mcp-ui build`.

### Common Config

```ts
import { defineConfig } from 'vite'

export default defineConfig({
  plugins: [],
  resolve: { alias: { '@': '/src' } },
  server: { port: 3000, proxy: { '/api': 'http://localhost:8080' } },
  build: { target: 'esnext', outDir: 'dist' },
})
```

### Official Plugins

- `@vitejs/plugin-vue` - Vue 3 SFC support (used by ui/mcp; version from the workspace catalog)
- `@vitejs/plugin-vue-jsx` - Vue 3 JSX
- `@vitejs/plugin-legacy` - Legacy browser support
