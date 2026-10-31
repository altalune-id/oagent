<script setup lang="ts">
import ThemeToggle from '../ui/ThemeToggle.vue';
import { useTheme } from './useTheme';

const { t } = useI18n();
const { mode, supportsMode, toggle } = useTheme();
</script>

<template>
  <ThemeToggle
    v-if="supportsMode"
    :mode="mode"
    :light-label="t('theme.light')"
    :dark-label="t('theme.dark')"
    @toggle="toggle"
  />
</template>
