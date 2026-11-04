import type { Photo, TranscriptItem } from '~/stores/threads';
import type { ChatScript, ChatStep } from '~/stores/tryIt';
import { getCurrentScope, onScopeDispose } from 'vue';
import { decideApproval } from '~/components/approval/decision';
import { useTryItStore } from '~/stores/tryIt';

export type Scenario = 'normal' | 'model_fails' | 'tool_fails' | 'cost_limit' | 'dropped';
export type Phase = 'idle' | 'thinking' | 'streaming' | 'tool' | 'awaiting' | 'completed' | 'failed' | 'cancelled' | 'limit' | 'dropped';
export type { Photo, ToolState } from '~/stores/threads';
export type ChatItem = TranscriptItem;

type NewItem<T> = T extends ChatItem ? Omit<T, 'id'> : never;

export const maxPhotoBytes = 5 * 1024 * 1024;
export const photoTypes = ['image/jpeg', 'image/png', 'image/webp'];
export const scenarios: Scenario[] = ['normal', 'model_fails', 'tool_fails', 'cost_limit', 'dropped'];

// photoError returns the text key that refuses a file as a chat photo, or '' when it is fine.
export function photoError(file: { size: number, type: string }): string {
  if (!photoTypes.includes(file.type)) {
    return 'try.photo_type';
  }
  return file.size > maxPhotoBytes ? 'try.photo_too_big' : '';
}

export interface TryTiming {
  think: number;
  tool: number;
  tick: number;
  chars: number;
}

const defaultTiming: TryTiming = { think: 700, tool: 600, tick: 30, chars: 4 };

// useTryChat plays the sample agent replies a run would stream, with tools, approvals and the failure cases, until the runtime exists.
export interface TryLimits {
  maxSteps: () => number;
  maxCostCents: () => number;
  money: (cents: number) => string;
  decider?: () => string;
}

export function useTryChat(limits: TryLimits, timing: TryTiming = defaultTiming, scriptFor?: (prompt: string) => ChatScript) {
  const scripted = scriptFor ?? useTryItStore().script;
  const items = ref<ChatItem[]>([]);
  const phase = ref<Phase>('idle');
  const scenario = ref<Scenario>('normal');
  const steps = ref(0);
  const tokens = ref(0);
  const costCents = ref(0);
  const photo = ref<Photo | null>(null);
  const photoProblem = ref('');
  let nextId = 1;
  let run = 0;
  const timers = new Map<ReturnType<typeof setTimeout>, () => void>();
  const photoUrls = new Set<string>();
  let script: ChatScript | null = null;
  let pending: ChatStep[] = [];
  let lastPrompt = '';

  const busy = computed(() => ['thinking', 'streaming', 'tool'].includes(phase.value));
  const toolCalls = computed(() => items.value.filter((i): i is Extract<ChatItem, { kind: 'tool' }> => i.kind === 'tool'));
  const awaiting = computed(() => items.value.find((i): i is Extract<ChatItem, { kind: 'approval' }> => i.kind === 'approval' && i.approval.status === 'pending'));

  function push<T extends ChatItem = ChatItem>(item: NewItem<T>): T {
    const added = { ...item, id: nextId++ } as unknown as T;
    items.value.push(added);
    return items.value.at(-1) as T;
  }

  async function wait(ms: number, mine: number) {
    await new Promise<void>((resolve) => {
      const id = setTimeout(() => {
        timers.delete(id);
        resolve();
      }, ms);
      timers.set(id, resolve);
    });
    return mine === run;
  }

  function cancelTimers() {
    run++;
    for (const [id, resolve] of timers) {
      clearTimeout(id);
      resolve();
    }
    timers.clear();
  }

  function dropPending() {
    if (photo.value) {
      URL.revokeObjectURL(photo.value.url);
      photoUrls.delete(photo.value.url);
    }
    photo.value = null;
  }

  function revokeAll() {
    photoUrls.forEach(url => URL.revokeObjectURL(url));
    photoUrls.clear();
    photo.value = null;
  }

  function count() {
    steps.value++;
    tokens.value += script?.tokensPerStep ?? 0;
    costCents.value = Math.round((costCents.value + (script?.centsPerStep ?? 0)) * 10) / 10;
  }

  function stopAtLimit(on: 'cost' | 'steps') {
    const cap = limits.maxCostCents();
    phase.value = 'limit';
    if (on === 'cost' && cap > 0) {
      costCents.value = Math.max(costCents.value, cap);
      push({ kind: 'notice', key: 'try.limit_cost', params: { cap: limits.money(cap) } });
    } else {
      push({ kind: 'notice', key: 'try.limit_steps', params: { n: String(limits.maxSteps() || steps.value) } });
    }
  }

  function overSteps() {
    const max = limits.maxSteps();
    return max > 0 && steps.value >= max;
  }

  async function streamText(text: string, mine: number, dropAt = -1) {
    const bubble = push<Extract<ChatItem, { kind: 'agent' }>>({ kind: 'agent', text: '', streaming: true, at: new Date() });
    phase.value = 'streaming';
    while (bubble.text.length < text.length) {
      if (!(await wait(timing.tick, mine))) {
        bubble.streaming = false;
        return false;
      }
      bubble.text = text.slice(0, bubble.text.length + timing.chars);
      if (dropAt >= 0 && bubble.text.length >= dropAt) {
        bubble.streaming = false;
        phase.value = 'dropped';
        pending = [{ kind: 'text', text: text.slice(bubble.text.length) }];
        push({ kind: 'notice', key: 'try.dropped' });
        return false;
      }
    }
    bubble.streaming = false;
    return true;
  }

  function gatedChip(name: string) {
    return items.value.find((i): i is Extract<ChatItem, { kind: 'tool' }> => i.kind === 'tool' && i.state === 'awaiting' && i.name === name);
  }

  function moveLast<T extends ChatItem>(item: T): T {
    items.value = [...items.value.filter(i => i.id !== item.id), item];
    return items.value.at(-1) as T;
  }

  async function play(list: ChatStep[], mine: number) {
    let failedTool = scenario.value === 'tool_fails';
    for (let i = 0; i < list.length; i++) {
      const step = list[i]!;
      if (overSteps()) {
        stopAtLimit('steps');
        return;
      }
      phase.value = 'thinking';
      if (!(await wait(timing.think, mine))) {
        return;
      }
      if (scenario.value === 'model_fails') {
        phase.value = 'failed';
        push({ kind: 'agent', text: '', streaming: false, failed: true, at: new Date() });
        return;
      }
      count();
      if (step.kind === 'tool') {
        phase.value = 'tool';
        const gated = gatedChip(step.name);
        const chip = gated ? moveLast(gated) : push<Extract<ChatItem, { kind: 'tool' }>>({ kind: 'tool', name: step.name, args: step.args, state: 'running' });
        chip.state = 'running';
        if (!(await wait(timing.tool, mine))) {
          chip.state = 'failed';
          return;
        }
        chip.state = failedTool ? 'failed' : step.preview ? 'preview' : 'done';
        chip.result = failedTool ? undefined : step.result;
        failedTool = false;
      } else if (step.kind === 'approval') {
        push({ kind: 'tool', name: step.approval.tool, args: step.approval.raw, state: 'awaiting' });
        push({ kind: 'approval', approval: { ...step.approval } });
        phase.value = 'awaiting';
        return;
      } else if (!(await streamText(step.text, mine, scenario.value === 'dropped' ? Math.floor(step.text.length / 2) : -1))) {
        return;
      }
      if (scenario.value === 'cost_limit') {
        stopAtLimit('cost');
        return;
      }
    }
    phase.value = 'completed';
  }

  function send(text: string) {
    const prompt = text.trim();
    if (!prompt || busy.value || phase.value === 'awaiting') {
      return Promise.resolve();
    }
    lastPrompt = prompt;
    push({ kind: 'user', text: prompt, photo: photo.value ?? undefined, at: new Date() });
    photo.value = null;
    script = scripted(prompt);
    return play(script.steps, ++run);
  }

  function decide(approve: boolean, reason = '') {
    const item = awaiting.value;
    const decided = item && decideApproval(item.approval, approve, limits.decider?.() ?? '', { channel: 'try' }, reason);
    if (!item || !decided || !script) {
      return Promise.resolve();
    }
    item.approval = decided;
    const chip = gatedChip(item.approval.tool);
    if (chip && !approve) {
      chip.state = 'rejected';
    }
    return play(approve ? script.afterApprove : script.afterReject, ++run);
  }

  function stop() {
    if (!busy.value) {
      return;
    }
    cancelTimers();
    items.value.filter(i => i.kind === 'agent' && i.streaming).forEach((i) => {
      (i as Extract<ChatItem, { kind: 'agent' }>).streaming = false;
    });
    items.value.filter(i => i.kind === 'tool' && i.state === 'running').forEach((i) => {
      (i as Extract<ChatItem, { kind: 'tool' }>).state = 'failed';
    });
    phase.value = 'cancelled';
    push({ kind: 'notice', key: 'try.stopped' });
  }

  function reload() {
    const rest = pending;
    pending = [];
    for (const step of rest) {
      if (step.kind === 'text') {
        const last = [...items.value].reverse().find((i): i is Extract<ChatItem, { kind: 'agent' }> => i.kind === 'agent');
        if (last) {
          last.text += step.text;
        }
      }
    }
    items.value = items.value.filter(i => !(i.kind === 'notice' && i.key === 'try.dropped'));
    phase.value = 'completed';
  }

  function retry() {
    if (phase.value !== 'failed' || !lastPrompt) {
      return Promise.resolve();
    }
    items.value = items.value.filter(i => !(i.kind === 'agent' && i.failed));
    scenario.value = 'normal';
    script = scripted(lastPrompt);
    return play(script.steps, ++run);
  }

  function newChat() {
    cancelTimers();
    revokeAll();
    items.value = [];
    phase.value = 'idle';
    steps.value = 0;
    tokens.value = 0;
    costCents.value = 0;
    photo.value = null;
    photoProblem.value = '';
    pending = [];
  }

  function attach(file: File) {
    const problem = photoError(file);
    photoProblem.value = problem;
    if (!problem) {
      dropPending();
      const url = URL.createObjectURL(file);
      photoUrls.add(url);
      photo.value = { name: file.name, url };
    }
  }

  function detach() {
    dropPending();
    photoProblem.value = '';
  }

  if (getCurrentScope()) {
    onScopeDispose(() => {
      cancelTimers();
      revokeAll();
    });
  }

  return { items, phase, scenario, steps, tokens, costCents, photo, photoProblem, busy, toolCalls, awaiting, send, decide, stop, reload, retry, newChat, attach, detach };
}
