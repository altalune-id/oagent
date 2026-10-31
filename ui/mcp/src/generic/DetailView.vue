<script setup lang="ts">
import type { DetailDescriptor } from './descriptor';
import ViewShell from '../components/ViewShell.vue';
import { t } from '../copy';
import { objectAt } from './descriptor';
import FieldList from './FieldList.vue';

const { data, descriptor: d } = defineProps<{ data: Record<string, unknown>, descriptor: DetailDescriptor }>();

const object = computed(() => objectAt(data, d.object));
const note = computed(() => (object.value && d.note ? d.note(object.value) : ''));
</script>

<template>
  <ViewShell :title="d.title">
    <p v-if="!object" class="text-sm text-base-content/60">
      {{ t('mcp.nothing') }}
    </p>
    <template v-else>
      <div v-if="d.kind === 'result'" role="status" class="alert alert-soft alert-success py-2 text-sm">
        <Icon name="lucide:circle-check" size="16" />
        {{ t('common.done') }}
      </div>
      <FieldList :fields="d.fields" :object="object" />
      <p v-if="note" role="note" class="text-sm text-base-content/70">
        {{ note }}
      </p>
    </template>
  </ViewShell>
</template>
