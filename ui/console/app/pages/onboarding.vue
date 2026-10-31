<script setup lang="ts">
import AppButton from '#base/components/ui/AppButton.vue';
import ErrorText from '#base/components/ui/ErrorText.vue';
import { useLanding } from '~/components/layout/useLanding';
import OrgFields from '~/components/org/create/OrgFields.vue';
import { useOnboarding } from '~/components/org/onboarding/useOnboarding';
import { useAuthStore } from '~/stores/auth';
import { useOrgsStore } from '~/stores/orgs';

const { t } = useI18n();
const orgs = useOrgsStore();
const auth = useAuthStore();
const { resolve } = useLanding();
const { name, slug, busy, error, fieldErrors, submit } = useOnboarding();

await Promise.all([orgs.ensureLoaded(), auth.load()]);

if (!orgs.error && (orgs.items.length > 0 || auth.me?.orgCreation !== true)) {
  await navigateTo(await resolve(), { replace: true });
}

useHead({ title: () => `${t('onboarding.title')} · oagent` });
</script>

<template>
  <div class="mx-auto w-full max-w-lg px-4 py-8 sm:px-6 sm:py-16">
    <ErrorText v-if="orgs.error" :error="orgs.error" />
    <div v-else class="card border border-base-300 bg-base-100">
      <form novalidate class="card-body gap-4" @submit.prevent="submit()">
        <div class="flex items-center gap-3">
          <Icon name="lucide:building-2" size="28" class="text-primary" />
          <h1 class="card-title text-xl">
            {{ t('onboarding.title') }}
          </h1>
        </div>
        <p class="text-sm text-base-content/70">
          {{ t('onboarding.body') }}
        </p>
        <OrgFields v-model:name="name" v-model:slug="slug" :field-errors="fieldErrors" :busy="busy" />
        <p class="text-sm text-base-content/60">
          {{ t('onboarding.project_note') }}
        </p>
        <ErrorText :error="error" />
        <AppButton type="submit" class="btn-block" :busy="busy" :disabled="!name.trim()">
          {{ busy ? t('common.creating') : t('onboarding.submit') }}
        </AppButton>
      </form>
    </div>
  </div>
</template>
