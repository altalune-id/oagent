export interface Org {
  slug: string
  name: string
}

export const useOrgsStore = defineStore('orgs', () => {
  const items = ref<Org[]>([
    { slug: 'acme', name: 'Acme Corp' },
    { slug: 'globex', name: 'Globex' },
  ])

  function bySlug(slug: string) {
    return items.value.find(org => org.slug === slug)
  }

  return { items, bySlug }
})
