<script setup lang="ts">
import type { Project } from '~/stores/projects';
import ProjectEditModal from '~/components/project/edit/ProjectEditModal.vue';
import AppButton from '~/components/ui/AppButton.vue';
import CopyButton from '~/components/ui/CopyButton.vue';
import ErrorText from '~/components/ui/ErrorText.vue';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';
import { useUIStore } from '~/stores/ui';

const route = useRoute();
const { t } = useI18n();
const orgs = useOrgsStore();
const projects = useProjectsStore();
const ui = useUIStore();
const org = computed(() => orgs.bySlug(route.params.org as string));
const items = computed(() => (org.value ? projects.forOrg(org.value.id) : []));

const editModal = ref<InstanceType<typeof ProjectEditModal>>();
function edit(project: Project) {
  editModal.value?.show(project);
}
</script>

<template>
  <div class="space-y-6">
    <div class="flex items-start justify-between gap-4">
      <h1 class="text-2xl font-bold">
        {{ t('project.title') }}
      </h1>
      <AppButton v-if="org" size="sm" @click="ui.openProjectCreate()">
        <Icon name="lucide:plus" size="16" />
        {{ t('project.new') }}
      </AppButton>
    </div>

    <ErrorText v-if="projects.error" :error="projects.error" />
    <div v-else-if="items.length" class="overflow-x-auto rounded-box border border-base-content/5 bg-base-100">
      <table class="table table-sm">
        <thead>
          <tr>
            <th>{{ t('common.name') }}</th>
            <th>{{ t('common.id') }}</th>
            <th>{{ t('common.slug') }}</th>
            <th />
          </tr>
        </thead>
        <tbody>
          <tr v-for="project in items" :key="project.id" class="hover:bg-base-200">
            <td>
              <NuxtLink :to="`/o/${org?.slug}/projects/${project.slug}`" class="link">
                {{ project.name }}
              </NuxtLink>
            </td>
            <td>
              <span class="inline-flex items-center gap-1">
                <span class="font-mono text-xs text-base-content/60">{{ project.id }}</span>
                <CopyButton :value="project.id" :label="t('common.copy')" />
              </span>
            </td>
            <td class="text-base-content/60">
              {{ project.slug }}
            </td>
            <td class="whitespace-nowrap text-right">
              <NuxtLink :to="`/o/${org?.slug}/projects/${project.slug}`" class="btn btn-ghost btn-xs">
                <Icon name="lucide:external-link" size="14" />
                {{ t('common.open') }}
              </NuxtLink>
              <button class="btn btn-ghost btn-xs" @click="edit(project)">
                <Icon name="lucide:pencil" size="14" />
                {{ t('common.edit') }}
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <p v-else class="text-sm text-base-content/60">
      {{ t('project.empty') }}
    </p>

    <ProjectEditModal ref="editModal" />
  </div>
</template>
