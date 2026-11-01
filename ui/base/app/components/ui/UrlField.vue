<script setup lang="ts">
import { computed, useId } from 'vue';
import CopyButton from './CopyButton.vue';
import { urlBreaks } from './urlBreaks';

const {
  label,
  value,
  open = false,
  help = '',
  size = 'md',
  copyLabel = 'Copy {label}',
  openLabel = 'Open {label} in a new tab',
  testid,
} = defineProps<{
  label: string,
  value: string,
  open?: boolean,
  help?: string,
  size?: 'md' | 'sm',
  copyLabel?: string,
  openLabel?: string,
  testid?: string,
}>();

const id = useId();
const ids = { label: `${id}-label`, help: `${id}-help` };
const parts = computed(() => urlBreaks(value));
</script>

<template>
  <div class="space-y-1">
    <span :id="ids.label" :class="size === 'sm' ? 'block text-xs font-medium text-base-content/70' : 'fieldset-legend'">{{ label }}</span>
    <div class="flex items-center gap-1 rounded-box border border-base-300 bg-base-200 py-1.5 pr-1.5 pl-3">
      <code class="min-w-0 flex-1 font-mono text-sm break-words" :data-testid="testid" :aria-labelledby="ids.label" :aria-describedby="help ? ids.help : undefined"><template v-for="(part, i) in parts" :key="i"><wbr v-if="i > 0">{{ part }}</template></code>
      <CopyButton :value="value" :label="copyLabel.replace('{label}', label)" class="shrink-0" />
      <a v-if="open" :href="value" target="_blank" rel="noopener noreferrer" class="btn btn-ghost btn-xs btn-square shrink-0" :aria-label="openLabel.replace('{label}', label)" :title="openLabel.replace('{label}', label)">
        <Icon name="lucide:external-link" size="14" />
      </a>
    </div>
    <p v-if="help" :id="ids.help" class="label whitespace-normal">
      {{ help }}
    </p>
  </div>
</template>
