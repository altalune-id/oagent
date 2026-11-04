<script setup lang="ts">
import type { Column } from '#base/components/ui/types';
import type { Webhook } from '~/stores/webhooks';
import AppButton from '#base/components/ui/AppButton.vue';
import AppDropdown from '#base/components/ui/AppDropdown.vue';
import AppModal from '#base/components/ui/AppModal.vue';
import ConfirmModal from '#base/components/ui/ConfirmModal.vue';
import DataTable from '#base/components/ui/DataTable.vue';
import EmptyState from '#base/components/ui/EmptyState.vue';
import LoadError from '#base/components/ui/LoadError.vue';
import LoadingStatus from '#base/components/ui/LoadingStatus.vue';
import SecretField from '#base/components/ui/SecretField.vue';
import StatusLabel from '#base/components/ui/StatusLabel.vue';
import { useConfirm } from '#base/composables/useConfirm';
import { useErrorMessage } from '#base/composables/useErrorMessage';
import { webhookEvents } from '~/components/webhook/events';
import { toWebhook, useWebhooksStore } from '~/stores/webhooks';

const { orgId, projectId, canManage = false } = defineProps<{ orgId: string, projectId: string, canManage?: boolean }>();

const route = useRoute();
const { t } = useI18n();
const errorMessage = useErrorMessage();
const webhooks = useWebhooksStore();
const { $webhookClient: client } = useNuxtApp();

void webhooks.ensureLoaded(orgId, projectId);

const items = computed(() => webhooks.forProject(projectId));
const labelByName = new Map(webhookEvents.map(e => [e.name, e.labelKey]));

function events(webhook: Webhook) {
  return webhook.eventTypes.map(n => t(labelByName.get(n) ?? n)).join(', ');
}

const columns = computed<Column[]>(() => [
  { key: 'url', label: t('webhooks.col_url'), mono: true },
  { key: 'events', label: t('webhooks.col_events') },
  { key: 'status', label: t('webhooks.col_status') },
  { key: 'menu', label: t('webhooks.col_actions'), hideLabel: true, align: 'end' },
]);

const rotating = ref<Webhook | null>(null);
const rotatedSecret = ref('');
const busyId = ref('');

async function ping(webhook: Webhook) {
  busyId.value = webhook.id;
  try {
    await client.pingWebhook({ orgId, projectId, id: webhook.id });
  } catch (e) {
    webhooks.error = errorMessage(e, 'error.ping_webhook');
  } finally {
    busyId.value = '';
  }
}

async function toggle(webhook: Webhook) {
  busyId.value = webhook.id;
  try {
    const res = await client.updateWebhook({
      orgId,
      projectId,
      id: webhook.id,
      url: webhook.url,
      description: webhook.description,
      eventTypes: webhook.eventTypes,
      active: !webhook.active,
    });
    if (res.webhook) {
      webhooks.replace(toWebhook(res.webhook));
    }
  } catch (e) {
    webhooks.error = errorMessage(e, 'error.update_webhook');
  } finally {
    busyId.value = '';
  }
}

async function rotate(webhook: Webhook) {
  busyId.value = webhook.id;
  try {
    const res = await client.rotateWebhookSecret({ orgId, projectId, id: webhook.id });
    rotating.value = webhook;
    rotatedSecret.value = res.secret;
  } catch (e) {
    webhooks.error = errorMessage(e, 'error.rotate_webhook');
  } finally {
    busyId.value = '';
  }
}

const deleting = useConfirm<Webhook>(async (webhook) => {
  await client.deleteWebhook({ orgId, projectId, id: webhook.id });
  webhooks.remove(webhook.id);
}, 'error.delete_webhook');

function closeRotate() {
  rotating.value = null;
  rotatedSecret.value = '';
}
</script>

<template>
  <LoadError v-if="webhooks.error" :error="webhooks.error" :busy="webhooks.loading" :retry-label="t('common.retry')" @retry="webhooks.load(orgId, projectId)" />
  <LoadingStatus v-else-if="webhooks.loading && items.length === 0" :label="t('common.loading')" />
  <EmptyState v-else-if="items.length === 0" icon="lucide:webhook" :title="t('webhooks.empty')" />
  <DataTable v-else :caption="t('webhooks.title')" row-key="id" :columns="columns" :rows="items">
    <template #cell-url="{ row }">
      <span class="block max-w-80 truncate" :title="row.url">{{ row.url }}</span>
    </template>
    <template #cell-events="{ row }">
      <span class="text-base-content/70">{{ events(row as Webhook) }}</span>
    </template>
    <template #cell-status="{ row }">
      <StatusLabel :tone="row.active ? 'success' : 'neutral'" :label="row.active ? t('webhooks.active') : t('webhooks.inactive')" />
    </template>
    <template #cell-menu="{ row }">
      <AppDropdown placement="end" size="xs" trigger-class="btn-square" content-class="w-48" :label="t('webhooks.actions_for', { url: row.url })" :disabled="busyId === row.id">
        <template #trigger>
          <Icon name="lucide:ellipsis" size="14" class="shrink-0" aria-hidden="true" />
        </template>
        <ul class="menu menu-sm w-full p-0 text-ui">
          <li>
            <NuxtLink :to="`/o/${route.params.org}/projects/${route.params.project}/webhooks/${row.id}`">
              <Icon name="lucide:scroll-text" size="16" class="shrink-0" aria-hidden="true" />
              {{ t('webhooks.logs') }}
            </NuxtLink>
          </li>
          <template v-if="canManage">
            <li>
              <button type="button" @click="ping(row as Webhook)">
                <Icon name="lucide:send" size="16" class="shrink-0" aria-hidden="true" />
                {{ t('webhooks.ping') }}
              </button>
            </li>
            <li>
              <button type="button" @click="toggle(row as Webhook)">
                <Icon :name="row.active ? 'lucide:pause' : 'lucide:play'" size="16" class="shrink-0" aria-hidden="true" />
                {{ row.active ? t('webhooks.deactivate') : t('webhooks.activate') }}
              </button>
            </li>
            <li>
              <button type="button" @click="rotate(row as Webhook)">
                <Icon name="lucide:key-round" size="16" class="shrink-0" aria-hidden="true" />
                {{ t('webhooks.rotate') }}
              </button>
            </li>
            <li>
              <button type="button" @click="deleting.ask(row as Webhook)">
                <Icon name="lucide:trash-2" size="16" class="shrink-0 text-error" aria-hidden="true" />
                {{ t('common.delete') }}
              </button>
            </li>
          </template>
        </ul>
      </AppDropdown>
    </template>
  </DataTable>

  <AppModal :open="!!rotating" :title="t('webhooks.rotate')" :dismissible="!rotatedSecret" @close="closeRotate()">
    <div class="space-y-3">
      <p class="flex items-start gap-1.5 text-ui text-base-content">
        <Icon name="lucide:triangle-alert" size="16" class="mt-0.5 shrink-0 text-warning" aria-hidden="true" />
        {{ t('webhooks.secret_warning') }}
      </p>
      <SecretField :value="rotatedSecret" :label="t('common.copy')" />
      <AppButton variant="ghost" size="sm" @click="closeRotate()">
        {{ t('common.done') }}
      </AppButton>
    </div>
  </AppModal>

  <ConfirmModal
    v-if="canManage"
    :handle="deleting"
    :title="t('webhooks.delete_title')"
    :body="t('webhooks.delete_body', { url: deleting.target.value?.url ?? '' })"
    :confirm-label="t('common.delete')"
    :cancel-label="t('common.cancel')"
  />
</template>
