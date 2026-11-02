<script setup lang="ts">
import UserMenu from '~/components/layout/UserMenu.vue';
import { useUserMenu } from '~/components/layout/useUserMenu';

const route = useRoute();
const { me, logoutUrl, labels, version } = useUserMenu();
const apiKeysUrl = computed(() => `/o/${route.params.org}/settings/personal-keys`);
</script>

<template>
  <UserMenu :me="me" :logout-url="logoutUrl" :labels="labels" :version="version" :api-keys-url="apiKeysUrl" placement="top" />
</template>
