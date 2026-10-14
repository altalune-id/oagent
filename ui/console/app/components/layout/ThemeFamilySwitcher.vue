<script setup lang="ts">
import type { ThemeFamilyOption } from '~/components/ui/types';
import { useTheme } from '~/components/layout/useTheme';
import ThemeSelect from '~/components/ui/ThemeSelect.vue';

const { t } = useI18n();
const { family, families, setFamily } = useTheme();

const options = computed<ThemeFamilyOption[]>(() =>
  families.map(f => ({ id: f.id, label: f.label, theme: f.themes[f.default] ?? '' })),
);
</script>

<template>
  <ThemeSelect :label="t('theme.label')" :model-value="family.id" :families="options" @select="setFamily" />
</template>
