<script setup lang="ts">
import { useTheme } from '~/components/layout/useTheme';
import { staticURL } from '~/lib/auth/urls';

const { init } = useTheme();
const { apiUrl } = useRuntimeConfig().public;

useHead({
  link: [
    { key: 'icon', rel: 'icon', href: staticURL(apiUrl, 'favicon.ico'), sizes: 'any' },
    { rel: 'icon', type: 'image/png', sizes: '32x32', href: staticURL(apiUrl, 'favicon-32x32.png') },
    { rel: 'apple-touch-icon', href: staticURL(apiUrl, 'apple-touch-icon.png') },
  ],
});

onMounted(init);
</script>

<template>
  <NuxtLayout>
    <NuxtPage />
  </NuxtLayout>
</template>
