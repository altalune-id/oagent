import type { ShownError } from '~/lib/apperror/message';
import { useErrorMessage } from '~/composables/useErrorMessage';

export interface Org {
  id: string;
  slug: string;
  name: string;
}

export const useOrgsStore = defineStore('orgs', () => {
  const items = ref<Org[]>([]);
  const loaded = ref(false);
  const loading = ref(false);
  const error = ref<ShownError | null>(null);

  const { $orgClient: client } = useNuxtApp();
  const errorMessage = useErrorMessage();
  let inflight: Promise<void> | null = null;

  function bySlug(slug: string) {
    return items.value.find(org => org.slug === slug);
  }

  function add(org: Org) {
    if (!bySlug(org.slug)) {
      items.value.push(org);
    }
  }

  function update(id: string, patch: { name: string, slug: string }) {
    items.value = items.value.map(org => (org.id === id ? { ...org, name: patch.name, slug: patch.slug } : org));
  }

  async function load() {
    loading.value = true;
    error.value = null;
    try {
      const res = await client.listOrgs({});
      items.value = (res.orgs ?? []).map(org => ({ id: org.id, slug: org.slug, name: org.name }));
      loaded.value = true;
    } catch (e) {
      error.value = errorMessage(e, 'error.load_orgs');
    } finally {
      loading.value = false;
    }
  }

  function ensureLoaded() {
    if (loaded.value) {
      return Promise.resolve();
    }
    inflight ??= load().finally(() => {
      inflight = null;
    });
    return inflight;
  }

  return { items, loaded, loading, error, bySlug, add, update, load, ensureLoaded };
});
