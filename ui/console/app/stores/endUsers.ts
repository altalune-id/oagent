import type { Timestamp } from '@bufbuild/protobuf/wkt';
import type { ShownError } from '#base/lib/apperror/message';
import type { EndUser as EndUserPB, EndUserThread as EndUserThreadPB, IdentityKind } from '#gen/enduser/v1/enduser_pb';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import { useErrorMessage } from '#base/composables/useErrorMessage';
import { errorCode } from '#base/lib/apperror/message';

export interface EndUser {
  id: string;
  displayName: string;
  identityKind: IdentityKind;
  identityHint: string;
  firstSeenAt?: Date;
  lastSeenAt?: Date;
  threadCount: number;
}

export interface EndUserThread {
  id: string;
  agentId: string;
  kind: string;
  lastActivityAt?: Date;
}

export interface EndUserDetail {
  user: EndUser;
  threads: EndUserThread[];
}

const date = (t?: Timestamp) => (t ? timestampDate(t) : undefined);

function toEndUser(u: EndUserPB): EndUser {
  return { id: u.id, displayName: u.displayName, identityKind: u.identityKind, identityHint: u.identityHint, firstSeenAt: date(u.firstSeenAt), lastSeenAt: date(u.lastSeenAt), threadCount: Number(u.threadCount) };
}

function toThread(th: EndUserThreadPB): EndUserThread {
  return { id: th.id, agentId: th.agentId, kind: th.kind, lastActivityAt: date(th.lastActivityAt) };
}

const pageSize = 200;

// useEndUsersStore reads a project's end users through EndUserService; it is read-only, and the name search runs on the server.
export const useEndUsersStore = defineStore('endUsers', () => {
  const { $endUserClient: client } = useNuxtApp();
  const errorMessage = useErrorMessage();
  const items = ref<EndUser[]>([]);
  const loading = ref(false);
  const error = ref<ShownError | null>(null);
  const detail = ref<EndUserDetail | null>(null);
  const detailLoading = ref(false);
  const detailError = ref<ShownError | null>(null);
  let loadedFor = '';
  let latest = 0;

  // shortcut: the first page only (200, the API maximum); add paging when a project outgrows it.
  async function load(projectId: string, query = '') {
    const mine = ++latest;
    loading.value = true;
    error.value = null;
    try {
      const res = await client.listEndUsers({ projectId, query: query.trim(), pageSize });
      if (mine === latest) {
        items.value = res.endUsers.map(toEndUser);
        loadedFor = projectId;
      }
    } catch (e) {
      if (mine === latest) {
        error.value = errorMessage(e, 'error.load_end_users');
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

  async function loadOne(id: string) {
    detailLoading.value = true;
    detailError.value = null;
    try {
      const res = await client.getEndUser({ id });
      detail.value = res.endUser ? { user: toEndUser(res.endUser), threads: res.threads.map(toThread) } : null;
    } catch (e) {
      detail.value = null;
      detailError.value = errorCode(e) === 'EUS001' ? null : errorMessage(e, 'error.load_end_users');
    } finally {
      detailLoading.value = false;
    }
  }

  return { items, loading, error, load, ensureLoaded, detail, detailLoading, detailError, loadOne };
});
