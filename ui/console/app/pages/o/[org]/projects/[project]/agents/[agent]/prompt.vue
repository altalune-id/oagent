<script setup lang="ts">
const { t } = useI18n();
const route = useRoute();
useHead({ title: () => `${route.params.agent} · ${t('nav.prompt')} · oagent` });
</script>

<template>
  <div class="space-y-6">
    <h1 class="text-2xl font-bold">
      {{ t('nav.prompt') }}
    </h1>
    <div class="rounded-box border border-dashed border-base-300 bg-base-100 p-6">
      <p class="text-sm text-base-content/50">
        {{ t('agent.placeholder') }}
      </p>
    </div>
  </div>
</template>
