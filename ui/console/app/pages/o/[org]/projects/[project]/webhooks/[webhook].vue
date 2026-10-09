<script setup lang="ts">
import { timestampDate } from '@bufbuild/protobuf/wkt'
import AppModal from '~/components/ui/AppModal.vue'
import { useOrgsStore } from '~/stores/orgs'
import { useProjectsStore } from '~/stores/projects'

interface Delivery {
  id: string
  eventType: string
  attempt: number
  status: string
  lastError: string
  payload: string
  createdAt?: Date
}

interface Attempt {
  attempt: number
  statusCode: number
  error: string
  durationMs: number
  responseBody: string
  responseTruncated: boolean
  responseHeaders: { name: string, value: string }[]
}

const route = useRoute()
const { t } = useI18n()
const orgs = useOrgsStore()
const projects = useProjectsStore()
const { $webhookClient: client } = useNuxtApp()

const org = computed(() => orgs.bySlug(route.params.org as string))
const project = computed(() =>
  projects.items.find(item => item.orgId === org.value?.id && item.slug === route.params.project),
)

if (!project.value) {
  throw createError({ statusCode: 404, statusMessage: t('error.project_not_found'), fatal: true })
}

const webhookId = computed(() => route.params.webhook as string)
const deliveries = ref<Delivery[]>([])
const error = ref('')
const busy = ref(false)

async function load() {
  busy.value = true
  error.value = ''
  try {
    const res = await client.listWebhookDeliveries({
      orgId: org.value?.id ?? '', projectId: project.value?.id ?? '', id: webhookId.value, limit: 50,
    })
    deliveries.value = (res.deliveries ?? []).map(d => ({
      id: d.id,
      eventType: d.eventType,
      attempt: d.attempt,
      status: d.status,
      lastError: d.lastError,
      payload: d.payload,
      createdAt: d.createdAt ? timestampDate(d.createdAt) : undefined,
    }))
  }
  catch (e) {
    error.value = e instanceof Error ? e.message : t('error.load_deliveries')
  }
  finally {
    busy.value = false
  }
}

const selected = ref<Delivery | null>(null)
const attempts = ref<Attempt[]>([])
const attemptsError = ref('')

async function showAttempts(delivery: Delivery) {
  selected.value = delivery
  attempts.value = []
  attemptsError.value = ''
  try {
    const res = await client.listWebhookDeliveryAttempts({
      orgId: org.value?.id ?? '', projectId: project.value?.id ?? '', id: webhookId.value, deliveryId: delivery.id,
    })
    attempts.value = (res.attempts ?? []).map(a => ({
      attempt: a.attempt,
      statusCode: a.statusCode,
      error: a.error,
      durationMs: a.durationMs,
      responseBody: a.responseBody,
      responseTruncated: a.responseTruncated,
      responseHeaders: (a.responseHeaders ?? []).map(h => ({ name: h.name, value: h.value })),
    }))
  }
  catch (e) {
    attemptsError.value = e instanceof Error ? e.message : t('error.load_attempts')
  }
}

function closeAttempts() {
  selected.value = null
  attempts.value = []
  attemptsError.value = ''
}

const prettyPayload = computed(() => {
  const raw = selected.value?.payload ?? ''
  try {
    return JSON.stringify(JSON.parse(raw), null, 2)
  }
  catch {
    return raw
  }
})

await load()
useHead({ title: () => `${project.value?.name ?? t('project.title')} · ${t('webhooks.deliveries')} · oagent` })
</script>

<template>
  <div class="space-y-6">
    <div class="flex items-start justify-between gap-4">
      <div>
        <NuxtLink :to="`/o/${route.params.org}/projects/${route.params.project}/webhooks`" class="link text-sm text-base-content/60">
          {{ t('webhooks.title') }}
        </NuxtLink>
        <h1 class="text-2xl font-bold">
          {{ t('webhooks.deliveries') }}
        </h1>
      </div>
      <button class="btn btn-ghost btn-sm" :disabled="busy" @click="load">
        <Icon name="lucide:refresh-cw" size="14" />
        {{ t('common.refresh') }}
      </button>
    </div>

    <p v-if="error" class="text-sm text-error">
      {{ error }}
    </p>
    <p v-else-if="deliveries.length === 0" class="text-sm text-base-content/60">
      {{ t('webhooks.no_deliveries') }}
    </p>
    <div v-else class="overflow-x-auto rounded-box border border-base-content/5 bg-base-100">
      <table class="table table-sm">
        <thead>
          <tr>
            <th>{{ t('webhooks.col_event') }}</th>
            <th>{{ t('webhooks.col_status') }}</th>
            <th>{{ t('webhooks.col_attempt') }}</th>
            <th>{{ t('webhooks.col_created') }}</th>
            <th />
          </tr>
        </thead>
        <tbody>
          <tr v-for="delivery in deliveries" :key="delivery.id" class="cursor-pointer hover:bg-base-200" @click="showAttempts(delivery)">
            <td class="font-mono text-xs">{{ delivery.eventType || '—' }}</td>
            <td>
              <span class="badge badge-sm" :class="delivery.status === 'delivered' ? 'badge-success' : delivery.status === 'failed' ? 'badge-error' : 'badge-ghost'">
                {{ delivery.status }}
              </span>
            </td>
            <td>{{ delivery.attempt }}</td>
            <td class="text-base-content/60">{{ delivery.createdAt?.toLocaleString() }}</td>
            <td class="text-right text-base-content/40">
              <Icon name="lucide:chevron-right" size="16" />
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <AppModal :open="!!selected" :title="t('webhooks.delivery_attempts')" @close="closeAttempts()">
      <div v-if="selected && selected.payload" class="mb-4 space-y-1">
        <span class="block text-xs text-base-content/60">{{ t('webhooks.payload') }}</span>
        <pre class="max-h-64 overflow-auto rounded-box bg-base-200 p-3 text-xs">{{ prettyPayload }}</pre>
      </div>
      <p v-if="attemptsError" class="text-sm text-error">
        {{ attemptsError }}
      </p>
      <p v-else-if="attempts.length === 0" class="text-sm text-base-content/60">
        {{ t('webhooks.no_attempts') }}
      </p>
      <div v-else class="space-y-3">
        <div v-for="attempt in attempts" :key="attempt.attempt" class="rounded-box border border-base-300 p-3">
          <div class="flex items-center gap-3 text-sm">
            <span class="badge badge-sm" :class="attempt.statusCode >= 200 && attempt.statusCode < 300 ? 'badge-success' : 'badge-error'">
              {{ attempt.statusCode }}
            </span>
            <span class="text-base-content/60">#{{ attempt.attempt }}</span>
            <span class="text-base-content/60">{{ attempt.durationMs }}ms</span>
          </div>
          <p v-if="attempt.error" class="mt-1 text-sm text-error">
            {{ attempt.error }}
          </p>
          <div v-if="attempt.responseBody" class="mt-2 space-y-1">
            <span class="block text-xs text-base-content/60">
              {{ t('webhooks.response') }}<template v-if="attempt.responseTruncated"> ({{ t('webhooks.response_truncated') }})</template>
            </span>
            <pre class="max-h-40 overflow-auto rounded-box bg-base-200 p-2 text-xs">{{ attempt.responseBody }}</pre>
          </div>
          <dl v-if="attempt.responseHeaders.length" class="mt-2 grid grid-cols-[auto_1fr] gap-x-3 text-xs text-base-content/60">
            <template v-for="header in attempt.responseHeaders" :key="header.name">
              <dt class="font-mono">{{ header.name }}</dt>
              <dd class="truncate">{{ header.value }}</dd>
            </template>
          </dl>
        </div>
      </div>
    </AppModal>
  </div>
</template>
