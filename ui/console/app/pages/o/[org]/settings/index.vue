<script setup lang="ts">
import type { ShownError } from '#base/lib/apperror/message';
import AppButton from '#base/components/ui/AppButton.vue';
import CopyButton from '#base/components/ui/CopyButton.vue';
import ErrorText from '#base/components/ui/ErrorText.vue';
import SavedNote from '#base/components/ui/SavedNote.vue';
import { useErrorMessage } from '#base/composables/useErrorMessage';
import { useOrgsStore } from '~/stores/orgs';

const route = useRoute();
const { t } = useI18n();
const ids = { name: useId(), slug: useId() };
const errorMessage = useErrorMessage();
const orgs = useOrgsStore();
const { $orgClient: client } = useNuxtApp();

const org = computed(() => orgs.bySlug(route.params.org as string));

if (!org.value) {
  throw createError({ statusCode: 404, statusMessage: t('error.org_not_found'), fatal: true });
}

const name = ref(org.value.name);
const slug = ref(org.value.slug);
const busy = ref(false);
const error = ref<ShownError | null>(null);
const saved = ref(false);

async function save() {
  const current = org.value;
  if (!current || busy.value) {
    return;
  }
  busy.value = true;
  error.value = null;
  saved.value = false;
  try {
    const res = await client.updateOrg({ id: current.id, name: name.value.trim(), slug: slug.value.trim() });
    if (res.org) {
      orgs.update(res.org.id, { name: res.org.name, slug: res.org.slug });
      if (res.org.slug !== route.params.org) {
        await navigateTo(`/o/${res.org.slug}/settings`, { replace: true });
      }
    }
    saved.value = true;
  } catch (e) {
    error.value = errorMessage(e, 'error.update_org');
  } finally {
    busy.value = false;
  }
}

useHead({ title: () => `${org.value?.name ?? t('org.title')} · ${t('settings.title')} · OAgent` });
</script>

<template>
  <div v-if="org" class="max-w-lg space-y-6">
    <h1 class="text-2xl font-bold">
      {{ t('settings.title') }}
    </h1>

    <div class="space-y-1">
      <span class="block text-xs text-base-content/60">{{ t('common.id') }}</span>
      <div class="flex items-center gap-1">
        <code class="rounded bg-base-200 px-2 py-1 font-mono text-xs">{{ org.id }}</code>
        <CopyButton :value="org.id" :label="t('common.copy')" />
      </div>
    </div>

    <form novalidate class="space-y-2" @submit.prevent="save()">
      <div class="fieldset">
        <label :for="ids.name" class="fieldset-legend">{{ t('common.name') }}</label>
        <input :id="ids.name" v-model="name" type="text" class="input w-full" :disabled="busy">
      </div>
      <div class="fieldset">
        <label :for="ids.slug" class="fieldset-legend">{{ t('common.slug') }}</label>
        <input :id="ids.slug" v-model="slug" type="text" class="input w-full font-mono" :disabled="busy">
      </div>
      <ErrorText :error="error" />
      <div class="flex items-center gap-3">
        <AppButton type="submit" size="sm" :disabled="busy || !name.trim()">
          <Icon name="lucide:check" size="16" />
          {{ busy ? t('common.saving') : t('common.save') }}
        </AppButton>
        <SavedNote v-if="saved" :label="t('common.saved')" />
      </div>
    </form>
  </div>
</template>
