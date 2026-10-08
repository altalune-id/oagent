import { useProjectClient } from '~/composables/project/useProjectClient'
import { useProjectsStore } from '~/stores/projects'

export function useProjectCreate(orgId: string) {
  const open = ref(false)
  const name = ref('')
  const slug = ref('')
  const busy = ref(false)
  const error = ref('')

  const projects = useProjectsStore()
  const client = useProjectClient()

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
      const res = await client.createProject({ orgId, name: value, slug: slug.value.trim() })
      const project = res.project
      if (project) {
        projects.add({ orgId: project.orgId, slug: project.slug, name: project.name })
        open.value = false
        name.value = ''
        slug.value = ''
      }
    }
    catch (e) {
      error.value = e instanceof Error ? e.message : 'Failed to create project'
    }
    finally {
      busy.value = false
    }
  }

  return { open, name, slug, busy, error, show, hide, submit }
}
