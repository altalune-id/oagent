---
name: nuxt-integration
description: Using Pinia with Nuxt 4 in a client-only SPA (ssr false) - auto-imports, explicit store imports, plugins
---

# Nuxt Integration (SPA)

This page covers `@pinia/nuxt` in a Nuxt 4 app built with `ssr: false`, such as oagent's `ui/console`. There is no server render, so there is no state serialization or hydration to think about.

## Configuration

```ts
// nuxt.config.ts
export default defineNuxtConfig({
  modules: ['@pinia/nuxt'],
})
```

Since Pinia v4, `@vue/devtools-api` is a required peer dependency of `pinia`.

## Auto Imports

`@pinia/nuxt` registers these as auto-imports, so a store file can call them without an import:

- `defineStore()` - define stores
- `storeToRefs()` - extract reactive refs
- `usePinia()` - get the pinia instance
- `acceptHMRUpdate()` - HMR support

The module can also auto-import every store in `app/stores/`. oagent turns directory scanning off (`imports: { scan: false }` in `nuxt.config.ts`), so stores are imported explicitly where they are used:

```vue
<script setup lang="ts">
import { useOrgsStore } from '~/stores/orgs';

const orgs = useOrgsStore();
</script>
```

## Loading Data

In an SPA, a page or layout calls a store action from `<script setup>` or `onMounted`. A store can guard against duplicate loads itself, for example with a `loaded` flag and a shared in-flight promise:

```ts
let inflight: Promise<void> | null = null;

function ensureLoaded() {
  if (loaded.value) {
    return Promise.resolve();
  }
  inflight ??= load().finally(() => {
    inflight = null;
  });
  return inflight;
}
```

## Using Stores Outside Components

In route middleware, Nuxt plugins or other stores, the active pinia is already installed, so `useStore()` works. Pass `nuxtApp.$pinia` only when you run outside the Nuxt app context:

```ts
// app/middleware/auth.ts
export default defineNuxtRouteMiddleware((to) => {
  const store = useAuthStore();

  if (to.meta.requiresAuth && !store.isLoggedIn) {
    return navigateTo('/login');
  }
});
```

## Pinia Plugins with Nuxt

Register a Pinia plugin from a Nuxt plugin:

```ts
// app/plugins/pinia-logger.client.ts
import type { PiniaPluginContext } from 'pinia';

function logger({ store }: PiniaPluginContext) {
  store.$subscribe((mutation) => {
    console.log(`[${mutation.storeId}]: ${mutation.type}`);
  });
}

export default defineNuxtPlugin(({ $pinia }) => {
  $pinia.use(logger);
});
```

<!--
Source references:
- https://pinia.vuejs.org/ssr/nuxt.html
-->
