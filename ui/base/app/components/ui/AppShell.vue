<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, useId, watch } from 'vue';

const { sidebarId = 'app-sidebar', closeLabel = 'Close menu', full = false, breakpoint = 'md', rail = false } = defineProps<{
  sidebarId?: string,
  closeLabel?: string,
  full?: boolean,
  breakpoint?: 'md' | 'lg',
  rail?: boolean,
}>();

const opens = { md: 'md:drawer-open', lg: 'lg:drawer-open' } as const;
const widths = { md: '(min-width: 48rem)', lg: '(min-width: 64rem)' } as const;

const open = defineModel<boolean>('open', { default: false });

const toggleId = `drawer-${useId()}`;
const side = ref<HTMLElement>();
const wide = ref(false);
let returnTo: HTMLElement | null = null;
let media: MediaQueryList | undefined;

function onKey(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    open.value = false;
  }
}

function onMedia() {
  wide.value = !!media?.matches;
}

watch(open, (now) => {
  if (now) {
    returnTo = document.activeElement as HTMLElement | null;
    document.addEventListener('keydown', onKey);
    side.value?.querySelector<HTMLElement>('a[href], button:not([disabled]), input:not([type="hidden"]), select')?.focus();
    return;
  }
  document.removeEventListener('keydown', onKey);
  if (returnTo && side.value?.contains(document.activeElement)) {
    returnTo.focus();
  }
  returnTo = null;
}, { flush: 'post' });

onMounted(() => {
  media = window.matchMedia?.(widths[breakpoint]);
  media?.addEventListener('change', onMedia);
  onMedia();
});

onBeforeUnmount(() => {
  document.removeEventListener('keydown', onKey);
  media?.removeEventListener('change', onMedia);
});
</script>

<template>
  <div class="drawer flex-1" :class="opens[breakpoint]">
    <input :id="toggleId" v-model="open" type="checkbox" class="drawer-toggle" tabindex="-1" aria-hidden="true">
    <div class="drawer-content flex min-w-0 flex-col" :inert="open && !wide">
      <slot />
    </div>
    <div :id="sidebarId" ref="side" class="drawer-side z-40" :class="{ 'top-14 h-[calc(100dvh-3.5rem)]': !full, 'lg:overflow-visible': rail }">
      <label :for="toggleId" :aria-label="closeLabel" class="drawer-overlay" />
      <slot name="sidebar" />
    </div>
  </div>
</template>
