import type { ShownError } from '#base/lib/apperror/message';
import type { Approval } from '#gen/approval/v1/approval_pb';
import { useErrorMessage } from '#base/composables/useErrorMessage';
import { errorCode, errorMeta } from '#base/lib/apperror/message';
import { statusAt } from '~/components/approval/decision';
import { toApprovalItem } from '~/components/approval/fromWire';
import { useNow } from '~/composables/useNow';
import { useAgentsStore } from '~/stores/agents';

export interface ApprovalArg {
  label: string;
  value: string;
  mono?: boolean;
  gate?: boolean;
}

export interface ApprovalCall {
  title: string;
  args: ApprovalArg[];
}

export type ApprovalStatus = 'pending' | 'approved' | 'rejected' | 'expired';
export type ApprovalChannel = 'whatsapp' | 'api' | 'console';
export type ApprovalTab = 'pending' | 'decided' | 'expired';

export interface DecidedVia {
  channel: 'whatsapp' | 'console' | 'mcp' | 'api' | 'try';
  reply?: string;
}

export interface ApprovalRequest {
  id: string;
  tool: string;
  title: string;
  args: ApprovalArg[];
  calls?: ApprovalCall[];
  more?: boolean;
  preview?: string;
  raw?: Record<string, unknown>;
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
  agentId: string;
  agentName: string;
  agentSlug?: string;
  threadId: string;
  runId: string;
  channel: ApprovalChannel;
  askedAt: Date;
}

export type DecideOutcome = 'decided' | 'elsewhere' | 'expired' | 'gone' | 'app' | 'error';

const pageSize = 50;
const tabStatus: Record<ApprovalTab, string> = { pending: 'pending', decided: 'decided', expired: 'expired' };

// useApprovalsStore lists a project's approvals per tab from ApprovalService and decides them; the pending count feeds the sidebar.
export const useApprovalsStore = defineStore('approvals', () => {
  const { $approvalClient: client } = useNuxtApp();
  const agents = useAgentsStore();
  const errorMessage = useErrorMessage();
  const now = useNow();
  const lists = ref<Record<ApprovalTab, ApprovalItem[]>>({ pending: [], decided: [], expired: [] });
  const tokens = ref<Record<ApprovalTab, string>>({ pending: '', decided: '', expired: '' });
  const loadedTabs = new Set<ApprovalTab>();
  const loading = ref(false);
  const error = ref<ShownError | null>(null);
  const projectId = ref('');
  const pendingCount = ref(0);
  const countProject = ref('');
  const noted = ref<Set<string>>(new Set());

  const pending = computed(() => lists.value.pending.filter(i => noted.value.has(i.approval.id) || statusAt(i.approval, now.value) === 'pending'));
  const decided = computed(() => lists.value.decided);
  const expired = computed(() => {
    const listed = new Set(lists.value.expired.map(i => i.approval.id));
    return [...lists.value.pending.filter(i => !listed.has(i.approval.id) && !noted.value.has(i.approval.id) && statusAt(i.approval, now.value) === 'expired'), ...lists.value.expired];
  });
  const more = computed(() => ({ pending: !!tokens.value.pending, decided: !!tokens.value.decided, expired: !!tokens.value.expired }));

  function item(a: Approval) {
    return toApprovalItem(a, (id) => {
      const agent = agents.items.find(x => x.id === id);
      return agent && { name: agent.name, slug: agent.slug };
    });
  }

  async function page(tab: ApprovalTab, pageToken: string) {
    const res = await client.listApprovals({ projectId: projectId.value, status: tabStatus[tab], pageToken, pageSize });
    pendingCount.value = Number(res.pendingCount);
    countProject.value = projectId.value;
    tokens.value[tab] = res.nextPageToken;
    return res.approvals.map(item);
  }

  async function load(project: string, tab: ApprovalTab) {
    if (projectId.value !== project) {
      projectId.value = project;
      loadedTabs.clear();
      lists.value = { pending: [], decided: [], expired: [] };
      noted.value = new Set();
    }
    loading.value = true;
    error.value = null;
    try {
      await agents.ensureLoaded(project);
      lists.value[tab] = await page(tab, '');
      loadedTabs.add(tab);
    } catch (e) {
      error.value = errorMessage(e, 'error.load_approvals');
    } finally {
      loading.value = false;
    }
  }

  function ensureLoaded(project: string, tab: ApprovalTab) {
    return projectId.value === project && loadedTabs.has(tab) ? Promise.resolve() : load(project, tab);
  }

  async function loadMore(tab: ApprovalTab) {
    if (!tokens.value[tab]) {
      return;
    }
    loading.value = true;
    try {
      const next = await page(tab, tokens.value[tab]);
      const seen = new Set(lists.value[tab].map(i => i.approval.id));
      lists.value[tab].push(...next.filter(i => !seen.has(i.approval.id)));
    } catch (e) {
      error.value = errorMessage(e, 'error.load_approvals');
    } finally {
      loading.value = false;
    }
  }

  // loadCount reads only the pending count for the sidebar; a failure keeps the last count.
  async function loadCount(project: string) {
    countProject.value = project;
    try {
      const res = await client.listApprovals({ projectId: project, status: 'pending', pageToken: '', pageSize: 1 });
      pendingCount.value = Number(res.pendingCount);
    } catch {}
  }

  // refreshPending merges the first pending page in place: new items go on top, items decided elsewhere leave, and kept ids stay untouched. It returns how many are new.
  async function refreshPending(keep: Set<string>) {
    if (!projectId.value) {
      return 0;
    }
    let fresh: ApprovalItem[];
    try {
      const res = await client.listApprovals({ projectId: projectId.value, status: 'pending', pageToken: '', pageSize });
      pendingCount.value = Number(res.pendingCount);
      countProject.value = projectId.value;
      fresh = res.approvals.map(item);
    } catch {
      return 0;
    }
    const old = new Map(lists.value.pending.map(i => [i.approval.id, i]));
    const ids = new Set(fresh.map(i => i.approval.id));
    const added = fresh.filter(i => !old.has(i.approval.id));
    const stays = lists.value.pending.filter(i => ids.has(i.approval.id) || keep.has(i.approval.id));
    noted.value = new Set([...noted.value].filter(id => keep.has(id)));
    lists.value.pending = [...added, ...stays];
    return added.length;
  }

  // note keeps a card that someone else settled in its place on the Pending tab, showing the outcome, until it is dismissed or refreshed away.
  function note(id: string, next: ApprovalItem, to: ApprovalTab) {
    lists.value.pending = lists.value.pending.map(i => (i.approval.id === id ? next : i));
    lists.value[to] = [next, ...lists.value[to].filter(i => i.approval.id !== id)];
    noted.value = new Set([...noted.value, id]);
    pendingCount.value = Math.max(0, pendingCount.value - 1);
  }

  function dismiss(id: string) {
    noted.value = new Set([...noted.value].filter(x => x !== id));
    lists.value.pending = lists.value.pending.filter(i => i.approval.id !== id);
  }

  function move(id: string, next: ApprovalItem | undefined, to: ApprovalTab | null) {
    lists.value.pending = lists.value.pending.filter(i => i.approval.id !== id);
    if (next && to) {
      lists.value[to] = [next, ...lists.value[to].filter(i => i.approval.id !== id)];
    }
  }

  // decide sends one decision; when someone else got there first it shows their outcome instead.
  async function decide(id: string, approve: boolean, reason = ''): Promise<{ outcome: DecideOutcome, error?: ShownError }> {
    const current = lists.value.pending.find(i => i.approval.id === id);
    if (!current) {
      return { outcome: 'gone' };
    }
    try {
      const res = await client.decideApproval({ id, approved: approve, text: reason.trim() });
      move(id, res.approval ? item(res.approval) : undefined, 'decided');
      pendingCount.value = Math.max(0, pendingCount.value - 1);
      return { outcome: 'decided' };
    } catch (e) {
      const code = errorCode(e);
      if (code === 'APR003') {
        if (errorMeta(e).status === 'expired') {
          note(id, { ...current, approval: { ...current.approval, status: 'expired' } }, 'expired');
          return { outcome: 'expired' };
        }
        const got = await client.getApproval({ id }).catch(() => undefined);
        if (got?.approval) {
          note(id, item(got.approval), 'decided');
        } else {
          move(id, undefined, null);
        }
        return { outcome: 'elsewhere' };
      }
      if (code === 'APR002') {
        move(id, undefined, null);
        return { outcome: 'gone' };
      }
      if (code === 'APR006') {
        return { outcome: 'app', error: errorMessage(e, 'error.decide_approval') };
      }
      return { outcome: 'error', error: errorMessage(e, 'error.decide_approval') };
    }
  }

  return { lists, projectId, countProject, loading, error, pending, decided, expired, more, pendingCount, noted, load, ensureLoaded, loadMore, loadCount, refreshPending, decide, dismiss };
});
