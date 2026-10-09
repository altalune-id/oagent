import { useOrgsStore } from '~/stores/orgs'
import { useUIStore } from '~/stores/ui'

export function useOrgCreate() {
  const name = ref('')
  const slug = ref('')
  const busy = ref(false)
  const error = ref('')

  const orgs = useOrgsStore()
  const ui = useUIStore()
  const { t } = useI18n()
  const { $orgClient: client } = useNuxtApp()

  const open = computed(() => ui.orgCreateOpen)

  function hide() {
    if (busy.value) {
      return
    }
    ui.closeOrgCreate()
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
        orgs.add({ id: org.id, publicId: org.publicId, slug: org.slug, name: org.name })
        ui.closeOrgCreate()
        name.value = ''
        slug.value = ''
        await navigateTo(`/o/${org.slug}`)
      }
    }
    catch (e) {
      error.value = e instanceof Error ? e.message : t('error.create_org')
    }
    finally {
      busy.value = false
    }
  }

  return { open, name, slug, busy, error, show: ui.openOrgCreate, hide, submit }
}
