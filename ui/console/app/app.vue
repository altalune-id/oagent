<script setup lang="ts">
import { useTheme } from '~/components/layout/useTheme';

const { init } = useTheme();

onMounted(init);
</script>

<template>
  <NuxtLayout>
    <NuxtPage />
  </NuxtLayout>
</template>
