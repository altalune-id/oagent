import { describe, expect, it } from 'vitest';
import { sampleThreads } from '~/preview/samples';
import { channelFilters, channelOf, filterFromQuery, filterQuery, filterThreads, runStatuses, runStatusTone } from './conversation';

const threads = sampleThreads();
const titles = (f: Parameters<typeof filterThreads>[1]) => filterThreads(threads, f).map(t => t.id);

describe('conversation helpers', () => {
  it('gives every run status a tone and groups WhatsApp kinds', () => {
    expect(runStatuses).toHaveLength(8);
    expect(runStatusTone.awaiting_approval).toBe('warning');
    expect(channelFilters).toEqual(['whatsapp', 'api', 'try']);
    expect(['whatsapp_dm', 'whatsapp_group', 'api', 'try'].map(k => channelOf(k as never))).toEqual(['whatsapp', 'whatsapp', 'api', 'try']);
  });

  it('reads and writes the filter in the URL query, ignoring unknown values', () => {
    expect(filterFromQuery({ channel: 'api', status: 'failed', q: 'kopi' })).toEqual({ channel: 'api', status: 'failed', query: 'kopi' });
    expect(filterFromQuery({ channel: 'fax', status: 'odd', q: ['x'] })).toEqual({ channel: '', status: '', query: '' });
    expect(filterQuery({ channel: 'try', status: '', query: '  ' })).toEqual({ channel: 'try' });
    expect(filterQuery({ channel: '', status: 'running', query: ' kopi ' })).toEqual({ status: 'running', q: 'kopi' });
  });

  it('filters by channel, status and words in the person or last message', () => {
    expect(titles({ channel: '', status: '', query: '' })).toHaveLength(4);
    expect(titles({ channel: 'whatsapp', status: '', query: '' })).toEqual(['thr_grp3Kd9Q', 'thr_dm8Kq2Wx']);
    expect(titles({ channel: '', status: 'failed', query: '' })).toEqual(['thr_api5Tn1R']);
    expect(titles({ channel: '', status: '', query: ' KOPI ' })).toEqual(['thr_dm8Kq2Wx']);
    expect(titles({ channel: 'api', status: 'running', query: '' })).toEqual([]);
  });

  it('dates the sample parking expense the day before the message that deletes it', () => {
    const items = sampleThreads()[0]!.items;
    const asked = items.find(i => i.kind === 'user' && i.text.includes('parkir'))!;
    const card = items.find(i => i.kind === 'approval' && i.approval.tool === 'yasaku__delete_tx')!;
    const day = new Date((asked as { at: Date }).at.getTime() - 86_400_000);
    expect(card.kind === 'approval' && card.approval.args[0]!.value).toMatch(new RegExp(`, ${day.getDate()} \\S+ ${day.getFullYear()}$`));
  });
});
