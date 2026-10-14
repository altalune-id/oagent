<script setup lang="ts">
import type { ShownError } from '~/lib/apperror/message';
import AppButton from '~/components/ui/AppButton.vue';
import CopyButton from '~/components/ui/CopyButton.vue';
import ErrorText from '~/components/ui/ErrorText.vue';
import { useErrorMessage } from '~/composables/useErrorMessage';
import { useOrgsStore } from '~/stores/orgs';

const route = useRoute();
const { t } = useI18n();
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

useHead({ title: () => `${org.value?.name ?? t('org.title')} · ${t('settings.title')} · oagent` });
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

    <form class="space-y-3" @submit.prevent="save()">
      <label class="block">
        <span class="mb-1 block text-xs text-base-content/60">{{ t('common.name') }}</span>
        <input v-model="name" type="text" class="input input-sm input-bordered w-full" :disabled="busy">
      </label>
      <label class="block">
        <span class="mb-1 block text-xs text-base-content/60">{{ t('common.slug') }}</span>
        <input v-model="slug" type="text" class="input input-sm input-bordered w-full" :disabled="busy">
      </label>
      <ErrorText :error="error" />
      <div class="flex items-center gap-3">
        <AppButton type="submit" size="sm" :disabled="busy || !name.trim()">
          <Icon name="lucide:check" size="16" />
          {{ busy ? t('common.saving') : t('common.save') }}
        </AppButton>
        <span v-if="saved" class="text-sm text-success">{{ t('common.saved') }}</span>
      </div>
    </form>
  </div>
</template>
