<script setup lang="ts">
import AppNavbar from '~/components/layout/AppNavbar.vue'
</script>

<template>
  <div class="flex min-h-dvh flex-col bg-base-100 text-base-content">
    <AppNavbar />
    <slot />
  </div>
</template>
