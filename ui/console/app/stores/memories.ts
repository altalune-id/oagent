import type { ShownError } from '#base/lib/apperror/message';
import type { Memory as MemoryPB } from '#gen/memory/v1/memory_pb';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import { useErrorMessage } from '#base/composables/useErrorMessage';
import { errorCode } from '#base/lib/apperror/message';

export interface Memory {
  id: string;
  text: string;
  agentId: string;
  createdAt?: Date;
  sourceRunId: string;
}

const pageSize = 50;

function toMemory(m: MemoryPB): Memory {
  return { id: m.id, text: m.text, agentId: m.agentId, createdAt: m.createdAt ? timestampDate(m.createdAt) : undefined, sourceRunId: m.sourceRunId };
}

// useMemoriesStore lists what agents remember about one person in one memory store, newest first, 50 at a time, and deletes or clears them.
export const useMemoriesStore = defineStore('memories', () => {
  const { $memoryClient: client } = useNuxtApp();
  const errorMessage = useErrorMessage();
  const storeId = ref('');
  const endUserId = ref('');
  const agentId = ref('');
  const items = ref<Memory[]>([]);
  const total = ref(0);
  const nextPageToken = ref('');
  const loading = ref(false);
  const error = ref<ShownError | null>(null);
  const actionError = ref<ShownError | null>(null);
  let latest = 0;

  const scope = () => ({ storeId: storeId.value, endUserId: endUserId.value, ...(agentId.value ? { agentId: agentId.value } : {}) });

  async function fetch(pageToken = '') {
    const mine = ++latest;
    loading.value = true;
    error.value = null;
    try {
      const res = await client.listMemories({ ...scope(), pageSize, ...(pageToken ? { pageToken } : {}) });
      if (mine === latest) {
        const page = res.memories.map(toMemory);
        items.value = pageToken ? [...items.value, ...page] : page;
        total.value = res.total;
        nextPageToken.value = res.nextPageToken;
      }
    } catch (e) {
      if (mine === latest) {
        error.value = errorMessage(e, 'error.load_memories');
      }
    } finally {
      if (mine === latest) {
        loading.value = false;
      }
    }
  }

  function load(store: string, person: string, agent = '') {
    storeId.value = store;
    endUserId.value = person;
    agentId.value = agent;
    actionError.value = null;
    return fetch();
  }

  function loadMore() {
    return nextPageToken.value ? fetch(nextPageToken.value) : Promise.resolve();
  }

  async function remove(id: string): Promise<'deleted' | 'gone' | false> {
    actionError.value = null;
    try {
      await client.deleteMemory({ storeId: storeId.value, endUserId: endUserId.value, id });
    } catch (e) {
      if (errorCode(e) !== 'MEM001') {
        actionError.value = errorMessage(e, 'error.delete_memory');
        return false;
      }
      await fetch();
      return 'gone';
    }
    items.value = items.value.filter(m => m.id !== id);
    total.value = Math.max(0, total.value - 1);
    return 'deleted';
  }

  async function clear(): Promise<number | null> {
    actionError.value = null;
    try {
      const res = await client.clearMemories(scope());
      items.value = [];
      total.value = 0;
      nextPageToken.value = '';
      return res.deleted;
    } catch (e) {
      actionError.value = errorMessage(e, 'error.clear_memories');
      return null;
    }
  }

  return { storeId, endUserId, agentId, items, total, nextPageToken, loading, error, actionError, load, loadMore, remove, clear };
});
