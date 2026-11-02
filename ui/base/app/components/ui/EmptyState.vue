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
  <div class="flex max-w-md flex-col items-start gap-3 py-16">
    <Icon :name="icon" size="24" class="shrink-0" aria-hidden="true" />
    <component :is="`h${headingLevel}`" class="text-xl font-semibold">
      {{ title }}
    </component>
    <p v-if="body" class="text-base-content/70">
      {{ body }}
    </p>
    <div v-if="slots.actions" class="flex flex-wrap gap-2">
      <slot name="actions" />
    </div>
  </div>
</template>
