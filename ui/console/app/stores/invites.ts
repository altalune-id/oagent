import type { Invite as InvitePB } from '~~/gen/invite/v1/invite_pb';
import type { ShownError } from '~/lib/apperror/message';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import { useErrorMessage } from '~/composables/useErrorMessage';

export interface Invite {
  id: string;
  email: string;
  role: string;
  status: string;
  invitedByName: string;
  expiresAt?: Date;
  createdAt?: Date;
}

export function toInvite(i: InvitePB): Invite {
  return {
    id: i.id,
    email: i.email,
    role: i.role,
    status: i.status,
    invitedByName: i.invitedByName,
    expiresAt: i.expiresAt ? timestampDate(i.expiresAt) : undefined,
    createdAt: i.createdAt ? timestampDate(i.createdAt) : undefined,
  };
}

// Every org id here is the org's public id.
export const useInvitesStore = defineStore('invites', () => {
  const pending = ref<Invite[]>([]);
  const drafts = ref<Invite[]>([]);
  const loading = ref(false);
  const error = ref<ShownError | null>(null);

  const { $inviteClient: client } = useNuxtApp();
  const errorMessage = useErrorMessage();
  let loadedFor = '';
  let latest = 0;

  function upsert(orgId: string, invite: Invite) {
    if (orgId !== loadedFor) {
      return;
    }
    drafts.value = drafts.value.filter(d => d.id !== invite.id);
    const at = pending.value.findIndex(p => p.id === invite.id);
    pending.value = at < 0 ? [...pending.value, invite] : pending.value.map(p => (p.id === invite.id ? invite : p));
  }

  function drop(orgId: string, id: string) {
    if (orgId === loadedFor) {
      pending.value = pending.value.filter(p => p.id !== id);
      drafts.value = drafts.value.filter(d => d.id !== id);
    }
  }

  async function load(orgId: string) {
    const mine = ++latest;
    if (orgId !== loadedFor) {
      pending.value = [];
      drafts.value = [];
      loadedFor = '';
    }
    loading.value = true;
    error.value = null;
    try {
      const res = await client.listInvites({ orgId });
      if (mine === latest) {
        pending.value = (res.invites ?? []).map(toInvite);
        drafts.value = (res.drafts ?? []).map(toInvite);
        loadedFor = orgId;
      }
    } catch (e) {
      if (mine === latest) {
        error.value = errorMessage(e, 'error.load_invites');
      }
    } finally {
      if (mine === latest) {
        loading.value = false;
      }
    }
  }

  return { pending, drafts, loading, error, upsert, drop, load };
});
