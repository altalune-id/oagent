<script setup lang="ts">
import { useId, useSlots } from 'vue';

const { title, description = '', tone = 'default' } = defineProps<{
  title: string,
  description?: string,
  tone?: 'default' | 'danger',
}>();

const slots = useSlots();
const titleId = useId();
</script>

<template>
  <section class="card card-border bg-base-100" :class="{ 'border-error': tone === 'danger' }" :aria-labelledby="titleId">
    <div class="card-body gap-4">
      <div class="flex flex-wrap items-start justify-between gap-2">
        <div class="min-w-0 space-y-1">
          <h2 :id="titleId" class="card-title text-base">
            {{ title }}
          </h2>
          <p v-if="description" class="text-sm text-base-content/70">
            {{ description }}
          </p>
        </div>
        <div v-if="slots.actions" class="flex flex-wrap items-center gap-2">
          <slot name="actions" />
        </div>
      </div>
      <slot />
    </div>
  </section>
</template>
