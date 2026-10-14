---
name: vite-build
description: Vite production builds for single-page and single-entry bundles, and the JavaScript API
---

# Build

oagent builds two kinds of Vite output. Neither is a published npm library, and nothing is server-rendered.

## Single-file SPA (ui/mcp)

`ui/mcp` bundles a Vue app into one `index.html` with `vite-plugin-singlefile`, so the Go binary can serve it as one string:

```ts
import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vite'
import { viteSingleFile } from 'vite-plugin-singlefile'

export default defineConfig({
  plugins: [vue(), viteSingleFile()],
  build: { outDir: 'dist', emptyOutDir: true },
})
```

## One ES entry served by the Go web pages (ui/web)

`ui/web` uses `build.lib` only to turn one TypeScript entry into one ES module file that lands in `internal/web/static`. It is not a library for distribution: one entry, `formats: ['es']`, a fixed file name, and no `package.json` exports.

```ts
import { defineConfig } from 'vite'

export default defineConfig({
  build: {
    lib: {
      entry: 'src/password/index.ts',
      formats: ['es'],
      fileName: () => 'password.js',
    },
    outDir: '../../internal/web/static',
    emptyOutDir: false, // the folder also holds the Tailwind CLI output
    minify: true,
  },
})
```

Keep `emptyOutDir: false` when `outDir` is shared with other generated files.

## JavaScript API

### createServer

```ts
import { createServer } from 'vite'

const server = await createServer({
  configFile: false,
  root: import.meta.dirname,
  server: { port: 1337 },
})

await server.listen()
server.printUrls()
```

### build

Returns `RolldownOutput | RolldownOutput[] | RolldownWatcher` (was `RollupOutput` in Vite 7). On failure it throws a `BundleError` — inspect `e.errors`.

```ts
import { build } from 'vite'

await build({
  root: './project',
  build: { outDir: 'dist' },
})
```

### preview

```ts
import { preview } from 'vite'

const previewServer = await preview({
  preview: { port: 8080, open: true },
})
previewServer.printUrls()
```

### resolveConfig

```ts
import { resolveConfig } from 'vite'

const config = await resolveConfig({}, 'build')
```

### loadEnv

```ts
import { loadEnv } from 'vite'

const env = loadEnv('development', process.cwd(), '')
// Loads all env vars (empty prefix = no filtering)
```

<!--
Source references:
- https://vite.dev/guide/build
- https://vite.dev/guide/api-javascript
-->
