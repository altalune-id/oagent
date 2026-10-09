<script setup lang="ts">
import LanguageSwitcher from '~/components/layout/LanguageSwitcher.vue'
import NavbarShell from '~/components/ui/NavbarShell.vue'
import OrgSelector from '~/components/org/OrgSelector.vue'
import ProjectSelector from '~/components/project/ProjectSelector.vue'
import ThemeFamilySwitcher from '~/components/layout/ThemeFamilySwitcher.vue'
import ThemeSwitcher from '~/components/layout/ThemeSwitcher.vue'
</script>

<template>
  <NavbarShell>
    <template #brand>
      <NuxtLink to="/" class="btn btn-ghost px-2 text-base font-bold tracking-tight">oagent</NuxtLink>
    </template>
    <OrgSelector />
    <ProjectSelector />
    <template #end>
      <LanguageSwitcher />
      <ThemeFamilySwitcher />
      <ThemeSwitcher />
    </template>
  </NavbarShell>
</template>
