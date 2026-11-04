import type { ShownError } from '#base/lib/apperror/message';
import type { ThreadKind } from '~/stores/threads';
import { useErrorMessage } from '#base/composables/useErrorMessage';
import { decideApproval, statusAt } from '~/components/approval/decision';
import { forgetInApprovals } from '~/components/enduser/forget';
import { tick, useNow } from '~/composables/useNow';
import { sampleApprovals } from '~/preview/samples';
import { useEndUsersStore } from './endUsers';

export interface ApprovalArg {
  label: string;
  value: string;
  mono?: boolean;
  gate?: boolean;
}

export type ApprovalStatus = 'pending' | 'executing' | 'approved' | 'rejected' | 'expired';

export interface DecidedVia {
  channel: 'whatsapp' | 'console' | 'try';
  reply?: string;
}

export interface ApprovalRequest {
  id: string;
  tool: string;
  title: string;
  args: ApprovalArg[];
  raw: Record<string, unknown>;
  status: ApprovalStatus;
  requestedBy?: string;
  requestedById?: string;
  requesterDeleted?: boolean;
  expiresAt?: Date;
  decidedBy?: string;
  decidedById?: string;
  deciderDeleted?: boolean;
  decidedVia?: DecidedVia;
  decidedAt?: Date;
  reason?: string;
}

export interface ApprovalItem {
  approval: ApprovalRequest;
  agentName: string;
  agentSlug: string;
  threadId: string;
  channel: ThreadKind;
  askedAt: Date;
}

// TODO: replace with the approval RPCs; until then deciding here changes only this page.
export const useApprovalsStore = defineStore('approvals', () => {
  const items = ref<ApprovalItem[]>([]);
  const loading = ref(false);
  const error = ref<ShownError | null>(null);
  const errorMessage = useErrorMessage();
  const endUsers = useEndUsersStore();
  let loadedFor = '';

  function forget() {
    for (const p of endUsers.forgotten) {
      items.value = forgetInApprovals(items.value, p);
    }
  }
  watch(() => endUsers.forgotten, forget);

  const loadedProject = ref('');
  const now = useNow();
  const pending = computed(() => items.value.filter(i => ['pending', 'executing'].includes(statusAt(i.approval, now.value))));
  const decided = computed(() => items.value.filter(i => ['approved', 'rejected'].includes(statusAt(i.approval, now.value))));
  const expired = computed(() => items.value.filter(i => statusAt(i.approval, now.value) === 'expired'));

  async function load(projectId: string) {
    loading.value = true;
    error.value = null;
    try {
      items.value = await Promise.resolve().then(sampleApprovals);
      forget();
      loadedFor = projectId;
      loadedProject.value = projectId;
    } catch (e) {
      error.value = errorMessage(e, 'error.load_approvals');
    } finally {
      loading.value = false;
    }
  }

  function ensureLoaded(projectId: string) {
    return loadedFor === projectId ? Promise.resolve() : load(projectId);
  }

  async function decide(id: string, approve: boolean, by: string, reason = '', runMs = 900) {
    tick();
    const item = items.value.find(i => i.approval.id === id);
    const decided = item && decideApproval(item.approval, approve, by, { channel: 'console' }, reason);
    if (!item || !decided) {
      return false;
    }
    if (!approve) {
      item.approval = decided;
      return true;
    }
    item.approval = { ...item.approval, status: 'executing' };
    await new Promise(resolve => setTimeout(resolve, runMs));
    item.approval = decided;
    return true;
  }

  return { items, projectId: loadedProject, loading, error, pending, decided, expired, load, ensureLoaded, decide };
});
