<script setup lang="ts">
import type { Lane } from './runView';
import EventMark from './EventMark.vue';
import { lanes } from './runView';

const { labels } = defineProps<{ labels: Record<Lane, string> }>();
</script>

<template>
  <ul class="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
    <li v-for="l in lanes" :key="l" class="inline-flex items-center gap-1.5">
      <EventMark :lane="l" />{{ labels[l] }}
    </li>
  </ul>
</template>
