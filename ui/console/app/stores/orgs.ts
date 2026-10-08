import { useOrgClient } from '~/composables/org/useOrgClient'

export interface Org {
  id: string
  slug: string
  name: string
}

export const useOrgsStore = defineStore('orgs', () => {
  const items = ref<Org[]>([])
  const loaded = ref(false)
  const loading = ref(false)
  const error = ref('')

  const client = useOrgClient()
  let inflight: Promise<void> | null = null

  function bySlug(slug: string) {
    return items.value.find(org => org.slug === slug)
  }

  function add(org: Org) {
    if (!bySlug(org.slug)) {
      items.value.push(org)
    }
  }

  async function load() {
    loading.value = true
    error.value = ''
    try {
      const res = await client.listOrgs({})
      items.value = (res.orgs ?? []).map(org => ({ id: org.id, slug: org.slug, name: org.name }))
      loaded.value = true
    }
    catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to load organizations'
    }
    finally {
      loading.value = false
    }
  }

  function ensureLoaded() {
    if (loaded.value) {
      return Promise.resolve()
    }
    inflight ??= load().finally(() => {
      inflight = null
    })
    return inflight
  }

  return { items, loaded, loading, error, bySlug, add, load, ensureLoaded }
})
