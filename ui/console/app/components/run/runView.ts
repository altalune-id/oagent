import type { RunDetail, RunEvent, RunEventType, RunStatus as RunState } from '~/stores/threads';

export type Lane = 'model' | 'tool' | 'approval' | 'error';
export type RunStatus = 'waiting' | 'approved' | 'rejected';

export const lanes: Lane[] = ['model', 'tool', 'approval', 'error'];

// NOTE: colour is the event type only, and each passes 3:1 on base-100 and base-200 in every theme (ui/base/test/markup.test.ts).
export const laneColor: Record<Lane, string> = { model: 'bg-info', tool: 'bg-base-content/70', approval: 'bg-warning', error: 'bg-error' };

const laneOf: Record<RunEventType, Lane | null> = { input: null, model: 'model', output: 'model', tool: 'tool', approval: 'approval', error: 'error' };

// eventName labels a run step in the UI language; tool steps keep their tool name.
export function eventName(event: RunEvent, t: (key: string) => string): string {
  return event.labelKey ? t(event.labelKey) : event.name;
}

export function lane(event: RunEvent): Lane | null {
  return laneOf[event.type];
}

export function eventsFor(run: RunDetail, status: RunStatus): RunEvent[] {
  if (status === 'approved') {
    return [...run.events, ...run.afterApprove];
  }
  if (status === 'rejected') {
    return [...run.events, ...run.afterReject];
  }
  return run.events;
}

export function totals(events: RunEvent[]) {
  const end = Math.max(0, ...events.map(e => e.at + e.took));
  return {
    seconds: Math.round(end * 10) / 10,
    costCents: Math.round(events.reduce((sum, e) => sum + (e.costCents ?? 0), 0) * 10) / 10,
    steps: events.filter(e => e.type !== 'input').length,
  };
}

export function scale(events: RunEvent[]): number {
  return Math.max(20, Math.ceil(totals(events).seconds / 20) * 20);
}

export function ticks(max: number): number[] {
  return Array.from({ length: max / 20 + 1 }, (_, i) => i * 20);
}

export function toolCalls(events: RunEvent[]) {
  const byName = new Map<string, { calls: number, failed: number, seconds: number }>();
  for (const e of events.filter(e => e.type === 'tool' || e.type === 'error')) {
    const row = byName.get(e.name) ?? { calls: 0, failed: 0, seconds: 0 };
    row.calls++;
    row.failed += e.type === 'error' ? 1 : 0;
    row.seconds = Math.round((row.seconds + e.took) * 10) / 10;
    byName.set(e.name, row);
  }
  return [...byName].map(([name, row]) => ({ name, ...row }));
}

// decisionStatus maps the sample run's approval decision to the run status every screen shows.
export function decisionStatus(status: RunStatus): RunState {
  return status === 'waiting' ? 'awaiting_approval' : 'completed';
}
