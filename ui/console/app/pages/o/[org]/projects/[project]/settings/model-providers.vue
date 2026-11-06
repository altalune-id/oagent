<script setup lang="ts">
import type { Column } from '#base/components/ui/types';
import type { ModelProvider } from '~/stores/modelProviders';
import AppButton from '#base/components/ui/AppButton.vue';
import AppDropdown from '#base/components/ui/AppDropdown.vue';
import { confirmHandle } from '#base/components/ui/confirmHandle';
import ConfirmModal from '#base/components/ui/ConfirmModal.vue';
import DataTable from '#base/components/ui/DataTable.vue';
import EmptyState from '#base/components/ui/EmptyState.vue';
import LoadError from '#base/components/ui/LoadError.vue';
import PageHeader from '#base/components/ui/PageHeader.vue';
import StatusLabel from '#base/components/ui/StatusLabel.vue';
import { useErrorMessage } from '#base/composables/useErrorMessage';
import { errorCode } from '#base/lib/apperror/message';
import ProjectReadOnlyNote from '~/components/project/ProjectReadOnlyNote.vue';
import { useProject } from '~/components/project/useProject';
import ProviderModal from '~/components/provider/ProviderModal.vue';
import { hasPrice, useProviderForm } from '~/components/provider/useProviderForm';
import { useCan } from '~/composables/useCan';
import { useAuthStore } from '~/stores/auth';
import { useModelProvidersStore } from '~/stores/modelProviders';

const { t } = useI18n();
const { project } = useProject();
const canEdit = useCan('provider:write');
const providers = useModelProvidersStore();
const form = useProviderForm(project.value!.orgId, project.value!.id);

void providers.ensureLoaded(project.value!.id);

const columns = computed<Column[]>(() => [
  { key: 'name', label: t('common.name') },
  { key: 'baseUrl', label: t('providers.base_url'), mono: true },
  { key: 'models', label: t('providers.models') },
  { key: 'key', label: t('providers.api_key') },
  { key: 'menu', label: t('providers.col_actions'), hideLabel: true, align: 'end' },
]);

const errorMessage = useErrorMessage();
const deleting = confirmHandle<ModelProvider>(p => providers.remove(p.id), e => (errorCode(e) === 'PRV005' ? { message: t('providers.in_use') } : errorMessage(e, 'error.delete_provider')));

function modelSummary(p: ModelProvider) {
  return p.models.slice(0, 2).map(m => m.id).join(', ');
}

useHead({ title: () => `${t('providers.title')} · ${project.value?.name ?? ''} · OAgent` });

await useAuthStore().load();
</script>

<template>
  <div class="flex flex-col gap-4">
    <PageHeader :title="t('providers.title')" :description="t('providers.subtitle')">
      <template v-if="canEdit && providers.items.length" #actions>
        <AppButton size="sm" @click="form.show()">
          <Icon name="lucide:plus" size="16" aria-hidden="true" />
          {{ t('providers.add') }}
        </AppButton>
      </template>
    </PageHeader>
    <ProjectReadOnlyNote v-if="!canEdit" :text="t('project.read_only')" />

    <LoadError v-if="providers.error" :error="providers.error" :busy="providers.loading" :retry-label="t('common.retry')" @retry="providers.load(project!.id)" />
    <DataTable v-else stacked :caption="t('providers.title')" row-key="id" :columns="columns" :rows="providers.items" :loading="providers.loading">
      <template #empty>
        <EmptyState icon="lucide:cpu" :title="t('providers.empty')" :body="t('providers.empty_body')">
          <template v-if="canEdit" #actions>
            <AppButton size="sm" @click="form.show()">
              <Icon name="lucide:plus" size="16" aria-hidden="true" />
              {{ t('providers.add') }}
            </AppButton>
          </template>
        </EmptyState>
      </template>
      <template #cell-name="{ row }">
        <button type="button" class="link link-hover font-medium" @click="form.show(row as ModelProvider)">
          {{ row.name }}
        </button>
      </template>
      <template #cell-baseUrl="{ row }">
        <span class="block max-w-64 truncate" :title="row.baseUrl">{{ row.baseUrl }}</span>
      </template>
      <template #cell-models="{ row }">
        <span class="flex flex-wrap items-center gap-x-2 gap-y-1">
          <span class="tabular-nums">{{ t('providers.model_count', { n: row.models.length }, row.models.length) }}</span>
          <span class="font-mono text-xs text-base-content/70">{{ modelSummary(row as ModelProvider) }}</span>
          <span v-if="row.models.length > 2" class="text-xs text-base-content/70 tabular-nums">{{ t('providers.more_models', { n: row.models.length - 2 }) }}</span>
          <span v-if="row.models.some(m => !hasPrice(m))" class="badge badge-sm badge-outline">{{ t('providers.no_price') }}</span>
        </span>
      </template>
      <template #cell-key="{ row }">
        <StatusLabel :tone="row.keySet ? 'success' : 'warning'" :label="row.keySet ? t('providers.key_set_short') : t('providers.key_missing')" />
      </template>
      <template #cell-menu="{ row }">
        <AppDropdown v-if="canEdit" placement="end" size="xs" trigger-class="btn-square" content-class="w-44" :label="t('providers.actions_for', { name: row.name })">
          <template #trigger>
            <Icon name="lucide:ellipsis" size="14" class="shrink-0" aria-hidden="true" />
          </template>
          <ul class="menu menu-sm w-full p-0 text-ui">
            <li>
              <button type="button" @click="form.show(row as ModelProvider)">
                <Icon name="lucide:pencil" size="16" class="shrink-0" aria-hidden="true" />
                {{ t('common.edit') }}
              </button>
            </li>
            <li>
              <button type="button" @click="deleting.ask(row as ModelProvider)">
                <Icon name="lucide:trash-2" size="16" class="shrink-0 text-error" aria-hidden="true" />
                {{ t('common.delete') }}
              </button>
            </li>
          </ul>
        </AppDropdown>
      </template>
    </DataTable>

    <ProviderModal :form="form" :read-only="!canEdit" />

    <ConfirmModal
      :handle="deleting"
      :title="t('providers.delete_title')"
      :body="t('providers.delete_body', { name: deleting.target.value?.name ?? '' })"
      :confirm-label="t('common.delete')"
      :cancel-label="t('common.cancel')"
    />
  </div>
</template>
