<template>
  <header class="navbar sticky top-0 z-40 min-h-14 gap-2 border-b border-base-300 bg-base-100/90 px-4 backdrop-blur">
    <div class="flex items-center gap-2">
      <NuxtLink to="/" class="btn btn-ghost px-2 text-base font-bold tracking-tight">oagent</NuxtLink>
      <OrgSelector />
      <ProjectSelector />
    </div>
    <div class="ml-auto flex items-center gap-1">
      <ThemeSwitcher />
    </div>
  </header>
</template>
