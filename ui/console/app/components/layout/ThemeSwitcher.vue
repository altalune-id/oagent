<script setup lang="ts">
import { useTheme } from '~/components/layout/useTheme';
import ThemeToggle from '~/components/ui/ThemeToggle.vue';

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
