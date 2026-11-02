<script setup lang="ts">
import type { StatItem } from './types';

defineProps<{ items: StatItem[] }>();
</script>

<template>
  <div class="grid gap-3 sm:grid-cols-3">
    <div v-for="item in items" :key="item.label" class="stat rounded-box border border-base-300 bg-base-100 px-4 py-3">
      <div class="stat-title text-xs text-base-content/70">
        {{ item.label }}
      </div>
      <div class="stat-value text-2xl font-semibold tabular-nums">
        {{ item.value }}
      </div>
    </div>
  </div>
</template>
