import type { ShownError } from '#base/lib/apperror/message';
import type { ApprovalRequest } from '~/stores/approvals';
import { useErrorMessage } from '#base/composables/useErrorMessage';
import { forgetInThreads } from '~/components/enduser/forget';
import { toItems, toRunSummary, toThread } from '~/components/thread/fromWire';
import { sampleRun, sampleRuns, sampleThreads } from '~/preview/samples';
import { useAgentsStore } from './agents';
import { useEndUsersStore } from './endUsers';

export const pollIntervalMs = 2_000;
const pageSize = 200;

export type RunEventType = 'input' | 'model' | 'tool' | 'approval' | 'error' | 'output';

export interface RunEvent {
  id: string;
  type: RunEventType;
  name: string;
  labelKey?: string;
  at: number;
  took: number;
  costCents?: number;
  tokens?: number;
  summary: string;
  input?: string;
  output?: string;
}

export interface RunDetail {
  id: string;
  agentName: string;
  version: number;
  model: string;
  price: { input: number, output: number };
  startedBy: string;
  threadId: string;
  startedAt: Date;
  maxSteps: number;
  approval: ApprovalRequest;
  events: RunEvent[];
  afterApprove: RunEvent[];
  afterReject: RunEvent[];
}

export type RunStatus = 'running' | 'awaiting_approval' | 'completed' | 'failed' | 'cancelled' | 'interrupted' | 'limit_reached' | 'expired';

export interface RunSummary {
  id: string;
  threadId: string;
  threadTitle: string;
  status: RunStatus;
  detail?: { key: string, params?: Record<string, string> };
  personId?: string;
  personDeleted?: boolean;
  startedAt: Date;
  seconds: number;
  tokens: number;
  costCents: number;
  unpriced?: boolean;
  estimated?: boolean;
}

export type ToolState = 'running' | 'done' | 'preview' | 'failed' | 'awaiting' | 'rejected';

export interface Photo {
  name: string;
  url: string;
}

export type TranscriptItem
  = | { id: number, kind: 'user', text: string, speaker?: string, speakerId?: string, photo?: Photo, photoPlaceholder?: boolean, deleted?: boolean, at: Date }
    | { id: number, kind: 'agent', text: string, streaming: boolean, failed?: boolean, at: Date }
    | { id: number, kind: 'tool', name: string, args: Record<string, unknown>, result?: unknown, state: ToolState }
    | { id: number, kind: 'approval', approval: ApprovalRequest }
    | { id: number, kind: 'notice', key: string, params?: Record<string, string> }
    | { id: number, kind: 'run', run: RunSummary };

export type ThreadKind = 'whatsapp_dm' | 'whatsapp_group' | 'api' | 'try';

export interface ThreadSummary {
  id: string;
  title: string;
  kind: ThreadKind;
  people: { name: string, endUserId?: string, deleted?: boolean }[];
  lastSpeaker: string;
  lastSpeakerId?: string;
  lastDeleted?: boolean;
  lastText: string;
  status?: RunStatus;
  createdAt: Date;
  updatedAt: Date;
  runs: number;
  tokens: number;
  costCents: number;
}

export interface ThreadDetail extends ThreadSummary {
  items: TranscriptItem[];
}

// useThreadsStore reads an agent's threads through ThreadService; the sample agent keeps its sample conversations.
export const useThreadsStore = defineStore('threads', () => {
  const threads = ref<ThreadDetail[]>([]);
  const runs = ref<RunSummary[]>([]);
  const loading = ref(false);
  const error = ref<ShownError | null>(null);
  const cursor = ref('');
  const errorMessage = useErrorMessage();
  const endUsers = useEndUsersStore();
  const agents = useAgentsStore();
  const { $threadClient: client } = useNuxtApp();
  let loadedFor = '';
  let sampled = false;
  let timer: ReturnType<typeof setInterval> | undefined;
  const cancelAsked = ref<string[]>([]);
  const lastSeq = new Map<string, bigint>();
  const wireMessages = new Map<string, Awaited<ReturnType<typeof client.listThreadMessages>>['messages']>();

  function forget() {
    for (const p of endUsers.forgotten) {
      ({ threads: threads.value, runs: runs.value } = forgetInThreads(threads.value, runs.value, p));
    }
  }
  watch(() => endUsers.forgotten, forget);

  async function page(agentId: string, more: boolean) {
    const projectId = agents.items.find(a => a.id === agentId)?.projectId ?? '';
    const res = await client.listThreads({ projectId, agentId, pageToken: more ? cursor.value : '' });
    cursor.value = res.nextPageToken;
    return res.threads.map(toThread);
  }

  async function load(agentId: string, sample: boolean) {
    loading.value = true;
    error.value = null;
    stopPolling();
    try {
      sampled = sample;
      if (sample) {
        threads.value = sampleThreads();
        runs.value = sampleRuns();
        cursor.value = '';
        forget();
      } else {
        threads.value = await page(agentId, false);
        runs.value = [];
      }
      loadedFor = agentId;
    } catch (e) {
      error.value = errorMessage(e, 'error.load_threads');
    } finally {
      loading.value = false;
    }
  }

  function ensureLoaded(agentId: string, sample: boolean) {
    return loadedFor === agentId ? Promise.resolve() : load(agentId, sample);
  }

  async function loadMore(agentId: string) {
    if (!cursor.value) {
      return;
    }
    loading.value = true;
    error.value = null;
    try {
      const next = await page(agentId, true);
      threads.value = [...threads.value, ...next.filter(n => !threads.value.some(t => t.id === n.id))];
    } catch (e) {
      error.value = errorMessage(e, 'error.load_threads');
    } finally {
      loading.value = false;
    }
  }

  function byId(id: string) {
    return threads.value.find(t => t.id === id);
  }

  function put(thread: ThreadDetail) {
    threads.value = threads.value.some(t => t.id === thread.id) ? threads.value.map(t => (t.id === thread.id ? thread : t)) : [...threads.value, thread];
  }

  function rebuild(threadId: string, threadRuns: RunSummary[]) {
    const thread = byId(threadId);
    if (!thread) {
      return;
    }
    const sorted = [...threadRuns].sort((a, b) => a.startedAt.getTime() - b.startedAt.getTime());
    const open = sorted.find(r => r.status === 'running' || r.status === 'awaiting_approval');
    put({ ...thread, items: toItems(wireMessages.get(threadId) ?? [], sorted), status: open?.status ?? sorted.at(-1)?.status, runs: sorted.length, tokens: sorted.reduce((n, r) => n + r.tokens, 0), costCents: sorted.reduce((n, r) => n + r.costCents, 0) });
  }

  async function messagesAfter(threadId: string) {
    const all = wireMessages.get(threadId) ?? [];
    let after = lastSeq.get(threadId) ?? 0n;
    for (;;) {
      const res = await client.listThreadMessages({ threadId, afterSeq: after, limit: pageSize });
      all.push(...res.messages);
      after = res.messages.at(-1)?.seq ?? after;
      if (res.messages.length < pageSize) {
        break;
      }
    }
    wireMessages.set(threadId, all);
    lastSeq.set(threadId, after);
  }

  function threadRuns(threadId: string) {
    return runs.value.filter(r => r.threadId === threadId);
  }

  function stopPolling() {
    clearInterval(timer);
    timer = undefined;
  }

  async function poll(threadId: string) {
    const live = threadRuns(threadId).find(r => r.status === 'running');
    if (!live) {
      stopPolling();
      return;
    }
    try {
      const [got] = await Promise.all([client.getRun({ runId: live.id }), messagesAfter(threadId)]);
      if (got.run) {
        const run = toRunSummary(got.run, byId(threadId)?.title ?? '');
        runs.value = runs.value.map(r => (r.id === run.id ? run : r));
      }
      rebuild(threadId, threadRuns(threadId));
    } catch (e) {
      error.value = errorMessage(e, 'error.load_thread');
    }
    if (!threadRuns(threadId).some(r => r.status === 'running')) {
      stopPolling();
    }
  }

  // open reads one thread's messages and runs, then polls every 2 s while a run is running.
  async function open(threadId: string) {
    stopPolling();
    if (sampled) {
      return;
    }
    error.value = null;
    try {
      const got = await client.getThread({ threadId });
      if (!got.thread) {
        return;
      }
      put({ ...toThread(got.thread), runs: Number(got.thread.runCount) });
      wireMessages.delete(threadId);
      lastSeq.delete(threadId);
      const [listed] = await Promise.all([client.listRuns({ threadId, pageSize }), messagesAfter(threadId)]);
      const mine = listed.runs.map(r => toRunSummary(r, byId(threadId)?.title ?? ''));
      runs.value = [...runs.value.filter(r => r.threadId !== threadId), ...mine];
      rebuild(threadId, mine);
      if (mine.some(r => r.status === 'running')) {
        timer = setInterval(() => void poll(threadId), pollIntervalMs);
      }
    } catch (e) {
      error.value = errorMessage(e, 'error.load_thread');
    }
  }

  async function cancelRun(threadId: string, runId: string, by: string) {
    if (!sampled) {
      const res = await client.cancelRun({ runId });
      cancelAsked.value = [...cancelAsked.value, runId];
      if (res.run) {
        const run = toRunSummary(res.run, byId(threadId)?.title ?? '');
        runs.value = runs.value.map(r => (r.id === run.id ? run : r));
        rebuild(threadId, threadRuns(threadId));
      }
      if (!threadRuns(threadId).some(r => r.status === 'running')) {
        stopPolling();
      }
      return;
    }
    sampleCancel(threadId, runId, by);
  }

  function sampleCancel(threadId: string, runId: string, by: string) {
    const thread = byId(threadId);
    const run = runs.value.find(r => r.id === runId);
    if (run?.status === 'cancelled') {
      return;
    }
    if (run) {
      run.status = 'cancelled';
      run.detail = { key: 'runstatus.detail_cancelled', params: { name: by } };
    }
    if (thread) {
      thread.status = 'cancelled';
      thread.items = thread.items.map((i) => {
        if (i.kind === 'run' && i.run.id === runId) {
          return { ...i, run: { ...i.run, status: 'cancelled' } };
        }
        return i.kind === 'tool' && i.state === 'running' ? { ...i, state: 'failed' } : i;
      });
      thread.items.push({ id: Date.now(), kind: 'notice', key: 'try.stopped' });
    }
  }

  function runDetail(): RunDetail {
    return sampleRun();
  }

  return { threads, runs, loading, error, cursor, load, loadMore, ensureLoaded, byId, open, cancelRun, cancelling: (runId: string) => cancelAsked.value.includes(runId), stopPolling, polling: () => !!timer, runDetail };
});
