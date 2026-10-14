---
name: data-fetching-best-practices
description: Patterns and best practices for efficient data fetching in Nuxt
---

# Data Fetching Best Practices

Data fetching patterns for a client-only (`ssr: false`) Nuxt app.

> **In oagent:** RPC data goes through Connect clients inside Pinia store actions (see the pinia skill), not through `useFetch`. The patterns below apply to the rare non-RPC fetch, and the ideas (parallel requests, lazy loading, explicit keys, error states) apply to store actions too.

## Choose the Right Tool

| Scenario | Use |
|----------|-----|
| oagent API (Connect RPC) | Store action calling `useNuxtApp().$xxxClient` |
| Component initial data (non-RPC) | `useFetch` or `useAsyncData` |
| User interactions (clicks, forms) | `$fetch` |
| Third-party SDK/API | `useAsyncData` with custom function |
| Multiple parallel requests | `useAsyncData` with `Promise.all` |
| Reusable API client with shared defaults | `createUseFetch` factory |

## Await vs Non-Await Usage

The `await` keyword controls whether data fetching **blocks navigation**:

### With `await` - Blocking Navigation

```vue
<script setup lang="ts">
// Navigation waits until data is fetched (uses Vue Suspense)
const { data } = await useFetch('/api/posts')
// data.value is available immediately after this line
</script>
```

- **Server**: Fetches data and includes it in the payload
- **Client hydration**: Uses payload data, no re-fetch
- **Client navigation**: Blocks until data is ready

### Without `await` - Non-Blocking (Lazy)

```vue
<script setup lang="ts">
// Navigation proceeds immediately, data fetches in background
const { data, status } = useFetch('/api/posts', { lazy: true })
// data.value may be undefined initially - check status!
</script>

<template>
  <div v-if="status === 'pending'">Loading...</div>
  <div v-else>{{ data }}</div>
</template>
```

Equivalent to using `useLazyFetch`:

```vue
<script setup lang="ts">
const { data, status } = useLazyFetch('/api/posts')
</script>
```

### When to Use Each

| Pattern | Use Case |
|---------|----------|
| `await useFetch()` | Data the page cannot render without |
| `useFetch({ lazy: true })` | Non-critical data, better perceived performance |
| `await useLazyFetch()` | Same as lazy, await only ensures initialization |

## Use Explicit Cache Keys

### ❌ Avoid: Auto-generated Keys

```vue
<script setup lang="ts">
// Key is auto-generated from file/line - can cause issues
const { data } = await useAsyncData(() => fetchPosts())
</script>
```

### ✅ Better: Explicit Keys

```vue
<script setup lang="ts">
// Explicit key for predictable caching
const { data } = await useAsyncData(
  'posts',
  () => fetchPosts(),
)

// Dynamic keys for parameterized data
const route = useRoute()
const { data: post } = await useAsyncData(
  `post-${route.params.id}`,
  () => fetchPost(route.params.id),
)
</script>
```

## Handle Loading States Properly

```vue
<script setup lang="ts">
const { data, status, error } = await useFetch('/api/posts')
</script>

<template>
  <div v-if="status === 'pending'">
    <SkeletonLoader />
  </div>
  <div v-else-if="error">
    <ErrorMessage :error="error" />
  </div>
  <div v-else>
    <PostList :posts="data" />
  </div>
</template>
```

## Use Lazy Fetching for Non-critical Data

```vue
<script setup lang="ts">
const id = useRoute().params.id

// Critical data - blocks navigation
const { data: post } = await useFetch(`/api/posts/${id}`)

// Non-critical data - doesn't block navigation
const { data: comments, status } = useFetch(`/api/posts/${id}/comments`, {
  lazy: true,
})

// Or use useLazyFetch
const { data: related } = useLazyFetch(`/api/posts/${id}/related`)
</script>

<template>
  <article>
    <h1>{{ post?.title }}</h1>
    <p>{{ post?.content }}</p>
  </article>

  <section v-if="status === 'pending'">Loading comments...</section>
  <CommentList v-else :comments="comments" />
</template>
```

## Minimize Payload Size

### Use `pick` for Simple Filtering

```vue
<script setup lang="ts">
const { data } = await useFetch('/api/users', {
  // Only include these fields in payload
  pick: ['id', 'name', 'avatar'],
})
</script>
```

### Use `transform` for Complex Transformations

```vue
<script setup lang="ts">
const { data } = await useFetch('/api/posts', {
  transform: (posts) => {
    return posts.map(post => ({
      id: post.id,
      title: post.title,
      excerpt: post.content.slice(0, 100),
      date: new Date(post.createdAt).toLocaleDateString(),
    }))
  },
})
</script>
```

## Parallel Fetching

### Fetch Independent Data with useAsyncData

```vue
<script setup lang="ts">
const { data } = await useAsyncData(
  'dashboard',
  async (_nuxtApp, { signal }) => {
    const [user, posts, stats] = await Promise.all([
      $fetch('/api/user', { signal }),
      $fetch('/api/posts', { signal }),
      $fetch('/api/stats', { signal }),
    ])
    return { user, posts, stats }
  },
)
</script>
```

### Multiple useFetch Calls

```vue
<script setup lang="ts">
// These run in parallel automatically
const [{ data: user }, { data: posts }] = await Promise.all([
  useFetch('/api/user'),
  useFetch('/api/posts'),
])
</script>
```

## Efficient Refresh Patterns

### Watch Reactive Dependencies

```vue
<script setup lang="ts">
const page = ref(1)
const category = ref('all')

const { data } = await useFetch('/api/posts', {
  query: { page, category },
  // Auto-refresh when these change
  watch: [page, category],
})
</script>
```

### Manual Refresh

```vue
<script setup lang="ts">
const { data, refresh, status } = await useFetch('/api/posts')

async function refreshPosts() {
  await refresh()
}
</script>
```

### Conditional Fetching

```vue
<script setup lang="ts">
const userId = ref<string | null>(null)

const { data, execute } = useFetch(() => `/api/users/${userId.value}`, {
  immediate: false, // Don't fetch until userId is set
})

// Later, when userId is available
function loadUser(id: string) {
  userId.value = id
  execute()
}
</script>
```

## Error Handling

```vue
<script setup lang="ts">
const { data, error, refresh } = await useFetch('/api/posts')

// Watch for errors if need event-like handling
watch(error, (err) => {
  if (err) {
    console.error('Fetch failed:', err)
    // Show toast, redirect, etc.
  }
}, { immediate: true })
</script>

<template>
  <div v-if="error">
    <p>Failed to load: {{ error.message }}</p>
    <button @click="refresh()">Retry</button>
  </div>
</template>
```

## Shared Data Across Components

```vue
<!-- ComponentA.vue -->
<script setup lang="ts">
const { data } = await useFetch('/api/user', { key: 'current-user' })
</script>

<!-- ComponentB.vue -->
<script setup lang="ts">
// Access cached data without refetching
const { data: user } = useNuxtData('current-user')

// Or refresh it
const { refresh } = await useFetch('/api/user', { key: 'current-user' })
</script>
```

## Centralize API Config with createUseFetch

Instead of hand-rolling a wrapper around `useFetch` (and worrying about whether to `await` it), use the `createUseFetch` factory. It produces a fully typed composable with shared `baseURL`, headers, and interceptors:

```ts
// app/composables/useAPI.ts
export const useAPI = createUseFetch({
  baseURL: 'https://api.nuxt.com',
  onRequest({ options }) {
    const { session } = useUserSession()
    if (session.value?.token) {
      options.headers.set('Authorization', `Bearer ${session.value.token}`)
    }
  },
  async onResponseError({ response }) {
    if (response.status === 401) await navigateTo('/login')
  },
})
```

```vue
<script setup lang="ts">
const { data: profile } = await useAPI('/me') // auth + 401 handling baked in
</script>
```

For lower-level control, create a custom `$fetch` instance in a plugin and wrap it with `useAsyncData` (or pass it to `createUseFetch`):

```ts
// app/plugins/api.ts
export default defineNuxtPlugin(() => {
  const api = $fetch.create({ baseURL: 'https://api.nuxt.com' })
  return { provide: { api } }
})
```

```vue
<script setup lang="ts">
const { $api } = useNuxtApp()
const { data } = await useAsyncData('modules', () => $api('/modules'))
</script>
```

## Avoid useAsyncData for Side Effects

### ❌ Wrong: Side Effects in useAsyncData

```vue
<script setup lang="ts">
// Don't trigger Pinia actions or side effects
await useAsyncData(() => store.fetchUser()) // Can cause issues
</script>
```

### ✅ Correct: Use callOnce for Side Effects

```vue
<script setup lang="ts">
await callOnce(async () => {
  await store.fetchUser()
})
</script>
```

<!-- 
Source references:
- https://nuxt.com/docs/4.x/getting-started/data-fetching
- https://nuxt.com/docs/4.x/api/composables/use-fetch
- https://nuxt.com/docs/4.x/api/composables/use-async-data
- https://nuxt.com/docs/4.x/api/composables/use-lazy-fetch
- https://nuxt.com/docs/4.x/guide/recipes/custom-usefetch
-->
