<script setup lang="ts">
import type { Column, StatusTone } from '#base/components/ui/types';
import type { ShownError } from '#base/lib/apperror/message';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import AppButton from '#base/components/ui/AppButton.vue';
import AppModal from '#base/components/ui/AppModal.vue';
import DataTable from '#base/components/ui/DataTable.vue';
import EmptyState from '#base/components/ui/EmptyState.vue';
import ErrorText from '#base/components/ui/ErrorText.vue';
import LoadError from '#base/components/ui/LoadError.vue';
import LoadingStatus from '#base/components/ui/LoadingStatus.vue';
import PageHeader from '#base/components/ui/PageHeader.vue';
import SavedNote from '#base/components/ui/SavedNote.vue';
import StatusLabel from '#base/components/ui/StatusLabel.vue';
import { useDates } from '#base/composables/useDates';
import { useErrorMessage } from '#base/composables/useErrorMessage';
import { useCan } from '~/composables/useCan';
import { useAuthStore } from '~/stores/auth';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

interface Delivery {
  id: string;
  eventType: string;
  attempt: number;
  status: string;
  lastError: string;
  payload: string;
  createdAt?: Date;
}

interface Attempt {
  attempt: number;
  statusCode: number;
  error: string;
  durationMs: number;
  responseBody: string;
  responseTruncated: boolean;
  responseHeaders: { name: string, value: string }[];
}

const route = useRoute();
const { t } = useI18n();
const canRetry = useCan('webhook:write');
const { moment } = useDates();
const errorMessage = useErrorMessage();
const orgs = useOrgsStore();
const projects = useProjectsStore();
const { $webhookClient: client } = useNuxtApp();

const org = computed(() => orgs.bySlug(route.params.org as string));
const project = computed(() =>
  projects.items.find(item => item.orgId === org.value?.id && item.slug === route.params.project),
);

if (!project.value) {
  throw createError({ statusCode: 404, statusMessage: t('error.project_not_found'), fatal: true });
}

const webhookId = computed(() => route.params.webhook as string);
const deliveries = ref<Delivery[]>([]);
const error = ref<ShownError | null>(null);
const busy = ref(false);

async function load() {
  busy.value = true;
  error.value = null;
  try {
    const res = await client.listWebhookDeliveries({
      orgId: org.value?.id ?? '',
      projectId: project.value?.id ?? '',
      id: webhookId.value,
      limit: 50,
    });
    deliveries.value = (res.deliveries ?? []).map(d => ({
      id: d.id,
      eventType: d.eventType,
      attempt: d.attempt,
      status: d.status,
      lastError: d.lastError,
      payload: d.payload,
      createdAt: d.createdAt ? timestampDate(d.createdAt) : undefined,
    }));
  } catch (e) {
    error.value = errorMessage(e, 'error.load_deliveries');
  } finally {
    busy.value = false;
  }
}

const selected = ref<Delivery | null>(null);
const attempts = ref<Attempt[]>([]);
const attemptsError = ref<ShownError | null>(null);

async function showAttempts(delivery: Delivery) {
  selected.value = delivery;
  attempts.value = [];
  attemptsError.value = null;
  try {
    const res = await client.listWebhookDeliveryAttempts({
      orgId: org.value?.id ?? '',
      projectId: project.value?.id ?? '',
      id: webhookId.value,
      deliveryId: delivery.id,
    });
    attempts.value = (res.attempts ?? []).map(a => ({
      attempt: a.attempt,
      statusCode: a.statusCode,
      error: a.error,
      durationMs: a.durationMs,
      responseBody: a.responseBody,
      responseTruncated: a.responseTruncated,
      responseHeaders: (a.responseHeaders ?? []).map(h => ({ name: h.name, value: h.value })),
    }));
  } catch (e) {
    attemptsError.value = errorMessage(e, 'error.load_attempts');
  }
}

function closeAttempts() {
  selected.value = null;
  attempts.value = [];
  attemptsError.value = null;
}

const retrying = ref(false);
const retryError = ref<ShownError | null>(null);
const requeued = ref('');
const retryOneError = ref<ShownError | null>(null);

async function retryFailed() {
  retrying.value = true;
  retryError.value = null;
  requeued.value = '';
  try {
    const res = await client.retryFailedWebhookDeliveries({
      orgId: org.value?.id ?? '',
      projectId: project.value?.id ?? '',
      id: webhookId.value,
    });
    requeued.value = t('webhooks.requeued', { n: res.requeued });
    await load();
  } catch (e) {
    retryError.value = errorMessage(e, 'error.retry_deliveries');
  } finally {
    retrying.value = false;
  }
}

async function retryOne() {
  if (!selected.value) {
    return;
  }
  retrying.value = true;
  retryOneError.value = null;
  try {
    await client.retryWebhookDelivery({
      orgId: org.value?.id ?? '',
      projectId: project.value?.id ?? '',
      id: webhookId.value,
      deliveryId: selected.value.id,
    });
    closeAttempts();
    await load();
  } catch (e) {
    retryOneError.value = errorMessage(e, 'error.retry_deliveries');
  } finally {
    retrying.value = false;
  }
}

const prettyPayload = computed(() => {
  const raw = selected.value?.payload ?? '';
  try {
    return JSON.stringify(JSON.parse(raw), null, 2);
  } catch {
    return raw;
  }
});

useHead({ title: () => `${project.value?.name ?? t('project.title')} · ${t('webhooks.deliveries')} · OAgent` });

const columns = computed<Column[]>(() => [
  { key: 'eventType', label: t('webhooks.col_event'), mono: true },
  { key: 'status', label: t('webhooks.col_status') },
  { key: 'attempt', label: t('webhooks.col_attempt'), align: 'end' },
  { key: 'createdAt', label: t('webhooks.col_created'), muted: true },
  { key: 'open', label: t('webhooks.view_attempts'), hideLabel: true, align: 'end' },
]);

function deliveryTone(status: string): StatusTone {
  if (status === 'delivered') {
    return 'success';
  }
  return status === 'failed' ? 'error' : 'neutral';
}

const statusLabels: Record<string, string> = {
  pending: t('webhooks.status_pending'),
  delivered: t('webhooks.status_delivered'),
  failed: t('webhooks.status_failed'),
};

function statusLabel(status: string): string {
  return statusLabels[status] ?? status;
}

function codeLabel(code: number): string {
  return code ? String(code) : t('webhooks.no_response');
}

function codeTone(code: number): StatusTone {
  return code >= 200 && code < 300 ? 'success' : 'error';
}

void load();
await useAuthStore().load();
</script>

<template>
  <div class="flex flex-col gap-4">
    <PageHeader
      :title="t('webhooks.deliveries')"
      :crumbs="[{ label: t('webhooks.title'), to: `/o/${route.params.org}/projects/${route.params.project}/webhooks` }, { label: t('webhooks.deliveries') }]"
      :crumbs-label="t('common.breadcrumb')"
    >
      <template #actions>
        <AppButton v-if="canRetry" variant="ghost" size="sm" :disabled="retrying" @click="retryFailed">
          <Icon name="lucide:rotate-cw" size="16" class="shrink-0" aria-hidden="true" />
          {{ t('webhooks.retry_failed') }}
        </AppButton>
        <AppButton variant="ghost" size="sm" :disabled="busy" @click="load">
          <Icon name="lucide:refresh-cw" size="16" class="shrink-0" aria-hidden="true" />
          {{ t('common.refresh') }}
        </AppButton>
      </template>
    </PageHeader>

    <ErrorText :error="retryError" />
    <SavedNote v-if="requeued" :label="requeued" />

    <LoadError v-if="error" :error="error" :busy="busy" :retry-label="t('common.retry')" @retry="load" />
    <LoadingStatus v-else-if="busy && deliveries.length === 0" :label="t('common.loading')" />
    <EmptyState v-else-if="deliveries.length === 0" icon="lucide:inbox" :title="t('webhooks.no_deliveries')" />
    <DataTable v-else :caption="t('webhooks.deliveries')" row-key="id" :columns="columns" :rows="deliveries">
      <template #cell-eventType="{ row }">
        {{ row.eventType || '—' }}
      </template>
      <template #cell-status="{ row }">
        <StatusLabel :tone="deliveryTone(row.status)" :label="statusLabel(row.status)" />
      </template>
      <template #cell-createdAt="{ row }">
        <span class="tabular-nums">{{ moment(row.createdAt) }}</span>
      </template>
      <template #cell-open="{ row }">
        <AppButton variant="ghost" size="xs" @click="showAttempts(row as Delivery)">
          {{ t('webhooks.view_attempts') }}
          <Icon name="lucide:chevron-right" size="14" class="shrink-0" aria-hidden="true" />
        </AppButton>
      </template>
    </DataTable>

    <AppModal :open="!!selected" :title="t('webhooks.delivery_attempts')" @close="closeAttempts()">
      <div v-if="selected && selected.payload" class="mb-4 flex flex-col gap-1">
        <span class="text-xs text-base-content/70">{{ t('webhooks.payload') }}</span>
        <pre class="max-h-64 overflow-auto rounded-box bg-base-200 p-3 font-mono text-xs">{{ prettyPayload }}</pre>
      </div>
      <ErrorText v-if="attemptsError" :error="attemptsError" />
      <p v-else-if="attempts.length === 0" class="text-ui text-base-content/70">
        {{ t('webhooks.no_attempts') }}
      </p>
      <ul v-else class="flex flex-col gap-3">
        <li v-for="attempt in attempts" :key="attempt.attempt" class="rounded-box border border-base-300 p-3">
          <div class="flex flex-wrap items-center gap-3 text-ui">
            <StatusLabel :tone="codeTone(attempt.statusCode)" :label="codeLabel(attempt.statusCode)" />
            <span class="tabular-nums text-base-content/70">#{{ attempt.attempt }}</span>
            <span class="tabular-nums text-base-content/70">{{ attempt.durationMs }}ms</span>
          </div>
          <p v-if="attempt.error" class="mt-1 flex items-start gap-1.5 text-ui text-base-content">
            <Icon name="lucide:circle-alert" size="16" class="mt-0.5 shrink-0 text-error" aria-hidden="true" />
            {{ attempt.error }}
          </p>
          <div v-if="attempt.responseBody" class="mt-2 flex flex-col gap-1">
            <span class="text-xs text-base-content/70">
              {{ t('webhooks.response') }}<template v-if="attempt.responseTruncated"> ({{ t('webhooks.response_truncated') }})</template>
            </span>
            <pre class="max-h-40 overflow-auto rounded-box bg-base-200 p-2 font-mono text-xs">{{ attempt.responseBody }}</pre>
          </div>
          <dl v-if="attempt.responseHeaders.length" class="mt-2 grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 text-xs text-base-content/70">
            <template v-for="header in attempt.responseHeaders" :key="header.name">
              <dt class="font-mono">
                {{ header.name }}
              </dt>
              <dd class="truncate">
                {{ header.value }}
              </dd>
            </template>
          </dl>
        </li>
      </ul>
      <template #footer>
        <ErrorText :error="retryOneError" class="mr-auto" />
        <AppButton v-if="canRetry && selected?.status === 'failed'" size="sm" :disabled="retrying" @click="retryOne">
          {{ t('webhooks.retry') }}
        </AppButton>
        <AppButton variant="ghost" size="sm" @click="closeAttempts()">
          {{ t('common.close') }}
        </AppButton>
      </template>
    </AppModal>
  </div>
</template>
