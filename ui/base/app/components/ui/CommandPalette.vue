<script setup lang="ts">
import type { PaletteItem } from './types';
import { computed, nextTick, ref, watch } from 'vue';
import SearchInput from './SearchInput.vue';

const { items, label, empty = '', closeLabel = 'Close' } = defineProps<{
  items: PaletteItem[],
  label: string,
  empty?: string,
  closeLabel?: string,
}>();

const open = defineModel<boolean>('open', { default: false });

const dialog = ref<HTMLDialogElement>();
const list = ref<HTMLElement>();
const query = ref('');
const shown = computed(() => {
  const q = query.value.trim().toLowerCase();
  return q ? items.filter(item => item.label.toLowerCase().includes(q)) : items;
});

watch(open, async (now) => {
  const el = dialog.value;
  if (!el) {
    return;
  }
  if (now && !el.open) {
    query.value = '';
    el.showModal();
    await nextTick();
    el.querySelector<HTMLInputElement>('input[type="search"]')?.focus();
  } else if (!now && el.open) {
    el.close();
  }
}, { flush: 'post' });

function first() {
  list.value?.querySelector<HTMLAnchorElement>('a')?.click();
}
</script>

<template>
  <dialog ref="dialog" class="modal modal-top sm:modal-middle" :aria-label="label" @close="open = false">
    <div class="modal-box p-0 sm:max-w-lg">
      <form @submit.prevent="first">
        <SearchInput v-model="query" :label="label" class="m-2 w-[calc(100%-1rem)]" />
      </form>
      <ul ref="list" class="menu menu-sm w-full text-ui">
        <li v-for="item in shown" :key="item.key">
          <NuxtLink :to="item.to" @click="open = false">
            {{ item.label }}
          </NuxtLink>
        </li>
        <li v-if="!shown.length && empty" class="menu-disabled">
          <span>{{ empty }}</span>
        </li>
      </ul>
    </div>
    <form method="dialog" class="modal-backdrop">
      <button>{{ closeLabel }}</button>
    </form>
  </dialog>
</template>
