<script setup lang="ts">
import ThemeSwitcher from '#base/components/layout/ThemeSwitcher.vue';
import ThemeFamilySwitcher from '~/components/layout/ThemeFamilySwitcher.vue';
import UserMenu from '~/components/layout/UserMenu.vue';
import { useUserMenu } from '~/components/layout/useUserMenu';

const route = useRoute();
const { me, logoutUrl, labels, version, accountUrl, locale, languages, setLocale } = useUserMenu();
const apiKeysUrl = computed(() => `/o/${route.params.org}/settings/personal-keys`);
</script>

<template>
  <div class="px-1 md:hidden">
    <div class="flex items-center gap-1">
      <ThemeFamilySwitcher />
      <ThemeSwitcher />
    </div>
  </div>
  <UserMenu
    :me="me" :logout-url="logoutUrl" :labels="labels" :version="version" :api-keys-url="apiKeysUrl" :account-url="accountUrl"
    :locale="locale" :locales="languages" placement="top" @locale="setLocale"
  />
</template>
