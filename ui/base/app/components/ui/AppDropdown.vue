<script setup lang="ts">
const { label = '', open = false, disabled = false, triggerClass = '' } = defineProps<{
  label?: string,
  open?: boolean,
  disabled?: boolean,
  triggerClass?: string,
}>();
</script>

<template>
  <div class="dropdown" :class="{ 'dropdown-open': open }">
    <div
      :tabindex="disabled ? -1 : 0"
      role="button"
      aria-haspopup="true"
      class="btn btn-ghost btn-sm"
      :class="[triggerClass, { 'btn-disabled': disabled }]"
      :aria-disabled="disabled || undefined"
    >
      <span v-if="label" class="sr-only">{{ label }}</span>
      <slot name="trigger" />
    </div>
    <slot />
  </div>
</template>
