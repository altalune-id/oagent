<script setup lang="ts">
import { computed } from 'vue';
import { avatarView } from './avatar';

const { id, name = '', email = '', avatarUrl = '', size = 'md' } = defineProps<{
  id: string,
  name?: string,
  email?: string,
  avatarUrl?: string,
  size?: 'xs' | 'sm' | 'md' | 'lg',
}>();

const view = computed(() => avatarView({ id, name, email, avatarUrl }));
const box = computed(() => ({ xs: 'size-6 text-[10px]', sm: 'size-7 text-xs', md: 'size-10 text-sm', lg: 'size-16 text-xl' })[size]);
</script>

<template>
  <div v-if="view.kind === 'image'" class="avatar shrink-0">
    <div class="rounded-full" :class="box">
      <img :src="view.src" :alt="view.alt">
    </div>
  </div>
  <div v-else class="avatar avatar-placeholder shrink-0">
    <div role="img" :aria-label="view.label" class="rounded-full font-semibold" :class="[box, view.color]">
      <span aria-hidden="true">{{ view.text }}</span>
    </div>
  </div>
</template>
