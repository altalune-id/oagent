<script setup lang="ts">
import type { ShownError } from '#base/lib/apperror/message';
import AppButton from '#base/components/ui/AppButton.vue';
import ErrorText from '#base/components/ui/ErrorText.vue';
import KeyValue from '#base/components/ui/KeyValue.vue';
import PageHeader from '#base/components/ui/PageHeader.vue';
import SavedNote from '#base/components/ui/SavedNote.vue';
import { useErrorMessage } from '#base/composables/useErrorMessage';
import ProjectEnvironmentCard from '~/components/project/ProjectEnvironmentCard.vue';
import ProjectReadOnlyNote from '~/components/project/ProjectReadOnlyNote.vue';
import { useCan } from '~/composables/useCan';
import { useAuthStore } from '~/stores/auth';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

const route = useRoute();
const { t } = useI18n();
const canEdit = useCan('project:write');
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

watch([name, slug], () => {
  saved.value = false;
});

const facts = computed(() => {
  const id = { label: t('common.id'), value: project.value?.id ?? '', mono: true, copy: true };
  if (canEdit.value) {
    return [id];
  }
  return [id, { label: t('common.name'), value: project.value?.name ?? '' }, { label: t('common.slug'), value: project.value?.slug ?? '', mono: true }];
});

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

useHead({ title: () => `${project.value?.name ?? t('project.title')} · ${t('settings.title')} · OAgent` });

await useAuthStore().load();
</script>

<template>
  <div v-if="project" class="flex max-w-2xl flex-col gap-5">
    <PageHeader :title="t('settings.title')" :description="t('project.settings_subtitle')" />
    <ProjectReadOnlyNote v-if="!canEdit" :text="t('project.read_only')" />

    <div class="rounded-box border border-base-300 bg-base-100" :data-testid="canEdit ? undefined : 'project-settings-readonly'">
      <KeyValue :items="facts" :copy-label="t('common.copy_label', { label: '{label}' })" />
    </div>

    <form v-if="canEdit" novalidate class="flex flex-col gap-4 rounded-box border border-base-300 bg-base-100 p-4" @submit.prevent="save()">
      <label class="flex flex-col gap-1">
        <span class="label text-ui text-base-content">{{ t('common.name') }}</span>
        <input v-model="name" type="text" class="input input-sm w-full text-ui" :disabled="busy">
      </label>
      <label class="flex flex-col gap-1">
        <span class="label text-ui text-base-content">{{ t('common.slug') }}</span>
        <input v-model="slug" type="text" class="input input-sm w-full font-mono text-ui" :disabled="busy">
      </label>
      <ErrorText :error="error" />
      <div class="flex items-center gap-3">
        <AppButton type="submit" size="sm" :disabled="busy || !name.trim()">
          <Icon name="lucide:check" size="16" class="shrink-0" aria-hidden="true" />
          {{ busy ? t('common.saving') : t('common.save') }}
        </AppButton>
        <SavedNote v-if="saved" :label="t('common.saved')" />
      </div>
    </form>

    <ProjectEnvironmentCard :project-id="project.id" :environment="project.environment" :can-edit="canEdit" />
  </div>
</template>
