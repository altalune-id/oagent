---
name: data-fetching
description: useFetch, useAsyncData, and $fetch in a client-only Nuxt app
---

# Data Fetching

Nuxt provides `useFetch`, `useAsyncData` and `$fetch`. With `ssr: false` they all run in the browser only; there is no server render and no hydration payload.

> **In oagent:** console data comes from typed Connect clients (`useNuxtApp().$orgClient` and friends, created in `app/plugins/connect.client.ts`) called from Pinia store actions. Do not call `/api` with `$fetch` or `useFetch` for RPCs. Use these composables only for a non-RPC resource, and keep the result in the page or a store.

## Overview

- `$fetch` - Basic fetch utility (use for client-side events)
- `useFetch` - wrapper around $fetch with reactive state, keys and caching (use for component data)
- `useAsyncData` - the same for any async function
- `createUseFetch` / `createUseAsyncData` - factories to build typed custom composables with baked-in defaults

### On `await`

`await` changes client behavior:

- **With `await`** (default): setup pauses until data is ready; client-side navigation is blocked until it resolves (user stays put, optionally with `<NuxtLoadingIndicator>`).
- **Without `await`**: setup continues immediately, `data` starts at its default and fills in later; client navigation happens instantly and you handle `status`/`error` yourself.

Prefer the explicit `lazy` option (or `useLazyFetch`/`useLazyAsyncData`) for non-blocking intent. Note: `await`-ing a `lazy` call does **not** block client navigation — drop `lazy` if you want the wait.

## useFetch

Primary composable for fetching data in components:

```vue
<script setup lang="ts">
const { data, status, error, refresh, clear } = await useFetch('/api/posts')
</script>

<template>
  <div v-if="status === 'pending'">Loading...</div>
  <div v-else-if="error">Error: {{ error.message }}</div>
  <div v-else>
    <article v-for="post in data" :key="post.id">
      {{ post.title }}
    </article>
  </div>
</template>
```

### With Options

```ts
const { data } = await useFetch('/api/posts', {
  // Query parameters
  query: { page: 1, limit: 10 },
  // Request body (for POST/PUT)
  body: { title: 'New Post' },
  // HTTP method
  method: 'POST',
  // Only pick specific fields
  pick: ['id', 'title'],
  // Transform response
  transform: (posts) => posts.map(p => ({ ...p, slug: slugify(p.title) })),
  // Custom key for caching
  key: 'posts-list',
  // Don't block navigation
  lazy: true,
  // Don't fetch immediately
  immediate: false,
  // Default value
  default: () => [],
})
```

### Reactive Parameters

```vue
<script setup lang="ts">
const page = ref(1)
const { data } = await useFetch('/api/posts', {
  query: { page }, // Automatically refetches when page changes
})
</script>
```

### Computed URL

```vue
<script setup lang="ts">
const id = ref(1)
const { data } = await useFetch(() => `/api/posts/${id.value}`)
// Refetches when id changes
</script>
```

## useAsyncData

For wrapping any async function:

```vue
<script setup lang="ts">
const { data, error } = await useAsyncData('user', () => {
  return myCustomFetch('/user/profile')
})
</script>
```

### Multiple Requests

```vue
<script setup lang="ts">
const { data } = await useAsyncData('cart', async () => {
  const [coupons, offers] = await Promise.all([
    $fetch('/api/coupons'),
    $fetch('/api/offers'),
  ])
  return { coupons, offers }
})
</script>
```

## Custom Fetchers: createUseFetch / createUseAsyncData

Factory macros that produce a fully typed custom composable with pre-defined options. Must be an **exported** declaration inside `app/composables/` (Nuxt injects dedup keys at build time).

```ts
// app/composables/useAPI.ts
export const useAPI = createUseFetch({
  baseURL: 'https://api.nuxt.com',
  // shared interceptors, headers, etc.
  onResponseError({ response }) {
    if (response.status === 401) navigateTo('/login')
  },
})
```

```vue
<script setup lang="ts">
// Same signature/return as useFetch, with defaults applied
const { data } = await useAPI('/modules')
// Caller can still override any option
const { data: other } = await useAPI('/modules', { baseURL: 'https://other.com' })
</script>
```

**Default vs Override mode:**

```ts
// Plain object → options act as DEFAULTS (caller can override)
export const useAPI = createUseFetch({ baseURL: '/api', lazy: true })

// Function → options OVERRIDE caller's (enforce auth/baseURL)
export const useAPI = createUseFetch(callerOptions => ({
  baseURL: 'https://api.nuxt.com', // always enforced
}))
```

Use the **function form** when you need `useNuxtApp()` (called in setup context, not module scope):

```ts
// app/composables/useAPI.ts
export const useAPI = createUseFetch(callerOptions => ({
  $fetch: useNuxtApp().$api as typeof $fetch,
  ...callerOptions,
}))
```

`createUseAsyncData` works identically for wrapping arbitrary async functions:

```ts
// app/composables/useCachedData.ts
export const useCachedData = createUseAsyncData({
  getCachedData(key, nuxtApp) {
    return nuxtApp.payload.data[key] ?? nuxtApp.static.data[key]
  },
})
```

> Replaces the old "don't await your custom `useFetch` wrapper" caveat — use these factories instead of hand-rolled wrappers.

## $fetch

For client-side events (form submissions, button clicks):

```vue
<script setup lang="ts">
async function submitForm() {
  const result = await $fetch('/api/submit', {
    method: 'POST',
    body: { name: 'John' },
  })
}
</script>
```

With `ssr: false` there is no server pass, so `$fetch` in setup does not fetch twice; `useFetch`/`useAsyncData` still add keyed caching, `status` and `refresh`.

## Return Values

All composables return:

| Property | Type | Description |
|----------|------|-------------|
| `data` | `Ref<T>` | Fetched data (`undefined` until resolved) |
| `error` | `Ref<Error>` | Error if request failed |
| `status` | `Ref<'idle' \| 'pending' \| 'success' \| 'error'>` | Request status |
| `pending` | `Ref<boolean>` | Whether a request is in progress |
| `refresh` | `() => Promise` | Refetch data |
| `execute` | `() => Promise` | Alias for refresh |
| `clear` | `() => void` | Reset to default/idle and cancel pending requests |

> Prefer `status` over `pending` for fine-grained state. `useFetch` no longer accepts a top-level `timeout` option (still available on `useAsyncData`); use a `cache` option (`'default'`, `'no-store'`, `false`, etc.) for Fetch cache control.

## Lazy Fetching

Don't block navigation:

```vue
<script setup lang="ts">
// Using lazy option
const { data, status } = await useFetch('/api/posts', { lazy: true })

// Or use lazy variants
const { data, status } = await useLazyFetch('/api/posts')
const { data, status } = await useLazyAsyncData('key', fetchFn)
</script>
```

## Refresh & Watch

```vue
<script setup lang="ts">
const category = ref('tech')

const { data, refresh } = await useFetch('/api/posts', {
  query: { category },
  // Auto-refresh when category changes
  watch: [category],
})

// Manual refresh
const refreshData = () => refresh()
</script>
```

## Caching

`useFetch` generates its key from the URL **plus** the fetch options and the call's source location — so two `useFetch('/api/user')` calls in different components have **different** keys and each run their own request. To share data, pass the **same explicit `key`**:

```vue
<script setup lang="ts">
// In component A
const { data } = await useFetch('/api/user', { key: 'current-user' })

// In component B - uses cached data
const { data } = useNuxtData('current-user')
</script>
```

Refresh cached data globally:

```ts
// Refresh specific key
await refreshNuxtData('current-user')

// Refresh all data
await refreshNuxtData()

// Clear cached data
clearNuxtData('current-user')
```

## Interceptors

```ts
const { data } = await useFetch('/api/auth', {
  onRequest({ options }) {
    options.headers.set('Authorization', `Bearer ${token}`)
  },
  onRequestError({ error }) {
    console.error('Request failed:', error)
  },
  onResponse({ response }) {
    // Process response
  },
  onResponseError({ response }) {
    if (response.status === 401) {
      navigateTo('/login')
    }
  },
})
```

<!-- 
Source references:
- https://nuxt.com/docs/getting-started/data-fetching
- https://nuxt.com/docs/api/composables/use-fetch
- https://nuxt.com/docs/api/composables/use-async-data
- https://nuxt.com/docs/api/composables/create-use-fetch
- https://nuxt.com/docs/api/composables/create-use-async-data
- https://nuxt.com/docs/guide/recipes/custom-usefetch
-->
