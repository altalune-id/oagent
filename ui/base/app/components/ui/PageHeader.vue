<script setup lang="ts">
import type { Crumb } from './types';
import { useSlots } from 'vue';
import Breadcrumbs from './Breadcrumbs.vue';

const { title, description = '', mono = false, crumbs = [], preview = '', crumbsLabel = 'Breadcrumb' } = defineProps<{
  title: string,
  description?: string,
  mono?: boolean,
  crumbs?: Crumb[],
  preview?: string,
  crumbsLabel?: string,
}>();

const slots = useSlots();
</script>

<template>
  <header class="flex flex-col gap-2">
    <Breadcrumbs v-if="crumbs.length" :crumbs="crumbs" :label="crumbsLabel" />
    <div class="flex flex-wrap justify-between gap-3" :class="mono ? 'items-start' : 'items-center'">
      <div class="min-w-0">
        <div class="flex flex-wrap items-center gap-2">
          <h1 :class="mono ? 'font-mono text-lg font-medium break-all sm:text-xl' : 'text-xl font-semibold break-words'">
            {{ title }}
          </h1>
          <span v-if="preview" class="badge badge-info badge-soft text-base-content badge-sm">{{ preview }}</span>
          <slot name="meta" />
        </div>
        <p v-if="description" class="text-ui text-base-content/70">
          {{ description }}
        </p>
        <slot name="status" />
      </div>
      <div v-if="slots.actions" class="flex flex-wrap items-center gap-2">
        <slot name="actions" />
      </div>
    </div>
  </header>
</template>
