<script setup lang="ts">
const { title, count } = defineProps<{ title: string, count?: number }>();
</script>

<template>
  <main class="card card-border card-sm max-w-xl bg-base-100">
    <div class="card-body gap-3">
      <header class="flex flex-wrap items-center gap-2">
        <span class="size-2 shrink-0 rounded-full bg-primary" aria-hidden="true" />
        <h1 class="card-title text-base">
          {{ title }}
        </h1>
        <span v-if="count !== undefined" class="text-sm text-base-content/60">· {{ count }}</span>
        <slot name="badges" />
      </header>
      <slot />
    </div>
  </main>
</template>
