import { useOrgsStore } from '~/stores/orgs'
import { useUIStore } from '~/stores/ui'
import type { ShownError } from '~/lib/apperror/message'
import { useErrorMessage } from '~/composables/useErrorMessage'

export function useOrgCreate() {
  const name = ref('')
  const slug = ref('')
  const busy = ref(false)
  const error = ref<ShownError | null>(null)

  const orgs = useOrgsStore()
  const ui = useUIStore()
  const errorMessage = useErrorMessage()
  const { $orgClient: client } = useNuxtApp()

  const open = computed(() => ui.orgCreateOpen)

  function hide() {
    if (busy.value) {
      return
    }
    ui.closeOrgCreate()
    error.value = null
  }

  async function submit() {
    const value = name.value.trim()
    if (!value || busy.value) {
      return
    }
    busy.value = true
    error.value = null
    try {
      const res = await client.createOrg({ name: value, slug: slug.value.trim() })
      const org = res.org
      if (org) {
        orgs.add({ id: org.id, slug: org.slug, name: org.name })
        ui.closeOrgCreate()
        name.value = ''
        slug.value = ''
        await navigateTo(`/o/${org.slug}`)
      }
    }
    catch (e) {
      error.value = errorMessage(e, 'error.create_org')
    }
    finally {
      busy.value = false
    }
  }

  return { open, name, slug, busy, error, show: ui.openOrgCreate, hide, submit }
}
