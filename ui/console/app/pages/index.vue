<script setup lang="ts">
import LoadError from '#base/components/ui/LoadError.vue';
import { useLanding } from '~/components/layout/useLanding';
import { useOrgsStore } from '~/stores/orgs';

const { t } = useI18n();
const orgs = useOrgsStore();
const { resolve } = useLanding();

await orgs.ensureLoaded();

async function land() {
  if (!orgs.error) {
    await navigateTo(await resolve(), { replace: true });
  }
}

async function retry() {
  await orgs.load();
  await land();
}

await land();
</script>

<template>
  <div class="mx-auto w-full max-w-3xl px-4 py-8 sm:px-6 sm:py-10">
    <LoadError v-if="orgs.error" :error="orgs.error" :busy="orgs.loading" :retry-label="t('common.retry')" @retry="retry" />
  </div>
</template>
