<script setup lang="ts">
import LanguageSwitcher from '~/components/layout/LanguageSwitcher.vue';
import ThemeFamilySwitcher from '~/components/layout/ThemeFamilySwitcher.vue';
import ThemeSwitcher from '~/components/layout/ThemeSwitcher.vue';
import UserMenu from '~/components/layout/UserMenu.vue';
import OrgSelector from '~/components/org/OrgSelector.vue';
import ProjectSelector from '~/components/project/ProjectSelector.vue';
import NavbarShell from '~/components/ui/NavbarShell.vue';
import { siblingURL, staticURL } from '~/lib/auth/urls';
import { useAuthStore } from '~/stores/auth';
import { useUIStore } from '~/stores/ui';

const route = useRoute();
const { t } = useI18n();
const ui = useUIStore();

const hasNav = computed(() => !!route.params.org);

const auth = useAuthStore();
const { apiUrl } = useRuntimeConfig().public;
const logoutUrl = siblingURL(apiUrl, '/logout');
const logoUrl = staticURL(apiUrl, 'altalune-logo.png');
const userLabels = computed(() => ({
  signOut: t('auth.sign_out'),
  signedInAs: t('auth.signed_in_as'),
  operator: t('auth.operator'),
  apikey: t('auth.source_apikey'),
  basic: t('auth.source_basic'),
  profile: t('profile.title'),
}));
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
        @click="ui.navOpen = !ui.navOpen"
      >
        <Icon name="lucide:menu" size="18" />
      </button>
      <NuxtLink to="/" class="btn btn-ghost gap-2 px-2 text-base font-bold tracking-tight" :class="{ 'max-sm:hidden': hasNav }">
        <img :src="logoUrl" alt="" width="28" height="28" class="h-7 w-7 rounded-md">
        oagent
      </NuxtLink>
    </template>
    <OrgSelector @focusin="ui.navOpen = false" />
    <ProjectSelector @focusin="ui.navOpen = false" />
    <template #end>
      <LanguageSwitcher @focusin="ui.navOpen = false" />
      <ThemeFamilySwitcher @focusin="ui.navOpen = false" />
      <ThemeSwitcher @focusin="ui.navOpen = false" />
      <UserMenu :me="auth.me" :logout-url="logoutUrl" :labels="userLabels" @focusin="ui.navOpen = false" />
    </template>
  </NavbarShell>
</template>
