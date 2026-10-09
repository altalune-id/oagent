import { timestampFromDate } from '@bufbuild/protobuf/wkt'
import { toAPIKey, useAPIKeysStore } from '~/stores/apiKeys'

export function useAPIKeyCreate(orgId: string) {
  const open = ref(false)
  const kind = ref<'org' | 'project'>('org')
  const name = ref('')
  const scopes = ref<string[]>([])
  const allProjects = ref(true)
  const projectId = ref('')
  const projectIds = ref<string[]>([])
  const expiresInHours = ref(720)
  const busy = ref(false)
  const error = ref('')
  const plaintext = ref('')

  const keys = useAPIKeysStore()
  const { $apiKeyClient: client } = useNuxtApp()

  function show() {
    name.value = ''
    kind.value = 'org'
    scopes.value = []
    allProjects.value = true
    projectId.value = ''
    projectIds.value = []
    expiresInHours.value = 720
    error.value = ''
    plaintext.value = ''
    open.value = true
  }

  function hide() {
    if (busy.value) {
      return
    }
    open.value = false
    plaintext.value = ''
  }

  async function submit() {
    if (busy.value || !name.value.trim()) {
      return
    }
    busy.value = true
    error.value = ''
    try {
      const res = await client.createAPIKey({
        orgId,
        kind: kind.value,
        name: name.value.trim(),
        scopes: scopes.value,
        projectId: kind.value === 'project' ? projectId.value : '',
        allProjects: kind.value === 'org' ? allProjects.value : false,
        projectIds: kind.value === 'org' && !allProjects.value ? projectIds.value : [],
        expiresAt: timestampFromDate(new Date(Date.now() + expiresInHours.value * 3600_000)),
      })
      if (res.key) {
        keys.add(toAPIKey(res.key))
      }
      plaintext.value = res.plaintext
    }
    catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to create API key'
    }
    finally {
      busy.value = false
    }
  }

  return {
    open, kind, name, scopes, allProjects, projectId, projectIds, expiresInHours,
    busy, error, plaintext, show, hide, submit,
  }
}
