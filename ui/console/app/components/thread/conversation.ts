import type { StatusTone } from '#base/components/ui/types';
import type { RunStatus, RunSummary, ThreadKind, ThreadSummary } from '~/stores/threads';

export type ChannelFilter = 'whatsapp' | 'api' | 'try';

export const runStatusTone: Record<RunStatus, StatusTone> = {
  running: 'info',
  awaiting_approval: 'warning',
  completed: 'success',
  failed: 'error',
  cancelled: 'neutral',
  interrupted: 'warning',
  limit_reached: 'warning',
  expired: 'neutral',
};

export const runStatuses = Object.keys(runStatusTone) as RunStatus[];

export const channelFilters: ChannelFilter[] = ['whatsapp', 'api', 'try'];

// channelOf groups a thread kind under the channel filter it belongs to.
export function channelOf(kind: ThreadKind): ChannelFilter {
  return kind === 'whatsapp_dm' || kind === 'whatsapp_group' ? 'whatsapp' : kind;
}

export interface ThreadFilter {
  channel: ChannelFilter | '';
  status: RunStatus | '';
  query: string;
}

// filterFromQuery reads the thread filter from the URL query, ignoring values it does not know.
export function filterFromQuery(query: Record<string, unknown>): ThreadFilter {
  const pick = <T extends string>(value: unknown, allowed: readonly T[]): T | '' => (allowed.includes(value as T) ? value as T : '');
  return { channel: pick(query.channel, channelFilters), status: pick(query.status, runStatuses), query: typeof query.q === 'string' ? query.q : '' };
}

// filterQuery writes the thread filter back as URL query values, leaving out empty ones.
export function filterQuery(filter: ThreadFilter): Record<string, string> {
  const out: Record<string, string> = {};
  if (filter.channel) {
    out.channel = filter.channel;
  }
  if (filter.status) {
    out.status = filter.status;
  }
  if (filter.query.trim()) {
    out.q = filter.query.trim();
  }
  return out;
}

// filterThreads keeps the threads that match the channel, the latest run status and the search words.
export function filterThreads<T extends ThreadSummary>(threads: T[], filter: ThreadFilter): T[] {
  const q = filter.query.trim().toLowerCase();
  return threads.filter(t => (!filter.channel || channelOf(t.kind) === filter.channel)
    && (!filter.status || t.status === filter.status)
    && (!q || `${t.title} ${t.lastSpeaker} ${t.lastText}`.toLowerCase().includes(q)));
}

type Translate = (key: string, params?: Record<string, unknown>) => string;

export interface RunFix {
  to: string;
  labelKey: string;
}

// runDetailText says why a run ended in plain words: an error code becomes its app error text with the code kept for support.
export function runDetailText(detail: RunSummary['detail'], t: Translate, exists: (key: string) => boolean): string {
  if (!detail) {
    return '';
  }
  const code = detail.params?.code;
  if (detail.key === 'runstatus.detail_error' && code && exists(`apperror.${code}`)) {
    return t('runstatus.detail_reason', { reason: t(`apperror.${code}`), code });
  }
  return t(detail.key, detail.params ?? {});
}

// runFix links a failed run to the page that fixes its cause, when there is one; agentBase is the agent's console path.
export function runFix(detail: RunSummary['detail'], agentBase: string): RunFix | undefined {
  const code = detail?.key === 'runstatus.detail_error' ? detail.params?.code ?? '' : '';
  if (code === 'AGT006') {
    return { to: agentBase, labelKey: 'run.fix_configure' };
  }
  if (code.startsWith('PRV')) {
    return { to: `${agentBase.replace(/\/agents\/[^/]+$/, '')}/settings/model-providers`, labelKey: 'run.fix_providers' };
  }
  return undefined;
}
