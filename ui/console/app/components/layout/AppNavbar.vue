<script setup lang="ts">
import LanguageSwitcher from '#base/components/layout/LanguageSwitcher.vue';
import ThemeSwitcher from '#base/components/layout/ThemeSwitcher.vue';
import NavbarShell from '#base/components/ui/NavbarShell.vue';
import ThemeFamilySwitcher from '~/components/layout/ThemeFamilySwitcher.vue';
import UserMenu from '~/components/layout/UserMenu.vue';
import { useUserMenu } from '~/components/layout/useUserMenu';
import { staticURL } from '~/lib/auth/urls';
import { useAuthStore } from '~/stores/auth';
import { useUIStore } from '~/stores/ui';

const route = useRoute();
const { t } = useI18n();
const ui = useUIStore();

const hasNav = computed(() => !!route.params.org);

const auth = useAuthStore();
const { apiUrl } = useRuntimeConfig().public;
const logoUrl = staticURL(apiUrl, 'altalune-logo.png');
const { me, logoutUrl, labels } = useUserMenu();
onMounted(() => auth.load());
</script>

<template>
  <NavbarShell>
    <template #brand>
      <button
        v-if="hasNav"
        type="button"
        class="btn btn-ghost btn-sm btn-square md:hidden"
        :aria-label="t('nav.menu')"
        aria-controls="app-sidebar"
        :aria-expanded="ui.navOpen"
        @click="ui.navOpen = !ui.navOpen"
      >
        <Icon name="lucide:menu" size="18" />
      </button>
      <NuxtLink to="/" class="btn btn-ghost gap-2 px-2 text-base font-bold tracking-tight" :class="{ 'max-sm:hidden': hasNav }">
        <img :src="logoUrl" alt="" width="28" height="28" class="h-7 w-7 rounded-md object-contain">
        OAgent
      </NuxtLink>
    </template>
    <template #end>
      <LanguageSwitcher />
      <ThemeFamilySwitcher />
      <ThemeSwitcher />
      <UserMenu v-if="!hasNav" :me="me" :logout-url="logoutUrl" :labels="labels" />
    </template>
  </NavbarShell>
</template>
