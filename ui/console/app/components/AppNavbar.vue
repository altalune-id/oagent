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
