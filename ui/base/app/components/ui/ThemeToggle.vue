<script setup lang="ts">
import AppButton from './AppButton.vue';

withDefaults(defineProps<{
  mode: 'light' | 'dark',
  lightLabel?: string,
  darkLabel?: string,
}>(), {
  lightLabel: 'Light',
  darkLabel: 'Dark',
});

const emit = defineEmits<{ toggle: [] }>();
</script>

<template>
  <AppButton variant="ghost" size="sm" @click="emit('toggle')">
    <Icon :name="mode === 'light' ? 'lucide:moon' : 'lucide:sun'" size="16" />
    <span class="ml-1 hidden sm:inline">{{ mode === 'light' ? darkLabel : lightLabel }}</span>
  </AppButton>
</template>
