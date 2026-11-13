import type { JsonObject, JsonValue } from '@bufbuild/protobuf';
import type { ThreadMessage as MessagePB, Run as RunPB, Thread as ThreadPB } from '#gen/thread/v1/thread_pb';
import type { PersonLabelParts } from '~/components/enduser/label';
import type { RunStatus, RunSummary, SkipReason, ThreadDetail, ThreadKind, TranscriptItem } from '~/stores/threads';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import { IdentityKind } from '#gen/enduser/v1/enduser_pb';
import { RunStatus as RunStatusPB } from '#gen/thread/v1/thread_pb';
import { personLabel } from '~/components/enduser/label';

type Translate = (key: string, params?: Record<string, unknown>) => string;

const statuses: Record<RunStatusPB, RunStatus | undefined> = {
  [RunStatusPB.UNSPECIFIED]: undefined,
  [RunStatusPB.RUNNING]: 'running',
  [RunStatusPB.AWAITING_APPROVAL]: 'awaiting_approval',
  [RunStatusPB.COMPLETED]: 'completed',
  [RunStatusPB.FAILED]: 'failed',
  [RunStatusPB.CANCELLED]: 'cancelled',
  [RunStatusPB.INTERRUPTED]: 'interrupted',
  [RunStatusPB.LIMIT_REACHED]: 'limit_reached',
  [RunStatusPB.EXPIRED]: 'expired',
};

const kinds: Record<string, ThreadKind> = { dm: 'whatsapp_dm', group: 'whatsapp_group', api: 'api', try: 'try' };

// skipReason maps why no turn took a channel message; a reason this console doesn't know yet still shows as skipped.
export function skipReason(reason: string): SkipReason | undefined {
  return !reason ? undefined : reason === 'stale' || reason === 'sender_rate' ? reason : 'other';
}

// threadKind maps a wire thread kind; an unknown one counts as an API thread.
export function threadKind(kind: string): ThreadKind {
  return kinds[kind] ?? 'api';
}

const phoneLike = /\+?\d[\d\s().-]{5,}\d/g;

// maskedLabel never lets a phone number through: any long digit run keeps only its last four digits.
export function maskedLabel(label: string): string {
  return label.replace(phoneLike, digits => `…${digits.replace(/\D/g, '').slice(-4)}`);
}

// wireLabel renders a person from the structured label fields in the viewer's locale, falling back to the server's masked label when the fields are empty.
export function wireLabel(parts: PersonLabelParts, fallback: string, t: Translate): string {
  if (!parts.displayName.trim() && parts.identityKind === IdentityKind.UNSPECIFIED && !parts.identityHint) {
    return maskedLabel(fallback);
  }
  return maskedLabel(personLabel(parts, t));
}

// runStatus maps the wire status; an unspecified one means no run is open.
export function runStatus(s: RunStatusPB): RunStatus | undefined {
  return statuses[s];
}

// toThread maps a ThreadService thread onto the console's thread shape; the transcript arrives later through ListThreadMessages.
export function toThread(th: ThreadPB, t: Translate): ThreadDetail {
  const at = th.lastActivityAt ? timestampDate(th.lastActivityAt) : new Date(0);
  const label = wireLabel({ displayName: th.endUserDisplayName, identityKind: th.endUserIdentityKind, identityHint: th.endUserIdentityHint }, th.endUserLabel, t);
  return {
    id: th.id,
    title: th.kind === 'try' ? t('try.title') : label || t(`threads.kind_${threadKind(th.kind)}`),
    kind: threadKind(th.kind),
    people: label ? [{ name: label }] : [],
    lastSpeaker: '',
    lastText: '',
    status: runStatus(th.openRunStatus),
    createdAt: at,
    updatedAt: at,
    runs: Number(th.runCount),
    tokens: 0,
    costCents: 0,
    items: [],
  };
}

const NO_REPLY: Record<string, string> = {
  no_text: 'runstatus.detail_no_reply_no_text',
  set_reply_empty: 'runstatus.detail_no_reply_set_reply_empty',
  step_limit: 'runstatus.detail_no_reply_step_limit',
};

function runDetail(r: RunPB): RunSummary['detail'] {
  if (r.errorCode) {
    return { key: 'runstatus.detail_error', params: { code: r.errorCode } };
  }
  if (r.noReplyReason) {
    return { key: NO_REPLY[r.noReplyReason] ?? 'runstatus.detail_no_reply' };
  }
  return undefined;
}

// toRunSummary maps a run, keeping whether its cost is unknown (unpriced) and its tokens estimated.
export function toRunSummary(r: RunPB, threadTitle: string): RunSummary {
  const u = r.usage;
  const tokens = u ? Number(u.inputTokens + u.outputTokens + u.cacheReadTokens + u.cacheWriteTokens) : 0;
  return {
    id: r.id,
    threadId: r.threadId,
    threadTitle,
    status: runStatus(r.status) ?? 'running',
    detail: runDetail(r),
    startedAt: r.startedAt ? timestampDate(r.startedAt) : new Date(0),
    seconds: r.latencyMs === undefined ? 0 : Number(r.latencyMs) / 1000,
    tokens,
    embeddingTokens: u ? Number(u.embeddingTokens) : 0,
    helperTokens: u ? Number(u.helperInputTokens + u.helperOutputTokens + u.helperCacheReadTokens + u.helperCacheWriteTokens) : 0,
    retrieveCalls: r.knowledgeRetrieveCalls,
    retrieveMs: Number(r.knowledgeRetrieveMs),
    retrievePassages: r.knowledgePassages,
    costCents: r.costMicros === undefined ? 0 : Number(r.costMicros) / 10_000,
    unpriced: r.costMicros === undefined,
    estimated: !!u?.estimated,
    draft: r.agentVersionKind === 'draft',
  };
}

function text(content: JsonValue | undefined): string {
  if (typeof content === 'string') {
    return content;
  }
  if (Array.isArray(content)) {
    return content.map(p => (p && typeof p === 'object' && !Array.isArray(p) && p.type === 'text' ? String(p.text ?? '') : '')).join('');
  }
  return '';
}

function args(raw: JsonValue | undefined): Record<string, unknown> {
  try {
    const parsed: unknown = typeof raw === 'string' ? JSON.parse(raw) : raw;
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed as Record<string, unknown> : {};
  } catch {
    return { arguments: raw };
  }
}

interface WireCall {
  id: string;
  name: string;
  args: Record<string, unknown>;
}

function calls(body: JsonObject): WireCall[] {
  const list = Array.isArray(body.toolCalls) ? body.toolCalls : [];
  return list.flatMap((c) => {
    if (!c || typeof c !== 'object' || Array.isArray(c)) {
      return [];
    }
    const fn = c.function && typeof c.function === 'object' && !Array.isArray(c.function) ? c.function : {};
    return [{ id: String(c.id ?? ''), name: String(fn.name ?? ''), args: args(fn.arguments) }];
  });
}

// toItems builds the transcript oldest first: tool results fold under their call, and each run's card follows its last message.
export function toItems(messages: MessagePB[], runs: RunSummary[], t: Translate): TranscriptItem[] {
  const sorted = [...messages].sort((a, b) => Number(a.seq - b.seq));
  const results = new Map(sorted.filter(m => m.role === 'tool').map(m => [m.toolCallId, text(m.message?.content)]));
  const lastOfRun = new Map(sorted.filter(m => m.runId).map(m => [m.runId, m.seq]));
  const byId = new Map(runs.map(r => [r.id, r]));
  const out: TranscriptItem[] = [];
  let id = 0;
  for (const m of sorted) {
    const body = m.message ?? {};
    const at = m.createdAt ? timestampDate(m.createdAt) : new Date(0);
    if (m.role === 'user') {
      const skipped = skipReason(m.skipReason);
      out.push({ id: id++, kind: 'user', text: text(body.content), speaker: wireLabel({ displayName: m.speakerDisplayName, identityKind: m.speakerIdentityKind, identityHint: m.speakerIdentityHint }, m.speakerLabel, t) || undefined, ...(skipped ? { skipped } : {}), at });
    } else if (m.role === 'assistant') {
      const said = text(body.content);
      if (said) {
        out.push({ id: id++, kind: 'agent', text: said, streaming: false, at });
      }
      for (const c of calls(body)) {
        const result = results.get(c.id);
        out.push({ id: id++, kind: 'tool', name: c.name, args: c.args, result, state: result === undefined ? 'running' : 'done' });
      }
    } else if (m.role === 'tool' && !sorted.some(o => calls(o.message ?? {}).some(c => c.id === m.toolCallId))) {
      out.push({ id: id++, kind: 'tool', name: m.toolCallId, args: {}, result: text(body.content), state: 'done' });
    }
    const run = byId.get(m.runId);
    if (run && lastOfRun.get(m.runId) === m.seq) {
      out.push({ id: id++, kind: 'run', run });
    }
  }
  return out;
}
