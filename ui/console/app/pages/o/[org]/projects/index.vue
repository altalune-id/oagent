<script setup lang="ts">
import type { Column } from '#base/components/ui/types';
import type { Project } from '~/stores/projects';
import AppBadge from '#base/components/ui/AppBadge.vue';
import AppButton from '#base/components/ui/AppButton.vue';
import AppDropdown from '#base/components/ui/AppDropdown.vue';
import { useCopy } from '#base/components/ui/copy';
import DataTable from '#base/components/ui/DataTable.vue';
import EmptyState from '#base/components/ui/EmptyState.vue';
import LoadError from '#base/components/ui/LoadError.vue';
import PageHeader from '#base/components/ui/PageHeader.vue';
import ProjectEditModal from '~/components/project/edit/ProjectEditModal.vue';
import { environmentLabelKey } from '~/components/project/environment';
import { useCan } from '~/composables/useCan';
import { useAuthStore } from '~/stores/auth';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';
import { useUIStore } from '~/stores/ui';

const route = useRoute();
const { t } = useI18n();
const orgs = useOrgsStore();
const projects = useProjectsStore();
const ui = useUIStore();
const canEdit = useCan('project:write');
await useAuthStore().load();
const org = computed(() => orgs.bySlug(route.params.org as string));
const items = computed(() => (org.value ? projects.forOrg(org.value.id) : []).map((p) => {
  const env = environmentLabelKey(p.environment);
  return { ...p, tag: env === 'environment.none' ? '' : t(env), to: `/o/${org.value?.slug}/projects/${p.slug}` };
}));
const { copy } = useCopy();
const columns = computed<Column[]>(() => [
  { key: 'name', label: t('common.name') },
  { key: 'slug', label: t('common.slug'), mono: true },
  { key: 'tag', label: t('environment.title') },
  { key: 'id', label: t('common.id'), mono: true },
  { key: 'menu', label: t('project.col_actions'), hideLabel: true, align: 'end' },
]);

const editModal = ref<InstanceType<typeof ProjectEditModal>>();
function edit(project: Project) {
  editModal.value?.show(project);
}

useHead({ title: () => `${org.value?.name ?? t('org.title')} · ${t('project.title')} · OAgent` });
</script>

<template>
  <div class="flex flex-col gap-4">
    <PageHeader :title="t('project.title')" :description="t('project.list_subtitle')">
      <template #actions>
        <AppButton v-if="org && canEdit" size="sm" @click="ui.openProjectCreate()">
          <Icon name="lucide:plus" size="16" class="shrink-0" aria-hidden="true" />
          {{ t('project.new') }}
        </AppButton>
      </template>
    </PageHeader>

    <LoadError v-if="projects.error" :error="projects.error" :busy="projects.loading" :retry-label="t('common.retry')" @retry="org && projects.load(org.id)" />
    <EmptyState v-else-if="!items.length" icon="lucide:folders" :title="t('project.empty')">
      <template v-if="org && canEdit" #actions>
        <AppButton size="sm" @click="ui.openProjectCreate()">
          <Icon name="lucide:plus" size="16" class="shrink-0" aria-hidden="true" />
          {{ t('project.new') }}
        </AppButton>
      </template>
    </EmptyState>
    <DataTable v-else :caption="t('project.title')" row-key="id" :columns="columns" :rows="items" data-testid="projects-table">
      <template #cell-name="{ row }">
        <NuxtLink :to="row.to" class="link link-hover font-medium">
          {{ row.name }}
        </NuxtLink>
      </template>
      <template #cell-tag="{ row }">
        <AppBadge v-if="row.tag" :label="row.tag" />
      </template>
      <template #cell-id="{ row }">
        <span class="block max-w-44 truncate" :title="row.id">{{ row.id }}</span>
      </template>
      <template #cell-menu="{ row }">
        <AppDropdown placement="end" size="xs" trigger-class="btn-square" content-class="w-48" :label="t('project.more_for', { name: row.name })">
          <template #trigger>
            <Icon name="lucide:ellipsis" size="14" class="shrink-0" aria-hidden="true" />
          </template>
          <ul class="menu menu-sm w-full p-0 text-ui">
            <li>
              <NuxtLink :to="row.to">
                {{ t('common.open') }}
              </NuxtLink>
            </li>
            <li v-if="canEdit">
              <button type="button" @click="edit(row as Project)">
                {{ t('common.edit') }}
              </button>
            </li>
            <li>
              <button type="button" @click="copy(row.id, t('common.copied'))">
                {{ t('project.copy_id') }}
              </button>
            </li>
          </ul>
        </AppDropdown>
      </template>
    </DataTable>

    <ProjectEditModal v-if="canEdit" ref="editModal" />
  </div>
</template>
