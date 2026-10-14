---
name: deployment
description: Shipping a client-only Nuxt app as static files embedded in a Go binary
---

# Deployment (static SPA)

oagent's console is never deployed as a Nuxt server. `nuxt generate` with `ssr: false` writes a static SPA shell plus assets, and the Go binary embeds and serves them.

## Build

```bash
# from the repo root
make ui-console
```

This runs `pnpm --filter @oagent/console generate`, empties `internal/web/console/public/` (keeping `.gitkeep`), and copies `ui/console/.output/public/.` into it. `internal/web/console/console.go` embeds that folder with `//go:embed all:public`. Rebuild the Go binary afterwards. `make ui` builds every embedded web asset (web, MCP UI, console).

## Static Generation

```bash
nuxt generate
```

Output goes to `.output/public/`. With `ssr: false`, every route gets the same `index.html` shell and the client router takes over. oagent clears the prerender route list (`hooks: { 'prerender:routes': ({ routes }) => routes.clear() }`) so generation does not crawl pages that need a signed-in API.

## Runtime Config in a Static Build

`runtimeConfig.public` values are baked into `index.html` at generate time; there is no Nuxt server to read `NUXT_PUBLIC_*` at request time. oagent leaves them empty (`apiUrl: ''`, `appVersion: ''`) and the Go handler (`internal/web/console/console.go`) rewrites them, the `/_nuxt/` asset prefix and `baseURL` in `index.html` per request. Add a new per-deployment value the same way: an empty default in `nuxt.config.ts` plus a rewrite in the Go handler.

## Serving Rules

The Go handler serves an existing file as it is and answers any other console path with `index.html` (`Cache-Control: no-cache`), so deep links like `/o/<slug>/...` work on reload.

## Dev Server

`nuxt dev` (port 8280 in oagent) proxies `/api` to the Go server on `localhost:5150` through `nitro.devProxy`. That proxy exists only in development.

<!--
Source references:
- https://nuxt.com/docs/4.x/getting-started/deployment#static-hosting
- https://nuxt.com/docs/4.x/guide/concepts/rendering#client-side-rendering
-->
