<script setup lang="ts">
import { useLanding } from '~/components/layout/useLanding';
import AppButton from '~/components/ui/AppButton.vue';
import ErrorText from '~/components/ui/ErrorText.vue';
import { useOrgsStore } from '~/stores/orgs';
import { useUIStore } from '~/stores/ui';

const { t } = useI18n();
const orgs = useOrgsStore();
const ui = useUIStore();
const { resolve } = useLanding();

await orgs.ensureLoaded();

if (orgs.items.length) {
  await navigateTo(await resolve(), { replace: true });
}
</script>

<template>
  <div class="mx-auto w-full max-w-3xl px-4 py-8 sm:px-6 sm:py-10">
    <div class="flex items-start justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold">
          {{ t('org.title') }}
        </h1>
        <p class="mt-1 text-sm text-base-content/60">
          {{ t('org.subtitle') }}
        </p>
      </div>
      <AppButton size="sm" @click="ui.openOrgCreate()">
        <Icon name="lucide:plus" size="16" />
        {{ t('org.new') }}
      </AppButton>
    </div>

    <ErrorText v-if="orgs.error" :error="orgs.error" class="mt-6" />
    <p v-else class="mt-6 text-sm text-base-content/60">
      {{ t('org.empty') }}
    </p>
  </div>
</template>
