<script setup lang="ts">
import { computed, useId, useSlots } from 'vue';

const { title, description = '', tone = 'default', flush = false } = defineProps<{
  title: string,
  description?: string,
  tone?: 'default' | 'danger',
  flush?: boolean,
}>();

const slots = useSlots();
const titleId = useId();
const head = computed(() => flush ? 'border-b border-base-300 px-4 py-2' : 'px-4 pt-4 pb-3');
</script>

<template>
  <section class="min-w-0 rounded-box border bg-base-100" :class="tone === 'danger' ? 'border-error' : 'border-base-300'" :aria-labelledby="titleId">
    <div class="flex flex-wrap items-center justify-between gap-x-3 gap-y-1" :class="head">
      <div class="min-w-0">
        <h2 :id="titleId" class="text-ui font-semibold">
          {{ title }}
        </h2>
        <p v-if="description" class="text-xs text-base-content/70">
          {{ description }}
        </p>
      </div>
      <div v-if="slots.actions" class="flex flex-wrap items-center gap-2">
        <slot name="actions" />
      </div>
    </div>
    <div :class="{ 'flex flex-col gap-4 px-4 pb-4': !flush }">
      <slot />
    </div>
  </section>
</template>
