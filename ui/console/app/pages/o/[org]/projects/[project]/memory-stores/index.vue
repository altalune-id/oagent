<script setup lang="ts">
import type { Column } from '#base/components/ui/types';
import type { ShownError } from '#base/lib/apperror/message';
import type { MemoryStore } from '~/stores/memoryStores';
import AppButton from '#base/components/ui/AppButton.vue';
import { confirmHandle } from '#base/components/ui/confirmHandle';
import ConfirmModal from '#base/components/ui/ConfirmModal.vue';
import DataTable from '#base/components/ui/DataTable.vue';
import EmptyState from '#base/components/ui/EmptyState.vue';
import LoadError from '#base/components/ui/LoadError.vue';
import PageHeader from '#base/components/ui/PageHeader.vue';
import { useToast } from '#base/components/ui/toast';
import { useDates } from '#base/composables/useDates';
import { useErrorMessage } from '#base/composables/useErrorMessage';
import { errorCode } from '#base/lib/apperror/message';
import MemoryStoreCreateModal from '~/components/memory/create/MemoryStoreCreateModal.vue';
import { useMemoryStoreCreate } from '~/components/memory/create/useMemoryStoreCreate';
import ProjectReadOnlyNote from '~/components/project/ProjectReadOnlyNote.vue';
import { useProject } from '~/components/project/useProject';
import { useCan } from '~/composables/useCan';
import { useAgentsStore } from '~/stores/agents';
import { useAuthStore } from '~/stores/auth';
import { useMemoryStoresStore } from '~/stores/memoryStores';

const { t } = useI18n();
const route = useRoute();
const toast = useToast();
const { when, moment } = useDates();
const errorMessage = useErrorMessage();
const { org, project } = useProject();
const canEdit = useCan('memory:write');
const store = useMemoryStoresStore();
const agents = useAgentsStore();
const projectId = project.value!.id;
const base = computed(() => `/o/${route.params.org}/projects/${route.params.project}`);
const form = useMemoryStoreCreate(org.value!.id, projectId);

const columns = computed<Column[]>(() => [
  { key: 'name', label: t('memorystores.col_name') },
  { key: 'usedBy', label: t('memorystores.col_used_by') },
  { key: 'created', label: t('memorystores.col_created'), muted: true },
  ...(canEdit.value ? [{ key: 'menu', label: t('memorystores.col_actions'), hideLabel: true, align: 'end' as const }] : []),
]);

function agentLink(id: string) {
  const slug = agents.items.find(a => a.id === id)?.slug;
  return slug ? `${base.value}/agents/${slug}` : '';
}

const deleting = confirmHandle<MemoryStore>(async (s) => {
  await store.remove(s.id);
  toast.push('success', t('memorystores.deleted', { name: s.name }));
}, (e): ShownError => (errorCode(e) === 'MEM004' ? { message: t('memorystores.in_use_now') } : errorMessage(e, 'error.delete_memory_store')));

function created(name: string) {
  toast.push('success', t('memorystores.created', { name }));
}

useHead({ title: () => `${t('memorystores.title')} · ${project.value?.name ?? ''} · OAgent` });

await Promise.all([useAuthStore().load(), store.load(projectId), agents.ensureLoaded(projectId)]);
</script>

<template>
  <div class="flex flex-col gap-4">
    <PageHeader :title="t('memorystores.title')" :description="t('memorystores.subtitle')">
      <template v-if="canEdit && store.on && store.items.length" #actions>
        <AppButton size="sm" @click="form.show()">
          <Icon name="lucide:plus" size="16" aria-hidden="true" />
          {{ t('memorystores.new') }}
        </AppButton>
      </template>
    </PageHeader>
    <ProjectReadOnlyNote v-if="!canEdit" :text="t('memorystores.read_only')" />

    <LoadError v-if="store.error" :error="store.error" :busy="store.loading" :retry-label="t('common.retry')" @retry="store.load(projectId)" />
    <EmptyState v-else-if="!store.loading && !store.on && !store.items.length" icon="lucide:brain" :title="t('memorystores.off')" :body="t('memorystores.off_body')" />
    <DataTable v-else stacked :caption="t('memorystores.title')" row-key="id" :columns="columns" :rows="store.items" :loading="store.loading">
      <template #empty>
        <EmptyState icon="lucide:brain" :title="t('memorystores.empty')" :body="t('memorystores.empty_body')">
          <template v-if="canEdit" #actions>
            <AppButton size="sm" @click="form.show()">
              <Icon name="lucide:plus" size="16" aria-hidden="true" />
              {{ t('memorystores.new') }}
            </AppButton>
          </template>
        </EmptyState>
      </template>
      <template #cell-name="{ row }">
        <span class="flex flex-wrap items-center gap-2">
          <span class="font-medium">{{ row.name }}</span>
          <span v-if="row.rememberApiUsers" class="badge badge-sm badge-ghost">{{ t('memorystores.api_users') }}</span>
        </span>
      </template>
      <template #cell-usedBy="{ row }">
        <span v-if="row.usedBy.length" class="flex flex-wrap gap-x-2">
          <template v-for="a in row.usedBy" :key="a.id">
            <NuxtLink v-if="agentLink(a.id)" :to="agentLink(a.id)" class="link link-hover inline-flex min-h-6 items-center">{{ a.name }}</NuxtLink>
            <span v-else>{{ a.name }}</span>
          </template>
        </span>
        <span v-else class="text-base-content/70">{{ t('memorystores.unused') }}</span>
      </template>
      <template #cell-created="{ row }">
        <span :title="row.createdAt ? moment(row.createdAt) : undefined">{{ row.createdAt ? when(row.createdAt) : '—' }}</span>
      </template>
      <template #cell-menu="{ row }">
        <AppButton variant="ghost" size="xs" square class="max-sm:size-11" :aria-label="t('memorystores.delete_store', { name: row.name })" :title="t('memorystores.delete_store', { name: row.name })" @click="deleting.ask(row as MemoryStore)">
          <Icon name="lucide:trash-2" size="14" class="text-error" aria-hidden="true" />
        </AppButton>
      </template>
    </DataTable>

    <MemoryStoreCreateModal v-if="canEdit" :form="form" @created="created" />
    <ConfirmModal
      v-if="canEdit"
      :handle="deleting"
      :title="deleting.target.value?.usedBy.length ? t('memorystores.in_use_title', { name: deleting.target.value.name }) : t('memorystores.delete_title', { name: deleting.target.value?.name ?? '' })"
      :body="deleting.target.value?.usedBy.length ? t('memorystores.in_use_body') : t('memorystores.delete_body')"
      :confirm-label="t('common.delete')"
      :cancel-label="t('common.cancel')"
      :confirm-disabled="!!deleting.target.value?.usedBy.length"
    >
      <ul v-if="deleting.target.value?.usedBy.length" class="mt-2 flex flex-col gap-1 text-ui">
        <li v-for="a in deleting.target.value.usedBy" :key="a.id">
          <NuxtLink v-if="agentLink(a.id)" :to="agentLink(a.id)" class="link">
            {{ a.name }}
          </NuxtLink>
          <span v-else>{{ a.name }}</span>
        </li>
      </ul>
    </ConfirmModal>
  </div>
</template>
