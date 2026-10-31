<script setup lang="ts">
const { open = false, title = '' } = defineProps<{
  open?: boolean,
  title?: string,
}>();

const emit = defineEmits<{ close: [] }>();
</script>

<template>
  <dialog class="modal" :class="{ 'modal-open': open }" @cancel.prevent="emit('close')">
    <div class="modal-box">
      <h3 v-if="title" class="text-lg font-bold">
        {{ title }}
      </h3>
      <div class="py-4">
        <slot />
      </div>
      <div class="modal-action">
        <slot name="footer" />
      </div>
    </div>
    <form method="dialog" class="modal-backdrop">
      <button type="submit" @click="emit('close')">
        close
      </button>
    </form>
  </dialog>
</template>
