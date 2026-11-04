import type { ShownError } from '#base/lib/apperror/message';
import type { ApprovalRequest } from '~/stores/approvals';
import { useErrorMessage } from '#base/composables/useErrorMessage';
import { forgetInThreads } from '~/components/enduser/forget';
import { sampleRun, sampleRuns, sampleThreads } from '~/preview/samples';
import { useEndUsersStore } from './endUsers';

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
  status: RunStatus;
  createdAt: Date;
  updatedAt: Date;
  runs: number;
  tokens: number;
  costCents: number;
}

export interface ThreadDetail extends ThreadSummary {
  items: TranscriptItem[];
}

// TODO: replace with the thread and run RPCs; until then this serves the sample conversations of the sample agent.
export const useThreadsStore = defineStore('threads', () => {
  const threads = ref<ThreadDetail[]>([]);
  const runs = ref<RunSummary[]>([]);
  const loading = ref(false);
  const error = ref<ShownError | null>(null);
  const errorMessage = useErrorMessage();
  const endUsers = useEndUsersStore();
  let loadedFor = '';

  function forget() {
    for (const p of endUsers.forgotten) {
      ({ threads: threads.value, runs: runs.value } = forgetInThreads(threads.value, runs.value, p));
    }
  }
  watch(() => endUsers.forgotten, forget);

  async function load(agentId: string, sample: boolean) {
    loading.value = true;
    error.value = null;
    try {
      const [t, r] = await Promise.resolve().then(() => [sample ? sampleThreads() : [], sample ? sampleRuns() : []] as const);
      threads.value = t;
      runs.value = r;
      forget();
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

  function byId(id: string) {
    return threads.value.find(t => t.id === id);
  }

  function cancelRun(threadId: string, runId: string, by: string) {
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

  return { threads, runs, loading, error, load, ensureLoaded, byId, cancelRun, runDetail };
});
