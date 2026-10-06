import { useOrgClient } from '~/composables/org/useOrgClient'
import { useOrgsStore } from '~/stores/orgs'

export function useOrgCreate() {
  const open = ref(false)
  const name = ref('')
  const slug = ref('')
  const busy = ref(false)
  const error = ref('')

  const orgs = useOrgsStore()
  const client = useOrgClient()

  function show() {
    error.value = ''
    open.value = true
  }

  function hide() {
    if (busy.value) {
      return
    }
    open.value = false
    error.value = ''
  }

  async function submit() {
    const value = name.value.trim()
    if (!value || busy.value) {
      return
    }
    busy.value = true
    error.value = ''
    try {
      const res = await client.createOrg({ name: value, slug: slug.value.trim() })
      const org = res.org
      if (org) {
        orgs.add({ slug: org.slug, name: org.name })
        open.value = false
        name.value = ''
        slug.value = ''
        await navigateTo(`/o/${org.slug}`)
      }
    }
    catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to create organization'
    }
    finally {
      busy.value = false
    }
  }

  return { open, name, slug, busy, error, show, hide, submit }
}
