const STORAGE_KEY = 'oagent_recent';
const LEGACY_KEY = 'oagent_pinned';
const KEEP = 20;

function front(list: string[], slug: string): string[] {
  return [slug, ...list.filter(s => s !== slug)].slice(0, KEEP);
}

function slugs(value: unknown): string[] {
  return Array.isArray(value) ? value.filter((s): s is string => typeof s === 'string' && s !== '') : [];
}

function byOrg(value: unknown): Record<string, string[]> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return {};
  }
  return Object.fromEntries(Object.entries(value).map(([org, list]) => [org, slugs(list)]));
}

function prune(projects: Record<string, string[]>, orgs: string[]): Record<string, string[]> {
  return Object.fromEntries(Object.entries(projects).filter(([org]) => orgs.includes(org)));
}

// useRecentStore remembers the orgs and, per org, the projects a person opened last, newest first, in this browser.
export const useRecentStore = defineStore('recent', () => {
  const orgs = ref<string[]>([]);
  const projects = ref<Record<string, string[]>>({});

  function persist() {
    if (!import.meta.client) {
      return;
    }
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify({ orgs: orgs.value, projects: projects.value }));
    } catch {
    }
  }

  function migrate() {
    try {
      const legacy = (JSON.parse(localStorage.getItem(LEGACY_KEY) ?? '{}') ?? {}) as { org?: unknown, projects?: unknown };
      const raw = legacy.projects;
      const entries = raw && typeof raw === 'object' && !Array.isArray(raw) ? Object.entries(raw) : [];
      const pins = entries.filter((e): e is [string, string] => typeof e[1] === 'string' && e[1] !== '');
      orgs.value = [...new Set([...slugs([legacy.org]), ...pins.map(([org]) => org)])].slice(0, KEEP);
      projects.value = prune(Object.fromEntries(pins.map(([org, slug]) => [org, [slug]])), orgs.value);
    } catch {
    }
    persist();
    try {
      localStorage.removeItem(LEGACY_KEY);
    } catch {
    }
  }

  function read() {
    if (!import.meta.client) {
      return;
    }
    try {
      const raw = localStorage.getItem(STORAGE_KEY);
      if (raw === null) {
        migrate();
        return;
      }
      const saved = JSON.parse(raw) as { orgs?: unknown, projects?: unknown } | null;
      orgs.value = slugs(saved?.orgs).slice(0, KEEP);
      projects.value = prune(byOrg(saved?.projects), orgs.value);
    } catch {
    }
  }

  function visit(org: string, project?: string) {
    orgs.value = front(orgs.value, org);
    const next = project ? { ...projects.value, [org]: front(projects.value[org] ?? [], project) } : projects.value;
    projects.value = prune(next, orgs.value);
    persist();
  }

  function lastOrg() {
    return orgs.value[0] ?? '';
  }

  function lastProject(org: string) {
    return projects.value[org]?.[0] ?? '';
  }

  function byRecency<T>(items: T[], slug: (item: T) => string, order: string[]): T[] {
    const rank = (item: T) => {
      const i = order.indexOf(slug(item));
      return i === -1 ? order.length : i;
    };
    return [...items].sort((a, b) => rank(a) - rank(b));
  }

  read();

  return { orgs, projects, visit, lastOrg, lastProject, byRecency };
});
