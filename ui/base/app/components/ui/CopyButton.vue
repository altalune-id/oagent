<script setup lang="ts">
import { useCopy } from './copy';

const { value, label = 'Copy', text = '', copiedLabel = '' } = defineProps<{ value: string, label?: string, text?: string, copiedLabel?: string }>();
const { copied, copy: write } = useCopy();
const copy = () => write(value, copiedLabel);
</script>

<template>
  <button v-if="text" type="button" class="btn btn-sm text-ui" @click="copy">
    <Icon :name="copied ? 'lucide:check' : 'lucide:copy'" size="16" class="shrink-0" :class="copied ? 'text-success' : ''" aria-hidden="true" />
    {{ text }}
  </button>
  <button v-else type="button" class="btn btn-ghost btn-xs btn-square" :aria-label="label" :title="label" @click="copy">
    <Icon :name="copied ? 'lucide:check' : 'lucide:copy'" size="14" :class="copied ? 'text-success' : ''" />
  </button>
</template>
