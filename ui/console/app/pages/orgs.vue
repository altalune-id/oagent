<script setup lang="ts">
import { useOrgCreation } from '~/components/org/create/useOrgCreation';
import OrgsEmptyState from '~/components/org/OrgsEmptyState.vue';
import AppButton from '~/components/ui/AppButton.vue';
import ErrorText from '~/components/ui/ErrorText.vue';
import { siblingURL } from '~/lib/auth/urls';
import { useAuthStore } from '~/stores/auth';
import { useOrgsStore } from '~/stores/orgs';
import { useUIStore } from '~/stores/ui';

const { t } = useI18n();
const orgs = useOrgsStore();
const auth = useAuthStore();
const ui = useUIStore();
const { apiUrl } = useRuntimeConfig().public;

await Promise.all([orgs.ensureLoaded(), auth.load()]);

const { allowed, ownsOne, canCreate } = useOrgCreation();
const isEmpty = computed(() => !orgs.error && orgs.items.length === 0);

if (isEmpty.value && canCreate.value) {
  await navigateTo('/onboarding', { replace: true });
}
const emptyLabels = computed(() => ({
  title: t('org.none_title'),
  body: t('org.none_body'),
  signOut: t('auth.sign_out'),
}));

useHead({ title: () => `${t('org.all')} · oagent` });
</script>

<template>
  <div class="mx-auto w-full max-w-3xl px-4 py-8 sm:px-6 sm:py-10">
    <div class="flex items-start justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold">
          {{ t('org.title') }}
        </h1>
        <p v-if="!isEmpty" class="mt-1 text-sm text-base-content/60">
          {{ t('org.subtitle') }}
        </p>
      </div>
      <p v-if="allowed && ownsOne" class="max-w-56 text-right text-xs text-base-content/60" data-testid="org-create-owned">
        {{ t('org.create_owned') }}
      </p>
      <AppButton v-else-if="canCreate && !isEmpty" size="sm" @click="ui.openOrgCreate()">
        <Icon name="lucide:plus" size="16" />
        {{ t('org.new') }}
      </AppButton>
    </div>

    <ErrorText v-if="orgs.error" :error="orgs.error" class="mt-6" />
    <OrgsEmptyState
      v-else-if="isEmpty"
      :logout-url="siblingURL(apiUrl, '/logout')"
      :labels="emptyLabels"
    />
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
