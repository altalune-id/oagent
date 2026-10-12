import { toWebhook, useWebhooksStore } from '~/stores/webhooks'
import type { ShownError } from '~/lib/apperror/message'
import { useErrorMessage } from '~/composables/useErrorMessage'

export function useWebhookCreate(orgId: string, projectId: string) {
  const open = ref(false)
  const url = ref('')
  const description = ref('')
  const eventTypes = ref<string[]>([])
  const busy = ref(false)
  const error = ref<ShownError | null>(null)
  const secret = ref('')
  const id = ref('')
  const pingBusy = ref(false)
  const pingError = ref<ShownError | null>(null)
  const pinged = ref(false)

  const webhooks = useWebhooksStore()
  const { $webhookClient: client } = useNuxtApp()
  const errorMessage = useErrorMessage()

  function show() {
    url.value = ''
    description.value = ''
    eventTypes.value = []
    error.value = null
    secret.value = ''
    id.value = ''
    pingError.value = null
    pinged.value = false
    open.value = true
  }

  function hide() {
    if (busy.value) {
      return
    }
    open.value = false
    secret.value = ''
  }

  async function submit() {
    const value = url.value.trim()
    if (busy.value || !value || eventTypes.value.length === 0) {
      return
    }
    busy.value = true
    error.value = null
    try {
      const res = await client.createWebhook({ orgId, projectId, url: value, description: description.value, eventTypes: eventTypes.value })
      if (res.webhook) {
        webhooks.add(toWebhook(res.webhook))
        id.value = res.webhook.id
      }
      secret.value = res.secret
    }
    catch (e) {
      error.value = errorMessage(e, 'error.create_webhook')
    }
    finally {
      busy.value = false
    }
  }

  async function ping() {
    if (!id.value || pingBusy.value) {
      return
    }
    pingBusy.value = true
    pingError.value = null
    try {
      await client.pingWebhook({ orgId, projectId, id: id.value })
      pinged.value = true
    }
    catch (e) {
      pingError.value = errorMessage(e, 'error.ping_webhook')
    }
    finally {
      pingBusy.value = false
    }
  }

  return { open, url, description, eventTypes, busy, error, secret, pingBusy, pingError, pinged, show, hide, submit, ping }
}
