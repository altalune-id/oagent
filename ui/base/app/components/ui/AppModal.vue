<script setup lang="ts">
import { onMounted, ref, useId, watch } from 'vue';

const { open = false, title = '', dismissible = true } = defineProps<{
  open?: boolean,
  title?: string,
  dismissible?: boolean,
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
    <div class="modal-box">
      <h3 v-if="title" :id="titleId" class="text-lg font-bold">
        {{ title }}
      </h3>
      <div class="py-4">
        <slot />
      </div>
      <div class="modal-action">
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
