<script setup lang="ts">
import ViewShell from '../components/ViewShell.vue';

defineProps<{ data: Record<string, unknown> }>();
</script>

<template>
  <ViewShell title="OAgent">
    <pre class="overflow-auto font-mono text-xs">{{ JSON.stringify(data, null, 2) }}</pre>
  </ViewShell>
</template>
