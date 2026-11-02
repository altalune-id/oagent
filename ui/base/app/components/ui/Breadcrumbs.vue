<script setup lang="ts">
import type { Crumb } from './types';

const { crumbs, label = 'Breadcrumb' } = defineProps<{ crumbs: Crumb[], label?: string }>();
</script>

<template>
  <nav :aria-label="label" class="breadcrumbs min-w-0 text-ui">
    <ul>
      <li v-for="(crumb, i) in crumbs" :key="i">
        <NuxtLink v-if="crumb.to" :to="crumb.to">
          <span :class="{ 'font-mono text-xs': crumb.mono }">{{ crumb.label }}</span>
        </NuxtLink>
        <span v-else aria-current="page" :class="{ 'font-mono text-xs': crumb.mono }">{{ crumb.label }}</span>
      </li>
    </ul>
  </nav>
</template>
