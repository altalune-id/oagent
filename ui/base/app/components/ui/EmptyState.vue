<script setup lang="ts">
import { useSlots } from 'vue';

const { title, body = '', icon = 'lucide:inbox', headingLevel = 2 } = defineProps<{
  title: string,
  body?: string,
  icon?: string,
  headingLevel?: 1 | 2 | 3,
}>();

const slots = useSlots();
</script>

<template>
  <div class="flex flex-col items-center gap-3 rounded-box border border-dashed border-base-300 p-8 text-center">
    <Icon :name="icon" size="32" class="text-base-content/70" />
    <component :is="`h${headingLevel}`" class="text-lg font-semibold">
      {{ title }}
    </component>
    <p v-if="body" class="max-w-prose text-sm text-base-content/70">
      {{ body }}
    </p>
    <div v-if="slots.actions" class="flex flex-wrap justify-center gap-2">
      <slot name="actions" />
    </div>
  </div>
</template>
