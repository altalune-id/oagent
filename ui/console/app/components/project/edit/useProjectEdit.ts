import { useProjectsStore } from '~/stores/projects'
import type { Project } from '~/stores/projects'

export function useProjectEdit() {
  const open = ref(false)
  const id = ref('')
  const name = ref('')
  const slug = ref('')
  const busy = ref(false)
  const error = ref('')

  const projects = useProjectsStore()
  const { t } = useI18n()
  const { $projectClient: client } = useNuxtApp()

  function show(project: Project) {
    id.value = project.id
    name.value = project.name
    slug.value = project.slug
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
      const res = await client.updateProject({ id: id.value, name: value, slug: slug.value.trim() })
      if (res.project) {
        projects.update(res.project.id, { name: res.project.name, slug: res.project.slug })
      }
      open.value = false
      name.value = ''
      slug.value = ''
    }
    catch (e) {
      error.value = e instanceof Error ? e.message : t('error.rename_project')
    }
    finally {
      busy.value = false
    }
  }

  return { open, name, slug, busy, error, show, hide, submit }
}
