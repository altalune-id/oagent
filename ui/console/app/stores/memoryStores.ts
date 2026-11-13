import type { ShownError } from '#base/lib/apperror/message';
import type { MemoryStore as MemoryStorePB } from '#gen/memory/v1/memory_pb';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import { useErrorMessage } from '#base/composables/useErrorMessage';

export interface MemoryStore {
  id: string;
  name: string;
  usedBy: { id: string, name: string }[];
  createdAt?: Date;
  rememberApiUsers: boolean;
}

export function toMemoryStore(s: MemoryStorePB): MemoryStore {
  return { id: s.id, name: s.name, usedBy: s.usedBy.map(u => ({ id: u.agentId, name: u.agentName })), createdAt: s.createdAt ? timestampDate(s.createdAt) : undefined, rememberApiUsers: s.rememberApiUsers };
}

// useMemoryStoresStore lists a project's memory stores, the agents using each, and whether this server has a memory backend on.
export const useMemoryStoresStore = defineStore('memoryStores', () => {
  const { $memoryClient: client } = useNuxtApp();
  const errorMessage = useErrorMessage();
  const items = ref<MemoryStore[]>([]);
  const on = ref(false);
  const loading = ref(false);
  const error = ref<ShownError | null>(null);
  let loadedFor = '';
  let latest = 0;

  async function load(projectId: string) {
    const mine = ++latest;
    loading.value = true;
    error.value = null;
    try {
      const res = await client.listMemoryStores({ projectId });
      if (mine === latest) {
        items.value = res.stores.map(toMemoryStore);
        on.value = res.enabledBackends.length > 0;
        loadedFor = projectId;
      }
    } catch (e) {
      if (mine === latest) {
        error.value = errorMessage(e, 'error.load_memory_stores');
      }
    } finally {
      if (mine === latest) {
        loading.value = false;
      }
    }
  }

  function ensureLoaded(projectId: string) {
    return loadedFor === projectId ? Promise.resolve() : load(projectId);
  }

  function add(s: MemoryStorePB) {
    items.value = [...items.value, toMemoryStore(s)];
  }

  async function remove(id: string) {
    await client.deleteMemoryStore({ id });
    items.value = items.value.filter(s => s.id !== id);
  }

  return { items, on, loading, error, load, ensureLoaded, add, remove };
});
