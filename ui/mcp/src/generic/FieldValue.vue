<script setup lang="ts">
import type { BadgeTone, Field } from './descriptor';
import CopyButton from '#base/components/ui/CopyButton.vue';
import SecretField from '#base/components/ui/SecretField.vue';
import { t } from '../copy';
import { badgeFor, formatValue, isEmpty } from './descriptor';

const { field, value } = defineProps<{ field: Field, value: unknown }>();

const tones: Record<BadgeTone, string> = {
  success: 'badge-soft badge-success',
  warning: 'badge-soft badge-warning',
  error: 'badge-soft badge-error',
  info: 'badge-soft badge-info',
  neutral: 'badge-outline',
  ghost: 'badge-ghost',
};

const text = computed(() => formatValue(field, value));
const badge = computed(() => badgeFor(field, value));
const present = computed(() => !isEmpty(value));
const copyLabel = computed(() => t('common.copy_label', { label: field.label }));
</script>

<template>
  <template v-if="field.kind === 'badge'">
    <span v-if="badge.label" class="badge badge-sm" :class="tones[badge.tone]">{{ badge.label }}</span>
  </template>
  <ul v-else-if="field.kind === 'list' && Array.isArray(value) && value.length" class="space-y-0.5">
    <li v-for="item in value" :key="String(item)">
      <code class="font-mono text-xs break-all">{{ item }}</code>
    </li>
  </ul>
  <span v-else-if="(field.kind === 'id' || field.kind === 'url') && present" class="inline-flex max-w-full items-center gap-1">
    <code class="min-w-0 font-mono text-xs break-all">{{ text }}</code>
    <CopyButton :value="String(value)" :label="copyLabel" class="shrink-0" />
  </span>
  <SecretField v-else-if="field.kind === 'secret' && present" :value="String(value)" :label="copyLabel" />
  <code v-else-if="field.kind === 'mono'" class="font-mono text-xs break-all">{{ text }}</code>
  <span v-else>{{ text }}</span>
</template>
