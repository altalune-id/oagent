<script setup lang="ts">
import type { Lane } from './runView';
import { laneColor } from './runView';

const { lane } = defineProps<{ lane: Lane | null }>();

const shape: Record<Lane, string> = { model: 'rounded-full', tool: 'rounded-[2px]', approval: 'rotate-45 rounded-[1px]', error: '' };
</script>

<template>
  <Icon v-if="lane === 'error'" name="lucide:x" size="14" class="shrink-0 text-error *:stroke-[3]" aria-hidden="true" />
  <Icon v-else-if="!lane" name="lucide:message-square" size="12" class="shrink-0 text-base-content/70" aria-hidden="true" />
  <span v-else aria-hidden="true" class="inline-block size-2.5 shrink-0" :class="[shape[lane], laneColor[lane]]" />
</template>
