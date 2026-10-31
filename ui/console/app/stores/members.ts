import type { ShownError } from '#base/lib/apperror/message';
import type { Member as MemberPB } from '#gen/member/v1/member_pb';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import { useErrorMessage } from '#base/composables/useErrorMessage';

export interface Member {
  userId: string;
  email: string;
  name: string;
  role: string;
  joinedAt?: Date;
}

export function toMember(m: MemberPB): Member {
  return {
    userId: m.userId,
    email: m.email,
    name: m.name,
    role: m.role,
    joinedAt: m.joinedAt ? timestampDate(m.joinedAt) : undefined,
  };
}

export const useMembersStore = defineStore('members', () => {
  const items = ref<Member[]>([]);
  const loading = ref(false);
  const error = ref<ShownError | null>(null);

  const { $memberClient: client } = useNuxtApp();
  const errorMessage = useErrorMessage();
  let loadedFor = '';
  let latest = 0;

  function byUser(userId: string) {
    return items.value.find(m => m.userId === userId);
  }

  function replace(orgId: string, member: Member) {
    if (orgId === loadedFor) {
      items.value = items.value.map(m => (m.userId === member.userId ? member : m));
    }
  }

  function drop(orgId: string, userId: string) {
    if (orgId === loadedFor) {
      items.value = items.value.filter(m => m.userId !== userId);
    }
  }

  async function load(orgId: string) {
    const mine = ++latest;
    loading.value = true;
    error.value = null;
    try {
      const res = await client.listMembers({ orgId });
      if (mine === latest) {
        items.value = (res.members ?? []).map(toMember);
        loadedFor = orgId;
      }
    } catch (e) {
      if (mine === latest) {
        error.value = errorMessage(e, 'error.load_members');
      }
    } finally {
      if (mine === latest) {
        loading.value = false;
      }
    }
  }

  return { items, loading, error, byUser, replace, drop, load };
});
