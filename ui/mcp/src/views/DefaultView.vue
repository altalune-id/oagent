<script setup lang="ts">
defineProps<{ data: Record<string, unknown> }>();
</script>

<template>
  <main class="card">
    <header><span class="dot" /><strong>auth-server</strong></header>
    <pre>{{ JSON.stringify(data, null, 2) }}</pre>
  </main>
</template>
