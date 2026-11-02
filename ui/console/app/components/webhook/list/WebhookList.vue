<script setup lang="ts">
import type { Webhook } from '~/stores/webhooks';
import AppButton from '#base/components/ui/AppButton.vue';
import AppModal from '#base/components/ui/AppModal.vue';
import ConfirmModal from '#base/components/ui/ConfirmModal.vue';
import EmptyState from '#base/components/ui/EmptyState.vue';
import LoadError from '#base/components/ui/LoadError.vue';
import LoadingStatus from '#base/components/ui/LoadingStatus.vue';
import SecretField from '#base/components/ui/SecretField.vue';
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
  <div v-else class="overflow-x-auto rounded-box border border-base-content/5 bg-base-100">
    <table class="table table-sm">
      <thead class="text-base-content/70">
        <tr>
          <th>{{ t('webhooks.col_url') }}</th>
          <th>{{ t('webhooks.col_events') }}</th>
          <th>{{ t('webhooks.col_status') }}</th>
          <th />
        </tr>
      </thead>
      <tbody>
        <tr v-for="webhook in items" :key="webhook.id" class="hover:bg-base-200">
          <td>
            <span class="font-mono text-xs">{{ webhook.url }}</span>
          </td>
          <td class="text-base-content/70">
            {{ events(webhook) }}
          </td>
          <td>
            <span class="badge badge-sm" :class="webhook.active ? 'badge-success' : 'badge-ghost'">
              {{ webhook.active ? t('webhooks.active') : t('webhooks.inactive') }}
            </span>
          </td>
          <td class="whitespace-nowrap text-right">
            <AppButton variant="ghost" size="xs" :to="`/o/${route.params.org}/projects/${route.params.project}/webhooks/${webhook.id}`">
              <Icon name="lucide:scroll-text" size="14" aria-hidden="true" />
              {{ t('webhooks.logs') }}
            </AppButton>
            <AppButton v-if="canManage" variant="ghost" size="xs" :disabled="busyId === webhook.id" @click="ping(webhook)">
              <Icon name="lucide:send" size="14" />
              {{ t('webhooks.ping') }}
            </AppButton>
            <AppButton v-if="canManage" variant="ghost" size="xs" :disabled="busyId === webhook.id" @click="toggle(webhook)">
              <Icon :name="webhook.active ? 'lucide:pause' : 'lucide:play'" size="14" />
              {{ webhook.active ? t('webhooks.deactivate') : t('webhooks.activate') }}
            </AppButton>
            <AppButton v-if="canManage" variant="ghost" size="xs" :disabled="busyId === webhook.id" @click="rotate(webhook)">
              <Icon name="lucide:key-round" size="14" />
              {{ t('webhooks.rotate') }}
            </AppButton>
            <AppButton v-if="canManage" variant="ghost" size="xs" :disabled="busyId === webhook.id" @click="deleting.ask(webhook)">
              <Icon name="lucide:trash-2" size="14" class="text-error" />
              {{ t('common.delete') }}
            </AppButton>
          </td>
        </tr>
      </tbody>
    </table>
  </div>

  <AppModal :open="!!rotating" :title="t('webhooks.rotate')" :dismissible="!rotatedSecret" @close="closeRotate()">
    <div class="space-y-3">
      <p class="flex items-start gap-1 text-sm text-base-content">
        <Icon name="lucide:triangle-alert" size="14" class="mt-0.5 shrink-0 text-warning" />
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
