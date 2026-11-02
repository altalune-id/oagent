<script setup lang="ts">
import ThemeSwitcher from '#base/components/layout/ThemeSwitcher.vue';
import CommandPalette from '#base/components/ui/CommandPalette.vue';
import TopBar from '#base/components/ui/TopBar.vue';
import ThemeFamilySwitcher from '~/components/layout/ThemeFamilySwitcher.vue';
import { useNavCrumbs } from '~/components/layout/useNavCrumbs';
import { useUIStore } from '~/stores/ui';

const { t } = useI18n();
const ui = useUIStore();
const { crumbs, palette } = useNavCrumbs();
const searching = ref(false);

function onKey(event: KeyboardEvent) {
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
    event.preventDefault();
    searching.value = !searching.value;
  }
}

onMounted(() => document.addEventListener('keydown', onKey));
onBeforeUnmount(() => document.removeEventListener('keydown', onKey));
</script>

<template>
  <TopBar
    :crumbs="crumbs"
    :crumbs-label="t('nav.breadcrumbs')"
    :menu-label="t('nav.menu')"
    :search-label="t('search.label')"
    :jump-label="t('search.jump')"
    :shortcut-label="t('search.shortcut')"
    @menu="ui.navOpen = !ui.navOpen"
    @search="searching = true"
  >
    <template #controls>
      <ThemeFamilySwitcher />
      <ThemeSwitcher />
    </template>
  </TopBar>
  <CommandPalette v-model:open="searching" :items="palette" :label="t('search.jump')" :empty="t('search.empty')" :close-label="t('common.close')" />
</template>
