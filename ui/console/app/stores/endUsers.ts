import type { ShownError } from '#base/lib/apperror/message';
import type { ForgottenPerson } from '~/components/enduser/forget';
import type { ThreadKind } from '~/stores/threads';
import { useErrorMessage } from '#base/composables/useErrorMessage';
import { forgottenFrom } from '~/components/enduser/forget';
import { sampleEndUsers } from '~/preview/samples';

export type IdentityKind = 'phone' | 'lid' | 'api' | 'visitor';

export interface EndUserIdentity {
  kind: IdentityKind;
  value: string;
  linkedAt: Date;
}

export interface EndUserThread {
  id: string;
  title: string;
  kind: ThreadKind;
  agentSlug: string;
  agentName: string;
  updatedAt: Date;
}

export interface Memory {
  id: string;
  text: string;
  threadId: string;
  agentSlug: string;
  learnedAt: Date;
}

export interface EndUser {
  id: string;
  name: string;
  identities: EndUserIdentity[];
  threads: EndUserThread[];
  memories: Memory[];
  lastSeenAt: Date;
}

export interface EndUserList {
  items: EndUser[];
  memoryOn: boolean;
}

// TODO: replace with the end user RPCs; until then a delete lasts only until the page reloads.
export const useEndUsersStore = defineStore('endUsers', () => {
  const items = ref<EndUser[]>([]);
  const memoryOn = ref(false);
  const forgotten = ref<ForgottenPerson[]>([]);
  const loading = ref(false);
  const error = ref<ShownError | null>(null);
  const errorMessage = useErrorMessage();
  let loadedFor = '';

  async function load(projectId: string) {
    loading.value = true;
    error.value = null;
    try {
      const list = await Promise.resolve().then(sampleEndUsers);
      items.value = list.items.filter(u => !forgotten.value.some(f => f.id === u.id));
      memoryOn.value = list.memoryOn;
      loadedFor = projectId;
    } catch (e) {
      error.value = errorMessage(e, 'error.load_end_users');
    } finally {
      loading.value = false;
    }
  }

  function ensureLoaded(projectId: string) {
    return loadedFor === projectId ? Promise.resolve() : load(projectId);
  }

  function byId(id: string) {
    return items.value.find(u => u.id === id);
  }

  function forgetMemory(userId: string, memoryId: string) {
    const user = byId(userId);
    if (user) {
      user.memories = user.memories.filter(m => m.id !== memoryId);
    }
  }

  function remove(userId: string) {
    const user = byId(userId);
    if (!user) {
      return;
    }
    forgotten.value = [...forgotten.value, forgottenFrom(user)];
    items.value = items.value.filter(u => u.id !== userId);
  }

  return { items, memoryOn, forgotten, loading, error, load, ensureLoaded, byId, forgetMemory, remove };
});
