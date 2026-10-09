import { useProjectsStore } from '~/stores/projects'
import { useUIStore } from '~/stores/ui'

export function useProjectCreate(orgId: string) {
  const name = ref('')
  const slug = ref('')
  const busy = ref(false)
  const error = ref('')

  const projects = useProjectsStore()
  const ui = useUIStore()
  const { t } = useI18n()
  const { $projectClient: client } = useNuxtApp()

  const open = computed(() => ui.projectCreateOpen)

  function hide() {
    if (busy.value) {
      return
    }
    ui.closeProjectCreate()
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
      const res = await client.createProject({ orgId, name: value, slug: slug.value.trim() })
      const project = res.project
      if (project) {
        projects.add({ id: project.id, publicId: project.publicId, orgId: project.orgId, slug: project.slug, name: project.name })
        ui.closeProjectCreate()
        name.value = ''
        slug.value = ''
      }
    }
    catch (e) {
      error.value = e instanceof Error ? e.message : t('error.create_project')
    }
    finally {
      busy.value = false
    }
  }

  return { open, name, slug, busy, error, show: ui.openProjectCreate, hide, submit }
}
