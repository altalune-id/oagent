<script setup lang="ts">
import type { StatItem } from './types';

defineProps<{ items: StatItem[] }>();
</script>

<template>
  <div class="grid gap-3 sm:grid-cols-3">
    <div v-for="item in items" :key="item.label" class="stat rounded-box border border-base-300 bg-base-100">
      <div class="stat-title">
        {{ item.label }}
      </div>
      <div class="stat-value text-3xl">
        {{ item.value }}
      </div>
    </div>
  </div>
</template>
