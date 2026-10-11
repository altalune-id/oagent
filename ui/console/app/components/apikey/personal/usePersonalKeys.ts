import { timestampFromDate } from '@bufbuild/protobuf/wkt'
import scopes from '@shared/scope/scopes.json'
import { useScopeLabel } from '~/components/apikey/useScopeLabel'
import { mcpURL } from '~/lib/auth/urls'
import { toAPIKey, useAPIKeysStore } from '~/stores/apiKeys'
import { useAuthStore } from '~/stores/auth'
import { useProjectsStore } from '~/stores/projects'
import { mintableForMe } from './grants'
import { mcpSnippet } from './mcpSnippet'
import type { PersonalKeyForm } from './PersonalKeys.vue'

// The server already hides other people's personal keys, so every personal key in the store is the viewer's.
export function usePersonalKeys(orgId: string) {
  const keys = useAPIKeysStore()
  const auth = useAuthStore()
  const projects = useProjectsStore()
  const scopeLabel = useScopeLabel()
  const { t } = useI18n()
  const { $apiKeyClient: client } = useNuxtApp()
  const apiUrl = useRuntimeConfig().public.apiUrl

  const busy = ref(false)
  const error = ref('')
  const created = ref<{ plaintext: string, snippet: string } | null>(null)

  const mine = computed(() => keys.items.filter(k => k.kind === 'personal').map(k => ({ ...k, scopes: k.scopes.map(scopeLabel) })))
  const mintable = computed(() => mintableForMe(scopes, auth.me?.permissions ?? []).map(s => ({ name: s.name, label: scopeLabel(s.name) })))
  const orgProjects = computed(() => projects.forOrg(orgId).map(p => ({ id: p.id, name: p.name })))
  const signedIn = computed(() => auth.me?.source === 'local')

  async function create(form: PersonalKeyForm) {
    busy.value = true
    error.value = ''
    try {
      const res = await client.createAPIKey({
        orgId,
        kind: 'personal',
        name: form.name,
        scopes: form.scopes,
        allProjects: form.allProjects,
        projectIds: form.allProjects ? [] : form.projectIds,
        expiresAt: timestampFromDate(new Date(Date.now() + form.expiresInHours * 3600_000)),
      })
      if (res.key) {
        keys.add(toAPIKey(res.key))
      }
      created.value = { plaintext: res.plaintext, snippet: mcpSnippet('oagent', mcpURL(apiUrl, window.location.origin), res.plaintext) }
    }
    catch (e) {
      error.value = e instanceof Error ? e.message : t('error.create_apikey')
    }
    finally {
      busy.value = false
    }
  }

  async function revoke(id: string) {
    error.value = ''
    try {
      await client.revokeAPIKey({ orgId, id })
      keys.markRevoked(id)
    }
    catch (e) {
      error.value = e instanceof Error ? e.message : t('error.revoke_apikey')
    }
  }

  function dismiss() {
    created.value = null
  }

  async function load() {
    await Promise.all([auth.load(), keys.ensureLoaded(orgId), projects.ensureLoaded(orgId)])
  }

  return { mine, mintable, orgProjects, signedIn, busy, error, created, create, revoke, dismiss, load }
}
