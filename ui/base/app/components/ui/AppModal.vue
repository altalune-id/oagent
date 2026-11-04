<script setup lang="ts">
import { onMounted, ref, useId, watch } from 'vue';

const { open = false, title = '', dismissible = true, wide = false, stickyFooter = false } = defineProps<{
  open?: boolean,
  title?: string,
  dismissible?: boolean,
  wide?: boolean,
  stickyFooter?: boolean,
}>();

const emit = defineEmits<{ close: [] }>();

const dialog = ref<HTMLDialogElement>();
const titleId = useId();

function sync() {
  const el = dialog.value;
  if (!el) {
    return;
  }
  if (open && !el.open) {
    el.showModal();
  } else if (!open && el.open) {
    el.close();
  }
}

function cancel() {
  if (dismissible) {
    emit('close');
  }
}

watch(() => open, sync, { flush: 'post' });
onMounted(sync);
</script>

<template>
  <dialog ref="dialog" class="modal" :aria-labelledby="title ? titleId : undefined" @cancel.prevent="cancel()" @close="sync()">
    <div class="modal-box" :class="{ 'w-11/12 max-w-3xl': wide }">
      <h2 v-if="title" :id="titleId" class="text-base font-semibold">
        {{ title }}
      </h2>
      <div class="py-4">
        <slot />
      </div>
      <div class="modal-action" :class="{ 'sticky bottom-0 -mb-6 bg-base-100 pt-2 pb-6': stickyFooter }">
        <slot name="footer" />
      </div>
    </div>
    <form v-if="dismissible" method="dialog" class="modal-backdrop" @submit.prevent="emit('close')">
      <button type="submit">
        close
      </button>
    </form>
  </dialog>
</template>
