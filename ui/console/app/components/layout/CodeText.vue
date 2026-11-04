<script setup lang="ts">
import { splitCode } from '~/lib/code/code';

const { text } = defineProps<{ text: string }>();

const parts = computed(() => splitCode(text));
</script>

<template>
  <template v-for="(part, i) in parts" :key="i">
    <code v-if="part.code" class="font-mono text-[0.92em]">{{ part.text }}</code><span v-else>{{ part.text }}</span>
  </template>
</template>
