<script setup lang="ts">
const route = useRoute()
const org = computed(() => route.params.org as string)

const items = computed(() => [
  { label: 'Overview', to: `/o/${org.value}` },
  { label: 'Projects', to: `/o/${org.value}/projects` },
  { label: 'Settings', to: `/o/${org.value}/settings` },
])

function isActive(to: string) {
  return to === `/o/${org.value}` ? route.path === to : route.path.startsWith(to)
}
</script>

<template>
  <aside class="hidden w-60 shrink-0 flex-col border-r border-base-300 bg-base-200/40 md:flex">
    <div class="px-4 pt-5 pb-2 text-xs font-semibold tracking-widest text-base-content/50 uppercase">
      {{ org }}
    </div>
    <ul class="menu w-full gap-0.5 px-2">
      <li v-for="item in items" :key="item.to">
        <NuxtLink :to="item.to" :class="{ 'menu-active': isActive(item.to) }">
          {{ item.label }}
        </NuxtLink>
      </li>
    </ul>
  </aside>
</template>
