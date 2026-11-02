<script setup lang="ts">
import type { ShownError } from '#base/lib/apperror/message';
import type { ProjectEnvironment } from '#gen/project/v1/project_pb';
import ErrorText from '#base/components/ui/ErrorText.vue';
import SavedNote from '#base/components/ui/SavedNote.vue';
import Section from '#base/components/ui/Section.vue';
import { useErrorMessage } from '#base/composables/useErrorMessage';
import { useProjectsStore } from '~/stores/projects';
import { environmentLabelKey, environmentOptions } from './environment';

const { projectId, environment, canEdit } = defineProps<{ projectId: string, environment?: ProjectEnvironment, canEdit: boolean }>();

const { t } = useI18n();
const errorMessage = useErrorMessage();
const projects = useProjectsStore();
const { $projectClient: client } = useNuxtApp();
const selectId = useId();
const busy = ref(false);
const saved = ref(false);
const error = ref<ShownError | null>(null);

async function change(e: Event) {
  const next = Number((e.target as HTMLSelectElement).value) as ProjectEnvironment;
  busy.value = true;
  saved.value = false;
  error.value = null;
  try {
    const res = await client.updateProjectEnvironment({ id: projectId, environment: next });
    projects.setEnvironment(projectId, res.project?.environment ?? next);
    saved.value = true;
  } catch (err) {
    error.value = errorMessage(err, 'error.save_environment');
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <Section :title="t('environment.title')" :description="t('environment.help')" data-testid="environment-card">
    <div v-if="canEdit" class="flex flex-wrap items-center gap-3">
      <label :for="selectId" class="sr-only">{{ t('environment.title') }}</label>
      <select :id="selectId" class="select select-sm w-56 text-ui" :value="environment ?? 0" :disabled="busy" @change="change">
        <option v-for="o in environmentOptions" :key="o.value" :value="o.value">
          {{ t(o.labelKey) }}
        </option>
      </select>
      <SavedNote v-if="saved" :label="t('common.saved')" />
    </div>
    <p v-else class="text-sm">
      {{ t(environmentLabelKey(environment)) }}
    </p>
    <ErrorText :error="error" />
  </Section>
</template>
