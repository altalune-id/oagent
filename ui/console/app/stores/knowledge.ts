import type { ShownError } from '#base/lib/apperror/message';
import { sampleKnowledgeSources } from '~/preview/samples';

export type KnowledgeStatus = 'queued' | 'crawling' | 'ready' | 'failed';

export interface KnowledgeSource {
  id: string;
  name: string;
  kind: 'website' | 'document';
  url?: string;
  status: KnowledgeStatus;
  chunks: number;
  pagesDone?: number;
  pagesTotal?: number;
  failure?: { reason: 'robots' | 'unreadable', path?: string };
  usedBy: string[];
  updatedAt: Date;
}

export const useKnowledgeStore = defineStore('knowledge', () => {
  const items = ref<KnowledgeSource[]>([]);
  const loading = ref(false);
  const error = ref<ShownError | null>(null);
  let loadedFor = '';

  async function load(projectId: string) {
    loading.value = true;
    error.value = null;
    items.value = await Promise.resolve(sampleKnowledgeSources());
    loadedFor = projectId;
    loading.value = false;
  }

  function ensureLoaded(projectId: string) {
    return loadedFor === projectId ? Promise.resolve() : load(projectId);
  }

  function addWebsite(url: string, maxPages: number) {
    const u = new URL(url);
    const source: KnowledgeSource = { id: `kns_${Date.now().toString(36)}`, name: `${u.host}${u.pathname.replace(/\/$/, '')}`, kind: 'website', url, status: 'queued', chunks: 0, pagesDone: 0, pagesTotal: maxPages, usedBy: [], updatedAt: new Date() };
    items.value = [source, ...items.value];
    return source;
  }

  function retry(id: string) {
    items.value = items.value.map(s => (s.id === id ? { ...s, status: 'queued', failure: undefined, pagesDone: 0, pagesTotal: s.pagesTotal ?? 50, updatedAt: new Date() } : s));
  }

  function remove(id: string) {
    items.value = items.value.filter(s => s.id !== id);
  }

  return { items, loading, error, load, ensureLoaded, addWebsite, retry, remove };
});
