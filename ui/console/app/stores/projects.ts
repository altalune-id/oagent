export interface Project {
  slug: string
  name: string
  org: string
}

export const useProjectsStore = defineStore('projects', () => {
  const items = ref<Project[]>([
    { org: 'acme', slug: 'support-bot', name: 'Support Bot' },
    { org: 'acme', slug: 'sales-copilot', name: 'Sales Copilot' },
    { org: 'globex', slug: 'onboarding', name: 'Onboarding Agent' },
  ])

  function forOrg(org: string) {
    return items.value.filter(project => project.org === org)
  }

  return { items, forOrg }
})
