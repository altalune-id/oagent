import type { ShownError } from '#base/lib/apperror/message';
import { useErrorMessage } from '#base/composables/useErrorMessage';

export interface Project {
  id: string;
  orgId: string;
  slug: string;
  name: string;
}

export const useProjectsStore = defineStore('projects', () => {
  const items = ref<Project[]>([]);
  const loading = ref(false);
  const error = ref<ShownError | null>(null);

  const { $projectClient: client } = useNuxtApp();
  const errorMessage = useErrorMessage();
  let loadedFor = '';
  let inflight: Promise<void> | null = null;
  let inflightFor = '';
  let latest = 0;

  function forOrg(orgId: string) {
    return items.value.filter(project => project.orgId === orgId);
  }

  function add(project: Project) {
    if (!items.value.some(p => p.orgId === project.orgId && p.slug === project.slug)) {
      items.value.push(project);
    }
  }

  function update(id: string, patch: { name: string, slug: string }) {
    items.value = items.value.map(p => (p.id === id ? { ...p, name: patch.name, slug: patch.slug } : p));
  }

  async function load(orgId: string) {
    const mine = ++latest;
    loading.value = true;
    error.value = null;
    try {
      const res = await client.listProjects({ orgId });
      if (mine !== latest) {
        return;
      }
      items.value = (res.projects ?? []).map(p => ({ id: p.id, orgId: p.orgId, slug: p.slug, name: p.name }));
      loadedFor = orgId;
    } catch (e) {
      if (mine === latest) {
        error.value = errorMessage(e, 'error.load_projects');
      }
    } finally {
      if (mine === latest) {
        loading.value = false;
      }
    }
  }

  function ensureLoaded(orgId: string) {
    if (!orgId || loadedFor === orgId) {
      return Promise.resolve();
    }
    if (inflight && inflightFor === orgId) {
      return inflight;
    }
    inflightFor = orgId;
    const run = load(orgId).finally(() => {
      if (inflight === run) {
        inflight = null;
      }
    });
    inflight = run;
    return run;
  }

  return { items, loading, error, forOrg, add, update, load, ensureLoaded };
});
