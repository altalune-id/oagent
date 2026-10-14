<script setup lang="ts">
import AppButton from '~/components/ui/AppButton.vue';
import ErrorText from '~/components/ui/ErrorText.vue';
import { useOrgsStore } from '~/stores/orgs';
import { useUIStore } from '~/stores/ui';

const { t } = useI18n();
const orgs = useOrgsStore();
const ui = useUIStore();

await orgs.ensureLoaded();

useHead({ title: () => `${t('org.all')} · oagent` });
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
    <p v-else-if="orgs.items.length === 0" class="mt-6 text-sm text-base-content/60">
      {{ t('org.empty') }}
    </p>
    <div v-else class="mt-6 grid gap-3 sm:grid-cols-2">
      <NuxtLink
        v-for="org in orgs.items"
        :key="org.slug"
        :to="`/o/${org.slug}`"
        class="card border border-base-300 bg-base-100 transition-colors hover:border-primary"
      >
        <div class="card-body">
          <h2 class="card-title text-base">
            {{ org.name }}
          </h2>
          <p class="text-sm text-base-content/60">
            /{{ org.slug }}
          </p>
        </div>
      </NuxtLink>
    </div>
  </div>
</template>
