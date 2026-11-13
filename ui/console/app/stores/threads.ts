import type { ShownError } from '#base/lib/apperror/message';
import type { ApprovalRequest } from '~/stores/approvals';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import { useErrorMessage } from '#base/composables/useErrorMessage';
import { errorCode } from '#base/lib/apperror/message';
import { toItems, toRunSummary, toThread } from '~/components/thread/fromWire';
import { sampleRuns, sampleThreads } from '~/preview/samples';
import { useAgentsStore } from './agents';

export const pollIntervalMs = 2_000;
const pageSize = 200;

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
  embeddingTokens?: number;
  retrieveCalls?: number;
  retrieveMs?: number;
  retrievePassages?: number;
  costCents: number;
  unpriced?: boolean;
  estimated?: boolean;
  draft?: boolean;
}

export interface RunView {
  run: RunSummary;
  attempt: number;
  agentVersion: number;
  endedAt?: Date;
  thread?: ThreadDetail;
  items: TranscriptItem[];
}

export type ToolState = 'running' | 'done' | 'preview' | 'failed' | 'awaiting' | 'rejected';

export interface Photo {
  name: string;
  url: string;
}

export type SkipReason = 'stale' | 'sender_rate' | 'other';

export type TranscriptItem
  = | { id: number, kind: 'user', text: string, speaker?: string, speakerId?: string, photo?: Photo, photoPlaceholder?: boolean, deleted?: boolean, skipped?: SkipReason, at?: Date }
    | { id: number, kind: 'agent', text: string, streaming: boolean, failed?: boolean, at?: Date }
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
  unpricedRuns?: number;
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
  const agents = useAgentsStore();
  const { $threadClient: client, $i18n } = useNuxtApp();
  // shortcut: labels render in the locale active when a thread loads; a locale switch shows them on the next load.
  const t = (key: string, params?: Record<string, unknown>) => $i18n.t(key, params ?? {});
  let loadedFor = '';
  let sampled = false;
  let timer: ReturnType<typeof setInterval> | undefined;
  const cancelAsked = ref<string[]>([]);
  const runView = ref<RunView | null>(null);
  const hasThreads = ref<Record<string, boolean>>({});
  const runsLoading = ref(false);
  const runsError = ref<ShownError | null>(null);
  const runLoading = ref(false);
  const runError = ref<ShownError | null>(null);
  const lastSeq = new Map<string, bigint>();
  const wireMessages = new Map<string, Awaited<ReturnType<typeof client.listThreadMessages>>['messages']>();

  function projectOf(agentId: string) {
    return agents.items.find(a => a.id === agentId)?.projectId ?? '';
  }

  async function page(agentId: string, more: boolean) {
    const projectId = projectOf(agentId);
    const res = await client.listThreads({ projectId, agentId, pageToken: more ? cursor.value : '' });
    cursor.value = res.nextPageToken;
    return res.threads.map(th => toThread(th, t));
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
    put({ ...thread, items: toItems(wireMessages.get(threadId) ?? [], sorted, t), status: open?.status ?? sorted.at(-1)?.status, runs: sorted.length, tokens: sorted.reduce((n, r) => n + r.tokens, 0), costCents: sorted.reduce((n, r) => n + r.costCents, 0), unpricedRuns: sorted.filter(r => r.unpriced).length });
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
      put({ ...toThread(got.thread, t), runs: Number(got.thread.runCount) });
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

  // checkHasThreads records whether someone has tried the agent in the console yet, for the setup steps; a failure counts as none.
  async function checkHasThreads(projectId: string, agentId: string) {
    try {
      const res = await client.listThreads({ projectId, agentId, kind: 'try', pageSize: 1 });
      hasThreads.value = { ...hasThreads.value, [agentId]: res.threads.length > 0 };
    } catch {
      hasThreads.value = { ...hasThreads.value, [agentId]: false };
    }
  }

  // loadAgentRuns lists an agent's runs, newest first, for the runs page; the sample agent keeps its sample runs.
  async function loadAgentRuns(agentId: string, sample: boolean) {
    await ensureLoaded(agentId, sample);
    if (sampled) {
      return;
    }
    runsLoading.value = true;
    runsError.value = null;
    try {
      // shortcut: the newest page only (200, the API maximum); add paging when an agent's list outgrows it.
      const listed = await client.listRuns({ projectId: projectOf(agentId), agentId, pageSize });
      runs.value = listed.runs.map(r => toRunSummary(r, byId(r.threadId)?.title ?? ''));
    } catch (e) {
      runsError.value = errorMessage(e, 'error.load_runs');
    } finally {
      runsLoading.value = false;
    }
  }

  // loadRun reads one run, its thread and the messages that run took or wrote.
  async function loadRun(runId: string) {
    runLoading.value = true;
    runError.value = null;
    try {
      const got = await client.getRun({ runId });
      if (!got.run) {
        runView.value = null;
        return;
      }
      const threadId = got.run.threadId;
      const [th] = await Promise.all([client.getThread({ threadId }), (wireMessages.delete(threadId), lastSeq.delete(threadId), messagesAfter(threadId))]);
      const thread = th.thread ? toThread(th.thread, t) : undefined;
      runView.value = {
        run: toRunSummary(got.run, thread?.title ?? ''),
        attempt: got.run.attempt,
        agentVersion: got.run.agentVersion,
        endedAt: got.run.endedAt ? timestampDate(got.run.endedAt) : undefined,
        thread,
        items: toItems((wireMessages.get(threadId) ?? []).filter(m => m.runId === runId), [], t),
      };
    } catch (e) {
      runView.value = null;
      runError.value = errorCode(e) === 'RUN001' ? null : errorMessage(e, 'error.load_run');
    } finally {
      runLoading.value = false;
    }
  }

  return { threads, runs, loading, error, cursor, load, loadMore, ensureLoaded, byId, open, cancelRun, cancelling: (runId: string) => cancelAsked.value.includes(runId), stopPolling, polling: () => !!timer, hasThreads, checkHasThreads, runsLoading, runsError, loadAgentRuns, runView, runLoading, runError, loadRun };
});
