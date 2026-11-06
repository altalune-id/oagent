<script setup lang="ts">
import type { Column } from '#base/components/ui/types';
import type { McpServer } from '~/stores/mcpServers';
import AppButton from '#base/components/ui/AppButton.vue';
import AppDropdown from '#base/components/ui/AppDropdown.vue';
import AppModal from '#base/components/ui/AppModal.vue';
import { confirmHandle } from '#base/components/ui/confirmHandle';
import ConfirmModal from '#base/components/ui/ConfirmModal.vue';
import DataTable from '#base/components/ui/DataTable.vue';
import EmptyState from '#base/components/ui/EmptyState.vue';
import LoadError from '#base/components/ui/LoadError.vue';
import PageHeader from '#base/components/ui/PageHeader.vue';
import { useErrorMessage } from '#base/composables/useErrorMessage';
import { errorCode } from '#base/lib/apperror/message';
import McpServerModal from '~/components/mcpserver/McpServerModal.vue';
import McpServerTools from '~/components/mcpserver/McpServerTools.vue';
import { useMcpServerForm } from '~/components/mcpserver/useMcpServerForm';
import ProjectReadOnlyNote from '~/components/project/ProjectReadOnlyNote.vue';
import { useProject } from '~/components/project/useProject';
import { useCan } from '~/composables/useCan';
import { useUnits } from '~/composables/useUnits';
import { useAuthStore } from '~/stores/auth';
import { useMcpServersStore } from '~/stores/mcpServers';

const { t } = useI18n();
const { seconds } = useUnits();
const { project } = useProject();
const canEdit = useCan('mcpserver:write');
const servers = useMcpServersStore();
const form = useMcpServerForm(project.value!.orgId, project.value!.id);

void servers.ensureLoaded(project.value!.id);

const columns = computed<Column[]>(() => [
  { key: 'name', label: t('common.name'), mono: true },
  { key: 'url', label: t('mcpservers.url'), mono: true },
  { key: 'auth', label: t('mcpservers.auth') },
  { key: 'trusted', label: t('mcpservers.trust') },
  { key: 'timeoutMs', label: t('mcpservers.timeout_short'), align: 'end' },
  { key: 'tools', label: t('mcpservers.tools') },
  { key: 'menu', label: t('providers.col_actions'), hideLabel: true, align: 'end' },
]);

const viewingId = ref('');
const viewing = computed(() => servers.items.find(s => s.id === viewingId.value));
const errorMessage = useErrorMessage();
const deleting = confirmHandle<McpServer>(s => servers.remove(s.id), e => (errorCode(e) === 'MCS004' ? { message: t('mcpservers.in_use') } : errorMessage(e, 'error.delete_mcp_server')));

function replaceSecret(s: McpServer) {
  viewingId.value = '';
  form.show(s);
  form.replacingSecret.value = true;
}

function showTools(s: McpServer, reload = false) {
  viewingId.value = s.id;
  void (reload ? servers.loadTools(s.id) : servers.ensureTools(s.id));
}

function authLabel(s: McpServer) {
  if (s.auth === 'header') {
    return t('mcpservers.auth_header_named', { name: s.headerName });
  }
  return s.auth === 'bearer' ? t('mcpservers.auth_bearer') : t('mcpservers.auth_none');
}

useHead({ title: () => `${t('mcpservers.title')} · ${project.value?.name ?? ''} · OAgent` });

await useAuthStore().load();
</script>

<template>
  <div class="flex flex-col gap-4">
    <PageHeader :title="t('mcpservers.title')" :description="t('mcpservers.subtitle')">
      <template v-if="canEdit && servers.items.length" #actions>
        <AppButton size="sm" @click="form.show()">
          <Icon name="lucide:plus" size="16" aria-hidden="true" />
          {{ t('mcpservers.add') }}
        </AppButton>
      </template>
    </PageHeader>
    <ProjectReadOnlyNote v-if="!canEdit" :text="t('project.read_only')" />

    <LoadError v-if="servers.error" :error="servers.error" :busy="servers.loading" :retry-label="t('common.retry')" @retry="servers.load(project!.id)" />
    <DataTable v-else stacked :caption="t('mcpservers.title')" row-key="id" :columns="columns" :rows="servers.items" :loading="servers.loading">
      <template #empty>
        <EmptyState icon="lucide:plug" :title="t('mcpservers.empty')" :body="t('mcpservers.empty_body')">
          <template v-if="canEdit" #actions>
            <AppButton size="sm" @click="form.show()">
              <Icon name="lucide:plus" size="16" aria-hidden="true" />
              {{ t('mcpservers.add') }}
            </AppButton>
          </template>
        </EmptyState>
      </template>
      <template #cell-name="{ row }">
        <button type="button" class="link link-hover" @click="form.show(row as McpServer)">
          {{ row.name }}
        </button>
      </template>
      <template #cell-url="{ row }">
        <span class="block max-w-64 truncate" :title="row.url">{{ row.url }}</span>
      </template>
      <template #cell-auth="{ row }">
        {{ authLabel(row as McpServer) }}
      </template>
      <template #cell-trusted="{ row }">
        <span v-if="row.trusted" class="badge badge-sm badge-soft text-base-content"><Icon name="lucide:shield-check" size="12" aria-hidden="true" />{{ t('mcpservers.trusted_badge') }}</span>
        <span v-else class="badge badge-sm badge-outline">{{ t('mcpservers.untrusted_badge') }}</span>
      </template>
      <template #cell-timeoutMs="{ row }">
        {{ seconds(row.timeoutMs / 1000) }}
      </template>
      <template #cell-tools="{ row }">
        <button type="button" class="link tabular-nums" aria-haspopup="dialog" @click="showTools(row as McpServer)">
          {{ row.tools.length ? t('mcpservers.tool_count', { n: row.tools.length }, row.tools.length) : t('mcpservers.show_tools') }}
        </button>
      </template>
      <template #cell-menu="{ row }">
        <AppDropdown v-if="canEdit" placement="end" size="xs" trigger-class="btn-square" content-class="w-44" :label="t('providers.actions_for', { name: row.name })">
          <template #trigger>
            <Icon name="lucide:ellipsis" size="14" class="shrink-0" aria-hidden="true" />
          </template>
          <ul class="menu menu-sm w-full p-0 text-ui">
            <li>
              <button type="button" @click="form.show(row as McpServer)">
                <Icon name="lucide:pencil" size="16" class="shrink-0" aria-hidden="true" />
                {{ t('common.edit') }}
              </button>
            </li>
            <li>
              <button type="button" @click="showTools(row as McpServer, true)">
                <Icon name="lucide:refresh-cw" size="16" class="shrink-0" aria-hidden="true" />
                {{ t('mcpservers.load_tools') }}
              </button>
            </li>
            <li>
              <button type="button" @click="deleting.ask(row as McpServer)">
                <Icon name="lucide:trash-2" size="16" class="shrink-0 text-error" aria-hidden="true" />
                {{ t('common.delete') }}
              </button>
            </li>
          </ul>
        </AppDropdown>
      </template>
    </DataTable>

    <McpServerModal :form="form" :read-only="!canEdit" />

    <AppModal :open="!!viewing" :title="t('mcpservers.tools_of', { name: viewing?.name ?? '' })" wide @close="viewingId = ''">
      <McpServerTools v-if="viewing" :server="viewing" :trusted="viewing.trusted" @replace-secret="replaceSecret(viewing)" />
      <template #footer>
        <AppButton variant="plain" size="sm" @click="viewingId = ''">
          {{ t('common.close') }}
        </AppButton>
      </template>
    </AppModal>

    <ConfirmModal
      :handle="deleting"
      :title="t('mcpservers.delete_title')"
      :body="t('mcpservers.delete_body', { name: deleting.target.value?.name ?? '' })"
      :confirm-label="t('common.delete')"
      :cancel-label="t('common.cancel')"
    />
  </div>
</template>
