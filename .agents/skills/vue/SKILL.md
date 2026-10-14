---
name: vue
description: Vue 3.5 Composition API with <script setup lang="ts"> for oagent's ui/console and ui/mcp. Use when writing or changing a .vue component or composable, declaring props/emits/v-model with defineProps/defineEmits/defineModel, choosing ref vs computed vs watch, or using Transition/Teleport/KeepAlive/custom directives.
metadata:
  version: "2026.9.25"
  source: Adapted from antfu/skills@feb0f3a (MIT), generated from https://github.com/vuejs/docs (CC BY 4.0)
---

# Vue

## In oagent

- Vue 3.5 (from the workspace `catalog:`) in `ui/console` (Nuxt 4 SPA, `ssr: false`) and `ui/mcp` (Vite single-file app). No SSR anywhere.
- Vue APIs (`ref`, `computed`, `watch`, ...) are auto-imported in both apps; shared `components/ui/*` files import them from `'vue'` explicitly. Follow the neighbouring files.
- Components and own composables are imported explicitly (`components.dirs: []`, `imports.scan: false` in `nuxt.config.ts`).
- Styling is Tailwind CSS 4 + daisyUI 5 classes in the template (`btn`, `dropdown`, `menu`); not UnoCSS. Visible text comes from `@nuxtjs/i18n` (`useI18n().t`), never hard-coded.
- Shared building blocks live in `app/components/ui/` and have Storybook 10 stories; reuse them before writing new markup.
- Show and pass public ids, never UUIDs; slugs only in route paths.
- Examples in `references/` use upstream style (no semicolons); write new code in oagent's lint style (see the eslint-config skill). See docs/howto/console-page.md.

> Based on Vue 3.5, generated at 2026-09-25. Always use Composition API with `<script setup lang="ts">`.

## Preferences

- Use TypeScript and `<script setup lang="ts">`; always Composition API, never Options API
- Use `ref` by default (that is what the console does); `shallowRef` only for large values replaced as a whole
- Use `computed` for derived values instead of syncing state with watchers
- Props: either `const props = defineProps<...>()` or Reactive Props Destructure with defaults (`const { label = 'Copy' } = defineProps<...>()`); both appear in the console. When you watch a destructured prop, pass a getter: `watch(() => label, ...)`
- Type emits with the tuple syntax: `defineEmits<{ select: [code: string] }>()`

## Core

| Topic | Description | Reference |
|-------|-------------|-----------|
| Script Setup & Macros | `<script setup>`, defineProps, defineEmits, defineModel, defineExpose, defineOptions, defineSlots, generics | [script-setup-macros](references/script-setup-macros.md) |
| Reactivity & Lifecycle | ref, shallowRef, computed, watch, watchEffect, effectScope, lifecycle hooks, composables | [core-new-apis](references/core-new-apis.md) |

## Features

| Topic | Description | Reference |
|-------|-------------|-----------|
| Built-in Components & Directives | Transition, Teleport, Suspense, KeepAlive, v-memo, custom directives | [advanced-patterns](references/advanced-patterns.md) |

## Quick Reference

### Component Template

Written in oagent's lint style (semicolons, commas in inline object types, braces on their own lines):

```vue
<script setup lang="ts">
import { computed, onMounted, watch } from 'vue';

const props = defineProps<{
  title: string,
  count?: number,
}>();

const emit = defineEmits<{
  update: [value: string],
}>();

const model = defineModel<string>();

const doubled = computed(() => (props.count ?? 0) * 2);

watch(() => props.title, (newVal) => {
  emit('update', newVal);
});

onMounted(() => {
  model.value ??= '';
});
</script>

<template>
  <div>{{ title }} - {{ doubled }}</div>
</template>
```

### Key Imports

```ts
// Reactivity
import { ref, shallowRef, computed, reactive, readonly, toRef, toRefs, toValue } from 'vue'

// Watchers
import { watch, watchEffect, watchPostEffect, onWatcherCleanup } from 'vue'

// Lifecycle
import { onMounted, onUpdated, onUnmounted, onBeforeMount, onBeforeUpdate, onBeforeUnmount } from 'vue'

// Utilities
import { nextTick, defineComponent, defineAsyncComponent } from 'vue'
```
