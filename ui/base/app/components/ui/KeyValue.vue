<script setup lang="ts">
import type { KeyValueItem } from './types';
import CopyButton from './CopyButton.vue';

const { items, copyLabel = 'Copy {label}' } = defineProps<{
  items: KeyValueItem[],
  copyLabel?: string,
}>();
</script>

<template>
  <dl class="divide-y divide-base-300">
    <div v-for="(item, i) in items" :key="item.label" class="grid grid-cols-[9.5rem_minmax(0,1fr)] gap-3 px-4 py-2 max-sm:grid-cols-1 max-sm:gap-0">
      <dt class="text-ui text-base-content/70">
        {{ item.label }}
      </dt>
      <dd class="flex min-w-0 flex-wrap items-center gap-x-2 text-ui">
        <slot :name="`value-${i}`" :item="item">
          <span :class="{ 'font-mono text-xs break-all': item.mono }">{{ item.value }}</span>
          <CopyButton v-if="item.copy && item.value" :value="item.value" :label="copyLabel.replace('{label}', item.label)" />
        </slot>
      </dd>
    </div>
  </dl>
</template>
