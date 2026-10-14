export const usePinnedStore = defineStore('pinned', () => {
  const orgSlug = ref('');
  const projects = ref<Record<string, string>>({});

  const STORAGE_KEY = 'oagent_pinned';

  function read() {
    if (!import.meta.client) {
      return;
    }
    try {
      const saved = JSON.parse(localStorage.getItem(STORAGE_KEY) ?? '{}') as { org?: string, projects?: Record<string, string> };
      if (saved.org) {
        orgSlug.value = saved.org;
      }
      if (saved.projects) {
        projects.value = saved.projects;
      }
    } catch {
    }
  }

  function persist() {
    if (!import.meta.client) {
      return;
    }
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ org: orgSlug.value, projects: projects.value }));
  }

  function setOrg(slug: string) {
    orgSlug.value = slug;
    persist();
  }

  function setProject(org: string, project: string) {
    projects.value = { ...projects.value, [org]: project };
    orgSlug.value = org;
    persist();
  }

  function unpinOrg() {
    orgSlug.value = '';
    persist();
  }

  function unpinProject(org: string) {
    const next = { ...projects.value };
    delete next[org];
    projects.value = next;
    persist();
  }

  function projectFor(org: string) {
    return projects.value[org] ?? '';
  }

  read();

  return { orgSlug, setOrg, setProject, unpinOrg, unpinProject, projectFor };
});
