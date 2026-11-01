<script setup lang="ts">
import type { KeyValueItem } from './types';
import CopyButton from './CopyButton.vue';

const { items, copyLabel = 'Copy {label}' } = defineProps<{
  items: KeyValueItem[],
  copyLabel?: string,
}>();
</script>

<template>
  <dl class="grid grid-cols-[minmax(0,1fr)] gap-x-6 gap-y-2 text-sm sm:grid-cols-[minmax(9rem,auto)_minmax(0,1fr)]">
    <template v-for="(item, i) in items" :key="item.label">
      <dt class="text-base-content/70">
        {{ item.label }}
      </dt>
      <dd class="flex min-w-0 items-center gap-1 break-words max-sm:mb-2">
        <slot :name="`value-${i}`" :item="item">
          <span :class="{ 'font-mono text-xs break-all': item.mono }">{{ item.value }}</span>
          <CopyButton v-if="item.copy && item.value" :value="item.value" :label="copyLabel.replace('{label}', item.label)" />
        </slot>
      </dd>
    </template>
  </dl>
</template>
