<script setup lang="ts">
import UserMenu from '~/components/layout/UserMenu.vue';
import { useUserMenu } from '~/components/layout/useUserMenu';

const { me, logoutUrl, labels, version } = useUserMenu();
</script>

<template>
  <UserMenu :me="me" :logout-url="logoutUrl" :labels="labels" :version="version" placement="top" />
</template>
