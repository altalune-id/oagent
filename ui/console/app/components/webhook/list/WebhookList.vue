<script setup lang="ts">
import type { Webhook } from '~/stores/webhooks';
import AppModal from '~/components/ui/AppModal.vue';
import ErrorText from '~/components/ui/ErrorText.vue';
import SecretField from '~/components/ui/SecretField.vue';
import { webhookEvents } from '~/components/webhook/events';
import { useErrorMessage } from '~/composables/useErrorMessage';
import { toWebhook, useWebhooksStore } from '~/stores/webhooks';

const { orgId, projectId } = defineProps<{ orgId: string, projectId: string }>();

const route = useRoute();
const { t } = useI18n();
const errorMessage = useErrorMessage();
const webhooks = useWebhooksStore();
const { $webhookClient: client } = useNuxtApp();

await webhooks.ensureLoaded(orgId, projectId);

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

async function destroy(webhook: Webhook) {
  busyId.value = webhook.id;
  try {
    await client.deleteWebhook({ orgId, projectId, id: webhook.id });
    webhooks.remove(webhook.id);
  } catch (e) {
    webhooks.error = errorMessage(e, 'error.delete_webhook');
  } finally {
    busyId.value = '';
  }
}

function closeRotate() {
  rotating.value = null;
  rotatedSecret.value = '';
}
</script>

<template>
  <ErrorText v-if="webhooks.error" :error="webhooks.error" />
  <p v-else-if="items.length === 0" class="text-sm text-base-content/60">
    {{ t('webhooks.empty') }}
  </p>
  <div v-else class="overflow-x-auto rounded-box border border-base-content/5 bg-base-100">
    <table class="table table-sm">
      <thead>
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
          <td class="text-base-content/60">
            {{ events(webhook) }}
          </td>
          <td>
            <span class="badge badge-sm" :class="webhook.active ? 'badge-success' : 'badge-ghost'">
              {{ webhook.active ? t('webhooks.active') : t('webhooks.inactive') }}
            </span>
          </td>
          <td class="whitespace-nowrap text-right">
            <NuxtLink :to="`/o/${route.params.org}/projects/${route.params.project}/webhooks/${webhook.id}`" class="btn btn-ghost btn-xs">
              <Icon name="lucide:scroll-text" size="14" />
              {{ t('webhooks.logs') }}
            </NuxtLink>
            <button class="btn btn-ghost btn-xs" :disabled="busyId === webhook.id" @click="ping(webhook)">
              <Icon name="lucide:send" size="14" />
              {{ t('webhooks.ping') }}
            </button>
            <button class="btn btn-ghost btn-xs" :disabled="busyId === webhook.id" @click="toggle(webhook)">
              <Icon :name="webhook.active ? 'lucide:pause' : 'lucide:play'" size="14" />
              {{ webhook.active ? t('webhooks.deactivate') : t('webhooks.activate') }}
            </button>
            <button class="btn btn-ghost btn-xs" :disabled="busyId === webhook.id" @click="rotate(webhook)">
              <Icon name="lucide:key-round" size="14" />
              {{ t('webhooks.rotate') }}
            </button>
            <button class="btn btn-ghost btn-xs text-error" :disabled="busyId === webhook.id" @click="destroy(webhook)">
              <Icon name="lucide:trash-2" size="14" />
              {{ t('common.delete') }}
            </button>
          </td>
        </tr>
      </tbody>
    </table>
  </div>

  <AppModal :open="!!rotating" :title="t('webhooks.rotate')" @close="closeRotate()">
    <div class="space-y-3">
      <p class="text-sm text-warning">
        {{ t('webhooks.secret_warning') }}
      </p>
      <SecretField :value="rotatedSecret" :label="t('common.copy')" />
      <button class="btn btn-ghost btn-sm" type="button" @click="closeRotate()">
        {{ t('common.done') }}
      </button>
    </div>
  </AppModal>
</template>
