<script setup lang="ts">
defineProps<{ text: string }>();
</script>

<template>
  <p role="note" class="flex items-center gap-1.5 text-xs text-base-content/70">
    <Icon name="lucide:flask-conical" size="14" class="shrink-0" aria-hidden="true" />
    {{ text }}
  </p>
</template>
