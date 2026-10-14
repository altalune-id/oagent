---
name: configuration
description: Nuxt configuration files including nuxt.config.ts, app.config.ts, and runtime configuration
---

# Nuxt Configuration

Nuxt uses configuration files to customize application behavior. The main configuration options are `nuxt.config.ts` for build-time settings and `app.config.ts` for runtime settings.

## nuxt.config.ts

The main configuration file at the root of your project:

```ts
// nuxt.config.ts
export default defineNuxtConfig({
  // Configuration options
  devtools: { enabled: true },
  modules: ['@nuxt/ui'],
})
```

### Path Aliases

The default `srcDir` is `app/`, so path aliases resolve as:

| Alias | Resolves to |
|-------|-------------|
| `~` / `@` | `<rootDir>/app` |
| `~~` / `@@` | `<rootDir>` (project root) |
| `#shared` | `<rootDir>/shared` |

Reference root-level paths with `~~`. oagent adds its own aliases for Go-shared code (`@shared/scope`, `@shared/apperror`) and keeps generated Connect code in `~~/gen`:

```ts
export default defineNuxtConfig({
  modules: ['~~/custom-modules/awesome.js'], // relative to rootDir
})
```

### Environment Overrides

Configure environment-specific settings:

```ts
export default defineNuxtConfig({
  $production: {
    ssr: false,
  },
  $development: {
    ssr: false,
    devServer: { port: 8280 },
  },
  $env: {
    staging: {
      // Staging environment config
    },
  },
})
```

Use `--envName` flag to select environment: `nuxt build --envName staging`

## Runtime Config

For values that need to be overridden via environment variables:

```ts
// nuxt.config.ts
export default defineNuxtConfig({
  runtimeConfig: {
    // Server-only keys
    apiSecret: '123',
    // Keys within public are exposed to client
    public: {
      apiBase: '/api',
    },
  },
})
```

Override with environment variables:

```ini
# .env
NUXT_API_SECRET=api_secret_token
NUXT_PUBLIC_API_BASE=https://api.example.com
```

Access in components/composables:

```vue
<script setup lang="ts">
const config = useRuntimeConfig()
// Server: config.apiSecret, config.public.apiBase
// Client: config.public.apiBase only
</script>
```

## App Config

For public tokens determined at build time (not overridable via env vars):

```ts
// app/app.config.ts
export default defineAppConfig({
  title: 'Hello Nuxt',
  theme: {
    dark: true,
    colors: {
      primary: '#ff0000',
    },
  },
})
```

Access in components:

```vue
<script setup lang="ts">
const appConfig = useAppConfig()
</script>
```

## runtimeConfig vs app.config

| Feature | runtimeConfig | app.config |
|---------|--------------|------------|
| Client-side | Hydrated | Bundled |
| Environment variables | Yes | No |
| Reactive | Yes | Yes |
| Hot module replacement | No | Yes |
| Non-primitive JS types | No | Yes |

**Use runtimeConfig** for secrets and values that change per environment.
**Use app.config** for public tokens, theme settings, and non-sensitive config.

## External Tool Configuration

Nuxt uses `nuxt.config.ts` as single source of truth. Configure external tools within it:

```ts
export default defineNuxtConfig({
  // Nitro configuration (in an SPA only the dev server uses it, e.g. nitro.devProxy)
  nitro: {
    // nitro options
  },
  // Vite configuration
  vite: {
    // vite options
    vue: {
      // @vitejs/plugin-vue options
    },
  },
  // PostCSS configuration
  postcss: {
    // postcss options
  },
})
```

### Environment-specific Vite Config

Top-level `vite` options are shared. Use `$client` to target only the client build (with `ssr: false` it is the build that matters):

```ts
export default defineNuxtConfig({
  vite: {
    $client: {
      build: { rollupOptions: { output: { manualChunks: { analytics: ['analytics-package'] } } } },
    },
  },
})
```

## Vue Configuration

Enable Vue experimental features:

```ts
export default defineNuxtConfig({
  vue: {
    propsDestructure: true,
  },
})
```

<!-- 
Source references:
- https://nuxt.com/docs/getting-started/configuration
- https://nuxt.com/docs/getting-started/upgrade
- https://nuxt.com/docs/guide/going-further/runtime-config
- https://nuxt.com/docs/api/nuxt-config
-->
