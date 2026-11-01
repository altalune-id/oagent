import { describe, expect, it } from 'vitest';
import { sampleRun } from '~/preview/samples';
import { eventsFor, lane, scale, ticks, toolCalls, totals } from './runView';

describe('runView', () => {
  const run = sampleRun();

  it('puts each event type on one lane, and the input on none', () => {
    expect(run.events.map(lane)).toEqual([null, 'model', 'tool', 'tool', 'model', 'error', 'tool', 'model', 'approval']);
    expect(lane(run.afterApprove[1]!)).toBe('model');
  });

  it('adds what happens after an approval or a rejection', () => {
    expect(eventsFor(run, 'waiting')).toHaveLength(9);
    expect(eventsFor(run, 'approved').at(-1)!.name).toBe('Final reply');
    expect(eventsFor(run, 'rejected').at(-1)!.name).toBe('Rejected');
  });

  it('totals duration, cost and steps, and scales the axis to whole 20 s ticks', () => {
    expect(totals(run.events)).toEqual({ seconds: 19.7, costCents: 1.6, steps: 8 });
    expect(totals(eventsFor(run, 'approved'))).toEqual({ seconds: 77.4, costCents: 2.4, steps: 10 });
    expect(totals([])).toEqual({ seconds: 0, costCents: 0, steps: 0 });
    expect(scale(eventsFor(run, 'approved'))).toBe(80);
    expect(scale([])).toBe(20);
    expect(ticks(80)).toEqual([0, 20, 40, 60, 80]);
  });

  it('counts tool calls and failures per tool', () => {
    expect(toolCalls(run.events)).toEqual([
      { name: 'orders.lookup', calls: 1, failed: 0, seconds: 0.8 },
      { name: 'billing.charges.list', calls: 1, failed: 0, seconds: 1.2 },
      { name: 'billing.refunds.preview', calls: 2, failed: 1, seconds: 5.8 },
    ]);
  });
});
