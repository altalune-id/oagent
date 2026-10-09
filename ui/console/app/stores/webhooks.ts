import type { Webhook as WebhookPB } from '~~/gen/webhook/v1/webhook_pb'

export interface Webhook {
  id: string
  orgId: string
  projectId: string
  url: string
  description: string
  eventTypes: string[]
  active: boolean
}

export function toWebhook(w: WebhookPB): Webhook {
  return {
    id: w.id,
    orgId: w.orgId,
    projectId: w.projectId,
    url: w.url,
    description: w.description,
    eventTypes: [...w.eventTypes],
    active: w.active,
  }
}

export const useWebhooksStore = defineStore('webhooks', () => {
  const items = ref<Webhook[]>([])
  const loading = ref(false)
  const error = ref('')

  const { $webhookClient: client } = useNuxtApp()
  const { t } = useI18n()
  let loadedFor = ''
  let inflight: Promise<void> | null = null

  function forProject(projectId: string) {
    return items.value.filter(w => w.projectId === projectId)
  }

  function add(webhook: Webhook) {
    if (!items.value.some(w => w.id === webhook.id)) {
      items.value.push(webhook)
    }
  }

  function replace(webhook: Webhook) {
    items.value = items.value.map(w => (w.id === webhook.id ? webhook : w))
  }

  function remove(id: string) {
    items.value = items.value.filter(w => w.id !== id)
  }

  async function load(orgId: string, projectId: string) {
    loading.value = true
    error.value = ''
    try {
      const res = await client.listWebhooks({ orgId, projectId })
      items.value = (res.webhooks ?? []).map(toWebhook)
      loadedFor = `${orgId}:${projectId}`
    }
    catch (e) {
      error.value = e instanceof Error ? e.message : t('error.load_webhooks')
    }
    finally {
      loading.value = false
    }
  }

  function ensureLoaded(orgId: string, projectId: string) {
    if (!orgId || !projectId || loadedFor === `${orgId}:${projectId}`) {
      return Promise.resolve()
    }
    inflight ??= load(orgId, projectId).finally(() => {
      inflight = null
    })
    return inflight
  }

  return { items, loading, error, forProject, add, replace, remove, load, ensureLoaded }
})
