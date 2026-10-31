<script setup lang="ts">
import ErrorText from '#base/components/ui/ErrorText.vue';
import { useLanding } from '~/components/layout/useLanding';
import { useOrgsStore } from '~/stores/orgs';

const orgs = useOrgsStore();
const { resolve } = useLanding();

await orgs.ensureLoaded();

if (!orgs.error) {
  await navigateTo(await resolve(), { replace: true });
}
</script>

<template>
  <div class="mx-auto w-full max-w-3xl px-4 py-8 sm:px-6 sm:py-10">
    <ErrorText v-if="orgs.error" :error="orgs.error" />
  </div>
</template>
