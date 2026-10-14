<script setup lang="ts">
import type { LanguageOption } from '~/components/ui/types';
import LanguageSelect from '~/components/ui/LanguageSelect.vue';

const { locale, locales, setLocale, t } = useI18n();

function flag(code: string) {
  const [language, region] = code.split('-');
  return `circle-flags:${(region ?? language ?? '').toLowerCase()}`;
}

const options = computed<LanguageOption[]>(() =>
  locales.value.map(l => (typeof l === 'string'
    ? { code: l, name: l, icon: flag(l) }
    : { code: l.code, name: l.name ?? l.code, icon: flag(l.code) })),
);

async function select(code: string) {
  await setLocale(code as typeof locale.value);
  if (import.meta.client) {
    (document.activeElement as HTMLElement | null)?.blur();
  }
}
</script>

<template>
  <LanguageSelect :model-value="locale" :options="options" :label="t('language.label')" @select="select" />
</template>
