<script setup lang="ts">
import NavbarShell from '~/components/ui/NavbarShell.vue'
import OrgSelector from '~/components/org/OrgSelector.vue'
import ProjectSelector from '~/components/project/ProjectSelector.vue'
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
      <ThemeSwitcher />
    </template>
  </NavbarShell>
</template>
