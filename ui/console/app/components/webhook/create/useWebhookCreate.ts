import { toWebhook, useWebhooksStore } from '~/stores/webhooks'

export function useWebhookCreate(orgId: string, projectId: string) {
  const open = ref(false)
  const url = ref('')
  const description = ref('')
  const eventTypes = ref<string[]>([])
  const busy = ref(false)
  const error = ref('')
  const secret = ref('')

  const webhooks = useWebhooksStore()
  const { $webhookClient: client } = useNuxtApp()
  const { t } = useI18n()

  function show() {
    url.value = ''
    description.value = ''
    eventTypes.value = []
    error.value = ''
    secret.value = ''
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
    error.value = ''
    try {
      const res = await client.createWebhook({ orgId, projectId, url: value, description: description.value, eventTypes: eventTypes.value })
      if (res.webhook) {
        webhooks.add(toWebhook(res.webhook))
      }
      secret.value = res.secret
    }
    catch (e) {
      error.value = e instanceof Error ? e.message : t('error.create_webhook')
    }
    finally {
      busy.value = false
    }
  }

  return { open, url, description, eventTypes, busy, error, secret, show, hide, submit }
}
