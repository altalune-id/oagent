<script setup lang="ts">
import { computed } from 'vue';
import { avatarView } from './avatar';

const { id, name = '', email = '', avatarUrl = '', size = 'md' } = defineProps<{
  id: string,
  name?: string,
  email?: string,
  avatarUrl?: string,
  size?: 'sm' | 'md' | 'lg',
}>();

const view = computed(() => avatarView({ id, name, email, avatarUrl }));
const box = computed(() => ({ sm: 'size-7 text-xs', md: 'size-10 text-sm', lg: 'size-16 text-xl' })[size]);
</script>

<template>
  <img v-if="view.kind === 'image'" :src="view.src" :alt="view.alt" class="shrink-0 rounded-full object-cover" :class="box">
  <span
    v-else
    role="img"
    :aria-label="view.label"
    class="grid shrink-0 place-items-center rounded-full font-semibold"
    :class="[box, view.color]"
  >{{ view.text }}</span>
</template>
