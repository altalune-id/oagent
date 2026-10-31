<script setup lang="ts">
import type { Field } from './descriptor';
import { isEmpty, valueAt } from './descriptor';
import FieldValue from './FieldValue.vue';

const { fields, object } = defineProps<{ fields: Field[], object: Record<string, unknown> }>();
const shown = computed(() => fields.filter(f => !(f.hideEmpty && isEmpty(valueAt(object, f.key)))));
</script>

<template>
  <dl class="grid grid-cols-[auto_1fr] items-baseline gap-x-4 gap-y-2 text-sm">
    <template v-for="f in shown" :key="f.key">
      <dt class="text-base-content/60">
        {{ f.label }}
      </dt>
      <dd class="min-w-0">
        <FieldValue :field="f" :value="valueAt(object, f.key)" />
      </dd>
    </template>
  </dl>
</template>
