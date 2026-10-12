import { timestampDate } from '@bufbuild/protobuf/wkt'
import type { APIKey as APIKeyPB } from '~~/gen/apikey/v1/apikey_pb'
import type { ShownError } from '~/lib/apperror/message'
import { useErrorMessage } from '~/composables/useErrorMessage'

export interface APIKey {
  id: string
  kind: string
  name: string
  scopes: string[]
  projectId: string
  projectIds: string[]
  allProjects: boolean
  secretHint: string
  expiresAt?: Date
  revokedAt?: Date
}

export function toAPIKey(k: APIKeyPB): APIKey {
  return {
    id: k.id,
    kind: k.kind,
    name: k.name,
    scopes: [...k.scopes],
    projectId: k.projectId,
    projectIds: [...k.projectIds],
    allProjects: k.allProjects,
    secretHint: k.secretHint,
    expiresAt: k.expiresAt ? timestampDate(k.expiresAt) : undefined,
    revokedAt: k.revokedAt ? timestampDate(k.revokedAt) : undefined,
  }
}

export const useAPIKeysStore = defineStore('apiKeys', () => {
  const items = ref<APIKey[]>([])
  const loading = ref(false)
  const error = ref<ShownError | null>(null)

  const { $apiKeyClient: client } = useNuxtApp()
  const errorMessage = useErrorMessage()
  let loadedFor = ''
  let inflight: Promise<void> | null = null

  function add(key: APIKey) {
    items.value = [key, ...items.value.filter(k => k.id !== key.id)]
  }

  function markRevoked(id: string) {
    const at = new Date()
    items.value = items.value.map(k => (k.id === id ? { ...k, revokedAt: at } : k))
  }

  async function load(orgId: string) {
    loading.value = true
    error.value = null
    try {
      const res = await client.listAPIKeys({ orgId })
      items.value = (res.keys ?? []).map(toAPIKey)
      loadedFor = orgId
    }
    catch (e) {
      error.value = errorMessage(e, 'error.load_apikeys')
    }
    finally {
      loading.value = false
    }
  }

  function ensureLoaded(orgId: string) {
    if (!orgId || loadedFor === orgId) {
      return Promise.resolve()
    }
    inflight ??= load(orgId).finally(() => {
      inflight = null
    })
    return inflight
  }

  return { items, loading, error, add, markRevoked, load, ensureLoaded }
})
