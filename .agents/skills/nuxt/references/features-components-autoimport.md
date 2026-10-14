---
name: components-auto-imports
description: Component auto-imports, lazy loading, and how to turn auto-imports off
---

# Components Auto-imports

Nuxt automatically imports Vue components from the `app/components/` directory by default.

> **In oagent:** `components: { dirs: [] }` in `nuxt.config.ts` turns this off (see "Disabling Component Auto-imports" below). Import each component from its file: `import AppButton from '~/components/ui/AppButton.vue';`. For a lazy component use `defineAsyncComponent(() => import('~/components/...'))`. The rest of this page explains the default behavior you will meet in Nuxt docs and other projects.

## Basic Auto-imports

```
components/
├── Button.vue         → <Button />
├── Card.vue           → <Card />
└── AppHeader.vue      → <AppHeader />
```

```vue
<template>
  <!-- No imports needed -->
  <AppHeader />
  <Card>
    <Button>Click me</Button>
  </Card>
</template>
```

## Naming Conventions

### Nested Directory Names

Component names include directory path:

```
components/
├── base/
│   └── Button.vue       → <BaseButton />
├── form/
│   ├── Input.vue        → <FormInput />
│   └── Select.vue       → <FormSelect />
└── ui/
    └── modal/
        └── Dialog.vue   → <UiModalDialog />
```

### Disable Path Prefix

```ts
// nuxt.config.ts
export default defineNuxtConfig({
  components: [
    {
      path: '~/components',
      pathPrefix: false, // Use filename only
    },
  ],
})
```

With `pathPrefix: false`:
```
components/base/Button.vue → <Button />
```

### Grouping Directories

Wrap a directory name in parentheses to group files without affecting the component name:

```
components/base/(foo)/Button.vue → <BaseButton />
```

### `pattern` / `ignore` Globs

Each `components` dir entry accepts `pattern` and `ignore` globs to scan non-standard layouts (e.g. domain-driven structure). When `pattern` is set, the `extensions` option is ignored:

```ts
export default defineNuxtConfig({
  components: [
    // ~/domains/blog/components/PostCard.vue => <PostCard />
    { path: '~/domains', pattern: '*/components/**', pathPrefix: false },
  ],
})
```

## Lazy Loading

Prefix with `Lazy` for dynamic imports:

```vue
<script setup lang="ts">
const showChart = ref(false)
</script>

<template>
  <!-- Component code loaded only when rendered -->
  <LazyHeavyChart v-if="showChart" />
  <button @click="showChart = true">Show Chart</button>
</template>
```

Benefits:
- Reduces initial bundle size
- Code-splits component into separate chunk
- Loads on-demand

## Dynamic Components

```vue
<script setup lang="ts">
import { SomeComponent } from '#components'

const dynamicComponent = resolveComponent('MyButton')
</script>

<template>
  <component :is="dynamicComponent" />
  <component :is="SomeComponent" />
</template>
```

## Direct Imports

Bypass auto-imports when needed:

```vue
<script setup lang="ts">
import { LazyMountainsList, NuxtLink } from '#components'
</script>
```

## Custom Directories

```ts
// nuxt.config.ts
export default defineNuxtConfig({
  components: [
    { path: '~/components/ui', prefix: 'Ui' },
    { path: '~/components/forms', prefix: 'Form' },
    '~/components', // Default, should come last
  ],
})
```

## Global Components

Register globally (creates async chunks):

```ts
// nuxt.config.ts
export default defineNuxtConfig({
  components: {
    global: true,
    dirs: ['~/components'],
  },
})
```

Or use `.global.vue` suffix:

```
components/
└── Icon.global.vue  → Available globally
```

## Disabling Component Auto-imports

```ts
// nuxt.config.ts
export default defineNuxtConfig({
  components: {
    dirs: [], // Disable auto-imports
  },
})
```

<!-- 
Source references:
- https://nuxt.com/docs/directory-structure/app/components
- https://nuxt.com/docs/guide/concepts/auto-imports#auto-imported-components
-->
