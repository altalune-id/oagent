import type { ShownError } from '#base/lib/apperror/message';
import type { Party, Role } from './roles';
import type { Member } from '~/stores/members';
import { useErrorMessage } from '#base/composables/useErrorMessage';
import { useAuthStore } from '~/stores/auth';
import { toMember, useMembersStore } from '~/stores/members';
import { useOrgsStore } from '~/stores/orgs';
import { displayName, initials } from './names';
import { canRemove, roleOptions } from './roles';

export interface MemberRow extends Member {
  initials: string;
  self: boolean;
  roles: Role[];
  removable: boolean;
}

export type Confirming
  = | { kind: 'remove' | 'leave', userId: string, name: string }
    | { kind: 'role', userId: string, name: string, from: string, to: string };

export function useMembers(orgId: string) {
  const members = useMembersStore();
  const auth = useAuthStore();
  const orgs = useOrgsStore();
  const errorMessage = useErrorMessage();
  const { t } = useI18n();
  const { $memberClient: client } = useNuxtApp();

  const busy = ref(false);
  const error = ref<ShownError | null>(null);
  const confirming = ref<Confirming | null>(null);

  const viewer = computed<Party>(() => {
    const userId = auth.me?.userId ?? '';
    return { userId, role: (userId && members.byUser(userId)?.role) || '' };
  });
  const canLeave = computed(() => viewer.value.role !== '');
  const rows = computed<MemberRow[]>(() => members.items.map(m => ({
    ...m,
    initials: initials(m.name, m.email),
    self: viewer.value.userId !== '' && m.userId === viewer.value.userId,
    roles: roleOptions(viewer.value, m),
    removable: canRemove(viewer.value, m),
  })));
  const confirmMessage = computed(() => {
    const c = confirming.value;
    if (!c) {
      return '';
    }
    switch (c.kind) {
      case 'role':
        return t('members.role_confirm', { name: c.name, from: t(`members.role_${c.from}`), to: t(`members.role_${c.to}`) });
      case 'remove':
        return t('members.remove_confirm', { name: c.name });
      default:
        return t('members.leave_confirm');
    }
  });

  async function load() {
    await Promise.all([auth.load(), members.load(orgId)]);
  }

  function askRole(userId: string, role: string) {
    const m = members.byUser(userId);
    confirming.value = m && m.role !== role ? { kind: 'role', userId, name: displayName(m), from: m.role, to: role } : null;
  }

  async function setRole(userId: string, role: string) {
    const before = members.byUser(userId);
    if (!before) {
      return;
    }
    members.replace(orgId, { ...before, role });
    try {
      const res = await client.setMemberRole({ orgId, userId, role });
      if (res.member) {
        members.replace(orgId, toMember(res.member));
      }
    } catch (e) {
      members.replace(orgId, before);
      error.value = errorMessage(e, 'error.set_member_role');
    }
  }

  function askRemove(userId: string) {
    const m = members.byUser(userId);
    confirming.value = m ? { kind: 'remove', userId, name: displayName(m) } : null;
  }

  function askLeave() {
    const m = canLeave.value ? members.byUser(viewer.value.userId) : undefined;
    confirming.value = m ? { kind: 'leave', userId: m.userId, name: displayName(m) } : null;
  }

  function cancel() {
    confirming.value = null;
  }

  async function remove(userId: string) {
    try {
      await client.removeMember({ orgId, userId });
      members.drop(orgId, userId);
    } catch (e) {
      error.value = errorMessage(e, 'error.remove_member');
    }
  }

  async function leave() {
    try {
      await client.leaveOrg({ orgId });
    } catch (e) {
      error.value = errorMessage(e, 'error.leave_org');
      return;
    }
    await orgs.load();
    await navigateTo('/orgs');
  }

  async function confirm() {
    const c = confirming.value;
    if (!c || busy.value) {
      return;
    }
    confirming.value = null;
    busy.value = true;
    error.value = null;
    try {
      switch (c.kind) {
        case 'role':
          await setRole(c.userId, c.to);
          break;
        case 'remove':
          await remove(c.userId);
          break;
        default:
          await leave();
      }
    } finally {
      busy.value = false;
    }
  }

  return {
    rows,
    viewer,
    canLeave,
    loading: computed(() => members.loading),
    loadError: computed(() => members.error),
    busy,
    error,
    confirming,
    confirmMessage,
    load,
    askRole,
    askRemove,
    askLeave,
    cancel,
    confirm,
  };
}
