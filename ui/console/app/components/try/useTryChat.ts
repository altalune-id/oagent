import type { AgUiEvent } from '#base/lib/agui/sse';
import type { Run as RunPB } from '#gen/thread/v1/thread_pb';
import type { RunStatus, RunSummary, TranscriptItem } from '~/stores/threads';
import { Code, ConnectError } from '@connectrpc/connect';
import { getCurrentScope, onScopeDispose } from 'vue';
import { readEvents } from '#base/lib/agui/sse';
import { ErrorDetailSchema } from '#gen/apperror/v1/error_pb';
import { toRunSummary } from '~/components/thread/fromWire';

export type Phase = 'idle' | 'thinking' | 'streaming' | 'tool' | 'stopping' | 'completed' | 'failed' | 'cancelled' | 'limit' | 'dropped';
export type { ToolState } from '~/stores/threads';
export type ChatItem = TranscriptItem;

type NewItem<T> = T extends ChatItem ? Omit<T, 'id'> : never;
type AgentItem = Extract<ChatItem, { kind: 'agent' }>;
type ToolItem = Extract<ChatItem, { kind: 'tool' }>;

export interface RunBody {
  threadId: string;
  messages: { id: string, role: 'user', content: string }[];
}

export interface StoredMessage {
  seq: number;
  message: Record<string, unknown>;
}

export interface MessagesPage {
  messages: StoredMessage[];
  next_after: number;
}

export interface ThreadMemory {
  get: () => string | null;
  set: (threadId: string) => void;
}

export interface TryApi {
  run: (body: RunBody, signal: AbortSignal) => Promise<ReadableStream<Uint8Array>>;
  cancel: (runId: string) => Promise<void>;
  messages: (threadId: string, after: number) => Promise<MessagesPage>;
  getRun: (runId: string) => Promise<RunPB | undefined>;
}

// envelopeError turns the data plane's JSON error into a ConnectError with the app error detail, so it shows like any RPC error.
export async function envelopeError(res: Response): Promise<ConnectError> {
  let body: { code?: string, message?: string, request_id?: string, meta?: Record<string, string> } = {};
  try {
    body = await res.json();
  } catch {
    body = {};
  }
  const details = body.code ? [{ desc: ErrorDetailSchema, value: { code: body.code, requestId: body.request_id ?? '', meta: body.meta ?? {} } }] : [];
  return new ConnectError(body.message ?? res.statusText, Code.Unknown, undefined, details);
}

// tryApi calls the console Try it routes for one agent with the session cookie.
export function tryApi(consoleBase: string, agentId: string, getRun: TryApi['getRun'], fetcher: typeof fetch = (...a) => fetch(...a)): TryApi {
  const root = `${consoleBase.replace(/\/$/, '')}/agents/${encodeURIComponent(agentId)}/try`;
  async function call(path: string, init: RequestInit = {}) {
    const res = await fetcher(root + path, { credentials: 'same-origin', ...init });
    if (!res.ok) {
      throw await envelopeError(res);
    }
    return res;
  }
  return {
    run: async (body, signal) => (await call('/runs', { method: 'POST', headers: { 'Content-Type': 'application/json', 'Accept': 'text/event-stream' }, body: JSON.stringify(body), signal })).body!,
    cancel: async (runId) => {
      await call(`/runs/${encodeURIComponent(runId)}/cancel`, { method: 'POST' });
    },
    messages: async (threadId, after) => (await call(`/threads/${encodeURIComponent(threadId)}/messages?after=${after}&limit=200`)).json(),
    getRun,
  };
}

const phaseOf: Partial<Record<RunStatus, Phase>> = { failed: 'failed', cancelled: 'cancelled', limit_reached: 'limit', interrupted: 'failed', expired: 'failed' };

function textOf(content: unknown): string {
  if (typeof content === 'string') {
    return content;
  }
  return Array.isArray(content) ? content.map(p => (p?.type === 'text' ? String(p.text ?? '') : '')).join('') : '';
}

function jsonOf(raw: string): unknown {
  try {
    return JSON.parse(raw);
  } catch {
    return raw;
  }
}

function argsOf(raw: string): Record<string, unknown> {
  const v = jsonOf(raw);
  return v && typeof v === 'object' && !Array.isArray(v) ? v as Record<string, unknown> : {};
}

// useTryChat runs a Try it conversation on the agent's draft: it streams each turn's AG-UI events into the transcript and reads the run's usage when it ends.
export function useTryChat(api: TryApi, newId: () => string = () => crypto.randomUUID(), memory?: ThreadMemory) {
  const items = ref<ChatItem[]>([]);
  const phase = ref<Phase>('idle');
  const tokens = ref(0);
  const costCents = ref(0);
  const error = ref<unknown>(null);
  const threadId = ref(memory?.get() || newId());
  let nextId = 1;
  let turn = 0;
  const runId = ref('');
  let stopping = false;
  let controller: AbortController | null = null;
  let last: { id: string, text: string } | null = null;
  const bubbles = new Map<string, AgentItem>();
  const calls = new Map<string, { chip: ToolItem, raw: string }>();

  const busy = computed(() => ['thinking', 'streaming', 'tool', 'stopping'].includes(phase.value));
  const canStop = computed(() => !!runId.value && ['thinking', 'streaming', 'tool'].includes(phase.value));

  function setPhase(next: Phase) {
    if (!stopping) {
      phase.value = next;
    }
  }
  const toolCalls = computed(() => items.value.filter((i): i is ToolItem => i.kind === 'tool'));

  function push<T extends ChatItem = ChatItem>(item: NewItem<T>): T {
    items.value.push({ ...item, id: nextId++ } as unknown as ChatItem);
    return items.value.at(-1) as T;
  }

  function bubble(messageId: string) {
    let b = bubbles.get(messageId);
    if (!b) {
      b = push<AgentItem>({ kind: 'agent', text: '', streaming: true, at: new Date() });
      bubbles.set(messageId, b);
    }
    return b;
  }

  function apply(e: AgUiEvent) {
    switch (e.type) {
      case 'RUN_STARTED':
        runId.value = String(e.runId ?? '');
        break;
      case 'TEXT_MESSAGE_START':
      case 'TEXT_MESSAGE_CONTENT':
        bubble(String(e.messageId)).text += String(e.delta ?? '');
        setPhase('streaming');
        break;
      case 'TEXT_MESSAGE_END':
        bubble(String(e.messageId)).streaming = false;
        break;
      case 'TOOL_CALL_START':
        calls.set(String(e.toolCallId), { chip: push<ToolItem>({ kind: 'tool', name: String(e.toolCallName ?? ''), args: {}, state: 'running' }), raw: '' });
        setPhase('tool');
        break;
      case 'TOOL_CALL_ARGS': {
        const call = calls.get(String(e.toolCallId));
        if (call) {
          call.raw += String(e.delta ?? '');
          call.chip.args = argsOf(call.raw);
        }
        break;
      }
      case 'TOOL_CALL_RESULT': {
        const call = calls.get(String(e.toolCallId));
        if (call) {
          call.chip.result = jsonOf(String(e.content ?? ''));
          call.chip.state = 'done';
        }
        break;
      }
    }
  }

  function settleStreaming(failed: boolean) {
    for (const i of items.value) {
      if (i.kind === 'agent' && i.streaming) {
        i.streaming = false;
      }
      if (failed && i.kind === 'tool' && i.state === 'running') {
        i.state = 'failed';
      }
    }
  }

  async function settle(fallback: Phase) {
    let run: RunSummary | undefined;
    try {
      const got = runId.value ? await api.getRun(runId.value) : undefined;
      run = got ? toRunSummary(got, '') : undefined;
    } catch {
      run = undefined;
    }
    if (run?.status === 'running') {
      run = undefined;
    }
    const asked = stopping;
    stopping = false;
    phase.value = run ? (phaseOf[run.status] ?? 'completed') : fallback;
    if (phase.value === 'cancelled' && asked) {
      push({ kind: 'notice', key: 'try.stopped' });
    }
    if (phase.value === 'dropped') {
      push({ kind: 'notice', key: 'try.dropped' });
      return;
    }
    settleStreaming(phase.value !== 'completed');
    if (phase.value === 'failed') {
      push({ kind: 'agent', text: '', streaming: false, failed: true, at: new Date() });
    }
    if (run) {
      tokens.value += run.tokens;
      costCents.value = Math.round((costCents.value + run.costCents) * 10_000) / 10_000;
      push({ kind: 'run', run });
    }
  }

  async function stream(message: { id: string, text: string }) {
    const mine = ++turn;
    phase.value = 'thinking';
    error.value = null;
    runId.value = '';
    stopping = false;
    bubbles.clear();
    calls.clear();
    controller = new AbortController();
    let end = '';
    try {
      const body = await api.run({ threadId: threadId.value, messages: [{ id: message.id, role: 'user', content: message.text }] }, controller.signal);
      for await (const e of readEvents(body)) {
        if (mine !== turn) {
          return;
        }
        apply(e);
        if (e.type === 'RUN_FINISHED') {
          end = 'finished';
        } else if (e.type === 'RUN_ERROR') {
          end = String(e.code ?? 'error');
        }
      }
    } catch (e) {
      if (mine !== turn) {
        return;
      }
      if (!runId.value) {
        phase.value = 'failed';
        error.value = e;
        return;
      }
    }
    if (mine !== turn) {
      return;
    }
    if (!end || end === 'detached') {
      stopping = false;
      settleStreaming(false);
      phase.value = 'dropped';
      push({ kind: 'notice', key: 'try.dropped' });
      return;
    }
    await settle(end === 'finished' ? 'completed' : end === 'cancelled' ? 'cancelled' : 'failed');
  }

  function send(text: string) {
    const prompt = text.trim();
    if (!prompt || busy.value) {
      return Promise.resolve();
    }
    last = { id: newId(), text: prompt };
    memory?.set(threadId.value);
    push({ kind: 'user', text: prompt, at: new Date() });
    return stream(last);
  }

  function retry() {
    if (phase.value !== 'failed' || !last) {
      return Promise.resolve();
    }
    items.value = items.value.filter(i => !(i.kind === 'agent' && i.failed));
    last = { id: newId(), text: last.text };
    return stream(last);
  }

  // stop asks the server to cancel and shows "stopping"; the run's own final status decides what the chat says.
  async function stop() {
    if (!canStop.value) {
      return;
    }
    const before = phase.value;
    stopping = true;
    phase.value = 'stopping';
    try {
      await api.cancel(runId.value);
    } catch (e) {
      stopping = false;
      phase.value = before;
      error.value = e;
    }
  }

  async function stored(): Promise<Record<string, unknown>[]> {
    const all: StoredMessage[] = [];
    for (let after = 0; ;) {
      const page = await api.messages(threadId.value, after);
      all.push(...page.messages);
      if (!page.messages.length || page.next_after <= after) {
        break;
      }
      after = page.next_after;
    }
    return all.sort((a, b) => a.seq - b.seq).map(m => m.message);
  }

  function appendStored(messages: Record<string, unknown>[], at?: Date) {
    bubbles.clear();
    calls.clear();
    for (const m of messages) {
      if (m.role === 'user') {
        push({ kind: 'user', text: textOf(m.content), at });
      } else if (m.role === 'assistant') {
        const text = textOf(m.content);
        if (text) {
          push({ kind: 'agent', text, streaming: false, at });
        }
        for (const c of Array.isArray(m.toolCalls) ? m.toolCalls as { id?: unknown, function?: { name?: unknown, arguments?: unknown } }[] : []) {
          calls.set(String(c.id), { chip: push<ToolItem>({ kind: 'tool', name: String(c.function?.name ?? ''), args: argsOf(String(c.function?.arguments ?? '')), state: 'running' }), raw: '' });
        }
      } else if (m.role === 'tool') {
        const call = calls.get(String(m.toolCallId));
        if (call) {
          call.chip.result = jsonOf(textOf(m.content));
          call.chip.state = 'done';
        }
      }
    }
  }

  async function reload() {
    let messages: Record<string, unknown>[];
    try {
      messages = await stored();
    } catch (e) {
      error.value = e;
      return;
    }
    const kept = items.value.filter(i => !(i.kind === 'notice' && i.key === 'try.dropped'));
    items.value = kept.slice(0, kept.findLastIndex(i => i.kind === 'user') + 1);
    appendStored(messages.slice(messages.findLastIndex(m => m.role === 'user') + 1), new Date());
    // shortcut: a run still going after the reload stays "dropped" with Reload; poll GetRun if that proves common.
    await settle('dropped');
  }

  // restore shows the remembered Try it conversation again, without times since stored messages carry none; a missing one starts fresh.
  async function restore() {
    if (!memory?.get() || items.value.length) {
      return;
    }
    try {
      appendStored(await stored());
    } catch {
      items.value = [];
    }
  }

  function newChat() {
    turn++;
    controller?.abort();
    items.value = [];
    phase.value = 'idle';
    tokens.value = 0;
    costCents.value = 0;
    error.value = null;
    threadId.value = newId();
    memory?.set('');
    runId.value = '';
    stopping = false;
    last = null;
    bubbles.clear();
    calls.clear();
  }

  if (getCurrentScope()) {
    onScopeDispose(() => {
      turn++;
      controller?.abort();
    });
  }

  return { items, phase, tokens, costCents, error, threadId, busy, canStop, toolCalls, send, retry, stop, reload, restore, newChat };
}
