<script setup lang="ts">
import type { ShownError } from '~/lib/apperror/message';
import AppButton from '~/components/ui/AppButton.vue';
import CopyButton from '~/components/ui/CopyButton.vue';
import ErrorText from '~/components/ui/ErrorText.vue';
import { useErrorMessage } from '~/composables/useErrorMessage';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

const route = useRoute();
const { t } = useI18n();
const errorMessage = useErrorMessage();
const orgs = useOrgsStore();
const projects = useProjectsStore();
const { $projectClient: client } = useNuxtApp();

const org = computed(() => orgs.bySlug(route.params.org as string));
const project = computed(() =>
  projects.items.find(item => item.orgId === org.value?.id && item.slug === route.params.project),
);

if (!project.value) {
  throw createError({ statusCode: 404, statusMessage: t('error.project_not_found'), fatal: true });
}

const name = ref(project.value.name);
const slug = ref(project.value.slug);
const busy = ref(false);
const error = ref<ShownError | null>(null);
const saved = ref(false);

async function save() {
  const current = project.value;
  if (!current || busy.value) {
    return;
  }
  busy.value = true;
  error.value = null;
  saved.value = false;
  try {
    const res = await client.updateProject({ id: current.id, name: name.value.trim(), slug: slug.value.trim() });
    if (res.project) {
      projects.update(res.project.id, { name: res.project.name, slug: res.project.slug });
      if (res.project.slug !== route.params.project) {
        await navigateTo(`/o/${route.params.org}/projects/${res.project.slug}/settings`, { replace: true });
      }
    }
    saved.value = true;
  } catch (e) {
    error.value = errorMessage(e, 'error.rename_project');
  } finally {
    busy.value = false;
  }
}

useHead({ title: () => `${project.value?.name ?? t('project.title')} · ${t('settings.title')} · oagent` });
</script>

<template>
  <div v-if="project" class="max-w-lg space-y-6">
    <h1 class="text-2xl font-bold">
      {{ t('settings.title') }}
    </h1>

    <div class="space-y-1">
      <span class="block text-xs text-base-content/60">{{ t('common.id') }}</span>
      <div class="flex items-center gap-1">
        <code class="rounded bg-base-200 px-2 py-1 font-mono text-xs">{{ project.id }}</code>
        <CopyButton :value="project.id" :label="t('common.copy')" />
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
