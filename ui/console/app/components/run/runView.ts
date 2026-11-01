import type { RunEvent, RunEventType, SampleRun } from '~/preview/samples';

export type Lane = 'model' | 'tool' | 'approval' | 'error';
export type RunStatus = 'waiting' | 'approved' | 'rejected';

export const lanes: Lane[] = ['model', 'tool', 'approval', 'error'];

const laneOf: Record<RunEventType, Lane | null> = { input: null, model: 'model', output: 'model', tool: 'tool', approval: 'approval', error: 'error' };

export function lane(event: RunEvent): Lane | null {
  return laneOf[event.type];
}

export function eventsFor(run: SampleRun, status: RunStatus): RunEvent[] {
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
