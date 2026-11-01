<script setup lang="ts">
import type { Crumb } from './types';
import { useSlots } from 'vue';

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
    <nav v-if="crumbs.length" :aria-label="crumbsLabel" class="breadcrumbs py-0 text-sm text-base-content/70">
      <ul>
        <li v-for="(crumb, i) in crumbs" :key="i">
          <NuxtLink v-if="crumb.to" :to="crumb.to" class="link link-hover">
            {{ crumb.label }}
          </NuxtLink>
          <span v-else aria-current="page">{{ crumb.label }}</span>
        </li>
      </ul>
    </nav>
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div class="min-w-0 flex-1 space-y-1">
        <div class="flex flex-wrap items-center gap-2">
          <h1 :class="mono ? 'font-mono text-xl font-semibold break-all' : 'text-2xl font-semibold break-words'">
            {{ title }}
          </h1>
          <span v-if="preview" class="badge badge-info badge-soft badge-sm">{{ preview }}</span>
          <slot name="meta" />
        </div>
        <p v-if="description" class="text-base-content/70">
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
