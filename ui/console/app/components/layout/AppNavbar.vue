<script setup lang="ts">
import LanguageSwitcher from '~/components/layout/LanguageSwitcher.vue'
import NavbarShell from '~/components/ui/NavbarShell.vue'
import OrgSelector from '~/components/org/OrgSelector.vue'
import ProjectSelector from '~/components/project/ProjectSelector.vue'
import ThemeFamilySwitcher from '~/components/layout/ThemeFamilySwitcher.vue'
import ThemeSwitcher from '~/components/layout/ThemeSwitcher.vue'
import { useUIStore } from '~/stores/ui'

const route = useRoute()
const { t } = useI18n()
const ui = useUIStore()

const hasNav = computed(() => !!route.params.org)
</script>

<template>
  <NavbarShell>
    <template #brand>
      <button
        v-if="hasNav"
        type="button"
        class="btn btn-ghost btn-sm btn-square md:hidden"
        :aria-label="t('nav.menu')"
        @click="ui.navOpen = !ui.navOpen"
      >
        <Icon name="lucide:menu" size="18" />
      </button>
      <NuxtLink to="/" class="btn btn-ghost px-2 text-base font-bold tracking-tight" :class="{ 'max-sm:hidden': hasNav }">oagent</NuxtLink>
    </template>
    <OrgSelector @focusin="ui.navOpen = false" />
    <ProjectSelector @focusin="ui.navOpen = false" />
    <template #end>
      <LanguageSwitcher @focusin="ui.navOpen = false" />
      <ThemeFamilySwitcher @focusin="ui.navOpen = false" />
      <ThemeSwitcher @focusin="ui.navOpen = false" />
    </template>
  </NavbarShell>
</template>
