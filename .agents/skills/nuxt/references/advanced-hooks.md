---
name: lifecycle-hooks
description: Nuxt build-time and app runtime hooks
---

# Lifecycle Hooks

Nuxt provides hooks to tap into the build process and the application lifecycle. With `ssr: false` there is no server runtime, so Nitro runtime hooks do not apply.

## Build-time Hooks (Nuxt)

Used in `nuxt.config.ts` or modules:

### In nuxt.config.ts

```ts
// nuxt.config.ts
export default defineNuxtConfig({
  hooks: {
    'build:before': () => {
      console.log('Build starting...')
    },
    'pages:extend': (pages) => {
      // Add custom pages
      pages.push({
        name: 'custom',
        path: '/custom',
        file: '~/pages/custom.vue',
      })
    },
    'components:dirs': (dirs) => {
      // Add component directories
      dirs.push({ path: '~/extra-components' })
    },
  },
})
```

oagent uses one build hook: `'prerender:routes'` clears the route list so `nuxt generate` emits only the SPA shell.

### In Modules

```ts
// modules/my-module.ts
export default defineNuxtModule({
  setup(options, nuxt) {
    nuxt.hook('ready', async (nuxt) => {
      console.log('Nuxt is ready')
    })

    nuxt.hook('close', async (nuxt) => {
      console.log('Nuxt is closing')
    })

    nuxt.hook('modules:done', () => {
      console.log('All modules loaded')
    })
  },
})
```

### Common Build Hooks

| Hook | When |
|------|------|
| `ready` | Nuxt initialization complete |
| `close` | Nuxt is closing |
| `modules:done` | All modules installed |
| `build:before` | Before build starts |
| `build:done` | Build complete |
| `pages:extend` | Add/modify routes (before meta scan) |
| `pages:resolved` | After page meta is scanned (with `scanPageMeta`) |
| `components:dirs` | Component dirs being resolved |
| `imports:extend` | Auto-imports being resolved |
| `nitro:config` | Before Nitro config finalized |
| `vite:extend` | Vite context created |
| `vite:extendConfig` | Before Vite config finalized |

## App Hooks (Runtime)

Used in plugins and composables:

### In Plugins

```ts
// plugins/lifecycle.ts
export default defineNuxtPlugin((nuxtApp) => {
  nuxtApp.hook('app:created', (vueApp) => {
    console.log('Vue app created')
  })

  nuxtApp.hook('app:mounted', (vueApp) => {
    console.log('App mounted')
  })

  nuxtApp.hook('page:start', () => {
    console.log('Page navigation starting')
  })

  nuxtApp.hook('page:finish', () => {
    console.log('Page navigation finished')
  })

  nuxtApp.hook('page:loading:start', () => {
    console.log('Page loading started')
  })

  nuxtApp.hook('page:loading:end', () => {
    console.log('Page loading ended')
  })
})
```

### Common App Hooks

| Hook | When |
|------|------|
| `app:created` | Vue app created |
| `app:mounted` | Vue app mounted (client only) |
| `app:error` | Fatal error occurred |
| `page:start` | Page navigation starting |
| `page:finish` | Page navigation finished |
| `page:loading:start` | Loading indicator should show |
| `page:loading:end` | Loading indicator should hide |
| `link:prefetch` | Link is being prefetched |

### Using Runtime Hooks

```ts
// composables/usePageTracking.ts
export function usePageTracking() {
  const nuxtApp = useNuxtApp()

  nuxtApp.hook('page:finish', () => {
    trackPageView(useRoute().path)
  })
}
```

## Custom Hooks

### Define Custom Hook Types

```ts
// types/hooks.d.ts
import type { HookResult } from '@nuxt/schema'

declare module '#app' {
  interface RuntimeNuxtHooks {
    'my-app:event': (data: MyEventData) => HookResult
  }
}

declare module '@nuxt/schema' {
  interface NuxtHooks {
    'my-module:init': () => HookResult
  }
}
```

### Call Custom Hooks

```ts
// In a plugin
export default defineNuxtPlugin((nuxtApp) => {
  // Call custom hook
  nuxtApp.callHook('my-app:event', { type: 'custom' })
})

// In a module
export default defineNuxtModule({
  setup(options, nuxt) {
    nuxt.callHook('my-module:init')
  },
})
```

`await` the result of `callHook` rather than chaining `.then()`/`.catch()`; that also stays correct in Nuxt 5, where it may return `void`.

## useRuntimeHook

Call hooks at runtime from components:

```vue
<script setup lang="ts">
// Register a callback for a runtime hook
useRuntimeHook('app:error', (error) => {
  console.error('App error:', error)
})
</script>
```

## Hook Examples

### Page View Tracking

```ts
// plugins/analytics.client.ts
export default defineNuxtPlugin((nuxtApp) => {
  nuxtApp.hook('page:finish', () => {
    const route = useRoute()
    analytics.track('pageview', {
      path: route.path,
      title: document.title,
    })
  })
})
```

### Performance Monitoring

```ts
// plugins/performance.client.ts
export default defineNuxtPlugin((nuxtApp) => {
  let navigationStart: number

  nuxtApp.hook('page:start', () => {
    navigationStart = performance.now()
  })

  nuxtApp.hook('page:finish', () => {
    const duration = performance.now() - navigationStart
    console.log(`Navigation took ${duration}ms`)
  })
})
```

<!-- 
Source references:
- https://nuxt.com/docs/guide/going-further/hooks
- https://nuxt.com/docs/api/advanced/hooks
-->
