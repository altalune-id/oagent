---
name: state-management
description: useState composable and shared state in Nuxt, with Pinia for domain state
---

# State Management

Nuxt provides `useState` for keyed reactive state shared across components. In oagent, small UI state (such as the theme) uses `useState`; domain data lives in Pinia setup stores.

## useState

A keyed `ref` shared across components:

```vue
<script setup lang="ts">
// State is shared by key 'counter' across all components
const counter = useState('counter', () => 0)
</script>

<template>
  <div>
    Counter: {{ counter }}
    <button @click="counter++">+</button>
    <button @click="counter--">-</button>
  </div>
</template>
```

## Creating Shared State

Define reusable state composables:

```ts
// composables/useUser.ts
export function useUser() {
  return useState<User | null>('user', () => null)
}

export function useLocale() {
  return useState('locale', () => 'en')
}
```

```vue
<script setup lang="ts">
// Same state instance everywhere
const user = useUser()
const locale = useLocale()
</script>
```

## Initializing State

Use `callOnce` to initialize state with async data:

```vue
<script setup lang="ts">
const config = useState('site-config')

await callOnce(async () => {
  config.value = await $fetch('/api/config')
})
</script>
```

## Best Practices

### ❌ Don't Define State Outside Setup

```ts
// ❌ Avoid - a module-level ref skips Nuxt's keyed state and devtools
export const globalState = ref({ user: null })
```

### ✅ Use useState Instead

```ts
// ✅ Keyed, shared state (oagent uses this for the theme in useTheme.ts)
export const useGlobalState = () => useState('global', () => ({ user: null }))
```

## Clearing State

```ts
// Clear specific state
clearNuxtState('counter')

// Clear multiple states
clearNuxtState(['counter', 'user'])

// Clear all state (use with caution)
clearNuxtState()
```

> **Nuxt 4:** `clearNuxtState` sets the state to `undefined` unless you pass `{ reset: true }` (or set `experimental.defaults.useState.resetOnClear: true`); then it resets to the `useState` init value. Nuxt 5 makes the reset the default.

## With Pinia

For domain state, use Pinia setup stores (oagent has `@pinia/nuxt` installed; see the pinia skill):

```ts
// app/stores/user.ts
export const useUserStore = defineStore('user', () => {
  const user = ref<User | null>(null)
  const isLoggedIn = computed(() => !!user.value)

  async function login(credentials: Credentials) {
    user.value = await $fetch('/api/login', {
      method: 'POST',
      body: credentials,
    })
  }

  return { user, isLoggedIn, login }
})
```

```vue
<script setup lang="ts">
const counterStore = useCounterStore()
const userStore = useUserStore()

// Initialize store data once
await callOnce(async () => {
  await userStore.fetchUser()
})
</script>
```

## Advanced: Locale Example

```ts
// composables/useLocale.ts
export function useLocale() {
  return useState('locale', () => useDefaultLocale().value)
}

export function useDefaultLocale(fallback = 'en-US') {
  const locale = ref(fallback)

  if (import.meta.client) {
    const navLang = navigator.language
    if (navLang) locale.value = navLang
  }

  return locale
}
```

## State Serialization

With SSR, `useState` values are serialized to JSON. oagent has no SSR, but keep values plain anyway; avoid:

- Functions
- Classes
- Symbols
- Circular references

```ts
// ❌ Won't work
useState('fn', () => () => console.log('hi'))
useState('instance', () => new MyClass())

// ✅ Works
useState('data', () => ({ name: 'John', age: 30 }))
useState('items', () => ['a', 'b', 'c'])
```

<!-- 
Source references:
- https://nuxt.com/docs/getting-started/state-management
- https://nuxt.com/docs/api/composables/use-state
- https://nuxt.com/docs/api/utils/clear-nuxt-state
-->
