import { describe, expect, it } from 'vitest';
import { sampleRun } from '~/preview/samples';
import { eventName, eventsFor, lane, laneColor, scale, ticks, toolCalls, totals } from './runView';

const run = sampleRun();

describe('runView', () => {
  it('puts each event type on one lane, and the input on none', () => {
    expect(run.events.map(lane)).toEqual([null, 'model', 'error', 'tool', 'model', 'tool', 'model', 'approval']);
    expect(Object.keys(laneColor)).toEqual(['model', 'tool', 'approval', 'error']);
  });

  it('labels model steps in the UI language and keeps tool names', () => {
    const t = (key: string) => `t:${key}`;
    expect(run.events.map(e => eventName(e, t)).slice(0, 4)).toEqual(['t:run.event_message', 't:run.event_plan', 'yasaku__list_wallets', 'yasaku__list_wallets']);
  });

  it('adds what happens after an approval or a rejection', () => {
    expect(eventsFor(run, 'waiting')).toHaveLength(8);
    expect(eventsFor(run, 'approved')).toHaveLength(10);
    expect(eventsFor(run, 'rejected').at(-1)!.summary).toBe('Oke, tidak saya simpan.');
  });

  it('totals duration, cost and steps, and scales the axis to whole 20 s ticks', () => {
    expect(totals(run.events)).toEqual({ seconds: 10.8, costCents: 0.1, steps: 7 });
    expect(scale(run.events)).toBe(20);
    expect(ticks(20)).toEqual([0, 20]);
    const approved = eventsFor(run, 'approved');
    expect(totals(approved)).toEqual({ seconds: 65, costCents: 0.2, steps: 9 });
    expect(scale(approved)).toBe(80);
    expect(totals([])).toEqual({ seconds: 0, costCents: 0, steps: 0 });
  });

  it('counts tool calls and failures per tool', () => {
    expect(toolCalls(run.events)).toEqual([
      { name: 'yasaku__list_wallets', calls: 2, failed: 1, seconds: 5.6 },
      { name: 'yasaku__record_expense', calls: 1, failed: 0, seconds: 0.5 },
    ]);
  });
});
