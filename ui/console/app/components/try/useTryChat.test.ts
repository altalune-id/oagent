import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { effectScope } from 'vue';
import { sampleChatScript } from '~/preview/samples';
import { phaseLabel } from './phase';
import { maxPhotoBytes, photoError, scenarios, useTryChat } from './useTryChat';

const timing = { think: 10, tool: 10, tick: 1, chars: 50 };
const usd = (cents: number) => `usd:${cents}`;

function make(maxCostCents = 20, maxSteps = 12) {
  return useTryChat({ maxSteps: () => maxSteps, maxCostCents: () => maxCostCents, money: usd, decider: () => 'Budi' }, timing, sampleChatScript);
}

const kinds = (chat: ReturnType<typeof make>) => chat.items.value.map(i => i.kind);

describe('useTryChat', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('plays the coffee expense up to the approval, then saves it on Approve', async () => {
    const chat = make();
    const run = chat.send('beli kopi kenangan 40rb pakai cash');
    expect(chat.phase.value).toBe('thinking');
    expect(chat.busy.value).toBe(true);
    await vi.runAllTimersAsync();
    await run;
    expect(kinds(chat)).toEqual(['user', 'tool', 'tool', 'tool', 'approval']);
    expect(chat.phase.value).toBe('awaiting');
    expect(chat.awaiting.value?.approval.title).toBe('Simpan Rp40.000 dari Cash?');
    expect(chat.toolCalls.value.map(c => [c.name, c.state])).toEqual([['yasaku__list_wallets', 'done'], ['yasaku__record_expense', 'preview'], ['yasaku__record_expense', 'awaiting']]);
    expect(chat.steps.value).toBe(3);
    expect(chat.tokens.value).toBe(3 * 812);
    const approved = chat.decide(true);
    await vi.runAllTimersAsync();
    await approved;
    expect(chat.phase.value).toBe('completed');
    expect(kinds(chat)).toEqual(['user', 'tool', 'tool', 'approval', 'tool', 'agent']);
    expect(chat.toolCalls.value.map(c => c.state)).toEqual(['done', 'preview', 'done']);
    const executed = chat.items.value[4]!;
    expect(executed.kind === 'tool' && [executed.name, executed.state]).toEqual(['yasaku__record_expense', 'done']);
    const card = chat.items.value[3]!;
    expect(card.kind === 'approval' && card.approval).toMatchObject({ status: 'approved', decidedBy: 'Budi', decidedVia: { channel: 'try' } });
    const reply = chat.items.value.at(-1)!;
    expect(reply.kind === 'agent' && reply.text).toBe('Tersimpan: Kopi Kenangan Rp40.000 dari Cash. Sisa saldo Cash Rp1.210.000.');
    expect(chat.costCents.value).toBe(2);
    expect(await chat.decide(true)).toBeUndefined();
  });

  it('sends the rejection reason back and ignores sends while waiting or empty', async () => {
    const chat = make();
    void chat.send('Transfer 500rb dari BCA ke Cash');
    await vi.runAllTimersAsync();
    await chat.send('more');
    await chat.send('   ');
    expect(kinds(chat)).toEqual(['user', 'tool', 'tool', 'tool', 'approval']);
    void chat.decide(false, ' salah dompet ');
    await vi.runAllTimersAsync();
    expect(chat.toolCalls.value.map(c => c.state)).toEqual(['done', 'preview', 'rejected']);
    const card = chat.items.value[4]!;
    expect(card.kind === 'approval' && [card.approval.status, card.approval.reason]).toEqual(['rejected', 'salah dompet']);
    const reply = chat.items.value.at(-1)!;
    expect(reply.kind === 'agent' && reply.text).toBe('Oke, transfernya tidak saya catat.');
  });

  it('answers a report question without an approval', async () => {
    const chat = make();
    void chat.send('Pengeluaran makan bulan ini berapa?');
    await vi.runAllTimersAsync();
    expect(kinds(chat)).toEqual(['user', 'tool', 'agent']);
    expect(chat.phase.value).toBe('completed');
  });

  it('shows a failed model call with Retry, which plays the normal reply', async () => {
    const chat = make();
    chat.scenario.value = 'model_fails';
    void chat.send('Pengeluaran makan bulan ini berapa?');
    await vi.runAllTimersAsync();
    expect(chat.phase.value).toBe('failed');
    expect(chat.items.value.at(-1)).toMatchObject({ kind: 'agent', failed: true });
    void chat.retry();
    await vi.runAllTimersAsync();
    expect(chat.scenario.value).toBe('normal');
    expect(kinds(chat)).toEqual(['user', 'tool', 'agent']);
    expect(await chat.retry()).toBeUndefined();
  });

  it('marks a failed tool and goes on', async () => {
    const chat = make();
    chat.scenario.value = 'tool_fails';
    void chat.send('Pengeluaran makan bulan ini berapa?');
    await vi.runAllTimersAsync();
    expect(chat.toolCalls.value.map(c => c.state)).toEqual(['failed']);
    expect(chat.phase.value).toBe('completed');
  });

  it('stops at the cost cap, or at the step cap when there is no cost cap', async () => {
    const chat = make(20);
    chat.scenario.value = 'cost_limit';
    void chat.send('Pengeluaran makan bulan ini berapa?');
    await vi.runAllTimersAsync();
    expect(chat.phase.value).toBe('limit');
    expect(chat.items.value.at(-1)).toMatchObject({ kind: 'notice', key: 'try.limit_cost', params: { cap: 'usd:20' } });
    expect(chat.costCents.value).toBe(20);
    const steps = make(0, 12);
    steps.scenario.value = 'cost_limit';
    void steps.send('Pengeluaran makan bulan ini berapa?');
    await vi.runAllTimersAsync();
    expect(steps.items.value.at(-1)).toMatchObject({ key: 'try.limit_steps', params: { n: '12' } });
  });

  it('stops a normal reply once it reaches the step cap', async () => {
    const chat = make(0, 1);
    void chat.send('beli kopi kenangan 40rb pakai cash');
    await vi.runAllTimersAsync();
    expect(chat.steps.value).toBe(1);
    expect(chat.phase.value).toBe('limit');
    expect(chat.items.value.at(-1)).toMatchObject({ kind: 'notice', key: 'try.limit_steps', params: { n: '1' } });
  });

  it('notes a dropped stream and finishes the reply on Reload', async () => {
    const chat = useTryChat({ maxSteps: () => 0, maxCostCents: () => 0, money: usd }, { ...timing, chars: 5 }, sampleChatScript);
    chat.scenario.value = 'dropped';
    void chat.send('Pengeluaran makan bulan ini berapa?');
    await vi.runAllTimersAsync();
    expect(chat.phase.value).toBe('dropped');
    expect(chat.items.value.at(-1)).toMatchObject({ kind: 'notice', key: 'try.dropped' });
    chat.reload();
    expect(chat.phase.value).toBe('completed');
    const reply = chat.items.value.at(-1)!;
    expect(reply.kind === 'agent' && reply.text).toBe(sampleChatScript('makan').steps[1]!.kind === 'text' ? (sampleChatScript('makan').steps[1] as { text: string }).text : '');
  });

  it('stops a reply on request and starts over on New chat', async () => {
    const chat = make();
    void chat.send('beli kopi');
    await vi.advanceTimersByTimeAsync(15);
    expect(chat.toolCalls.value.map(c => c.state)).toEqual(['running']);
    chat.stop();
    expect(chat.phase.value).toBe('cancelled');
    expect(chat.toolCalls.value.map(c => c.state)).toEqual(['failed']);
    expect(chat.items.value.at(-1)).toMatchObject({ kind: 'notice', key: 'try.stopped' });
    await vi.runAllTimersAsync();
    expect(chat.phase.value).toBe('cancelled');
    chat.stop();
    chat.newChat();
    expect([chat.items.value, chat.phase.value, chat.steps.value, chat.tokens.value, chat.costCents.value]).toEqual([[], 'idle', 0, 0, 0]);
  });

  it('changes nothing after its scope ends mid-stream, and a stopped stream never resumes', async () => {
    const scope = effectScope();
    const chat = scope.run(() => useTryChat({ maxSteps: () => 0, maxCostCents: () => 0, money: usd }, { ...timing, chars: 1 }, sampleChatScript))!;
    void chat.send('Pengeluaran makan bulan ini berapa?');
    await vi.advanceTimersByTimeAsync(40);
    const bubble = chat.items.value.find(i => i.kind === 'agent')!;
    const text = bubble.kind === 'agent' ? bubble.text : '';
    scope.stop();
    expect(vi.getTimerCount()).toBe(0);
    await vi.advanceTimersByTimeAsync(1000);
    expect(bubble.kind === 'agent' && bubble.text).toBe(text);
    const again = make();
    void again.send('Pengeluaran makan bulan ini berapa?');
    await vi.advanceTimersByTimeAsync(40);
    again.stop();
    const items = JSON.stringify(again.items.value);
    await vi.runAllTimersAsync();
    expect(JSON.stringify(again.items.value)).toBe(items);
  });

  it('stops a streaming bubble', async () => {
    const chat = useTryChat({ maxSteps: () => 0, maxCostCents: () => 0, money: usd }, { ...timing, chars: 1 }, sampleChatScript);
    void chat.send('Pengeluaran makan bulan ini berapa?');
    await vi.advanceTimersByTimeAsync(40);
    expect(chat.phase.value).toBe('streaming');
    chat.stop();
    const bubble = chat.items.value.find(i => i.kind === 'agent')!;
    expect(bubble.kind === 'agent' && bubble.streaming).toBe(false);
    await vi.runAllTimersAsync();
    expect(chat.phase.value).toBe('cancelled');
  });

  it('revokes photo URLs on re-attach, detach, New chat and when its scope ends', () => {
    let n = 0;
    const createObjectURL = vi.fn(() => `blob:${++n}`);
    const revokeObjectURL = vi.fn();
    vi.stubGlobal('URL', Object.assign(URL, { createObjectURL, revokeObjectURL }));
    const scope = effectScope();
    const chat = scope.run(() => make())!;
    const jpg = (name: string) => ({ name, size: 10, type: 'image/jpeg' }) as File;
    chat.attach(jpg('a.jpg'));
    chat.attach(jpg('b.jpg'));
    expect(revokeObjectURL).toHaveBeenLastCalledWith('blob:1');
    chat.detach();
    expect(revokeObjectURL).toHaveBeenLastCalledWith('blob:2');
    chat.attach(jpg('c.jpg'));
    void chat.send('struk');
    chat.newChat();
    expect(revokeObjectURL).toHaveBeenLastCalledWith('blob:3');
    chat.attach(jpg('d.jpg'));
    scope.stop();
    expect(revokeObjectURL).toHaveBeenLastCalledWith('blob:4');
    vi.unstubAllGlobals();
  });

  it('attaches one allowed photo to the next message and refuses others', async () => {
    const createObjectURL = vi.fn().mockReturnValue('blob:receipt');
    vi.stubGlobal('URL', Object.assign(URL, { createObjectURL, revokeObjectURL: vi.fn() }));
    const chat = make();
    chat.attach({ name: 'struk.pdf', size: 10, type: 'application/pdf' } as File);
    expect(chat.photoProblem.value).toBe('try.photo_type');
    chat.attach({ name: 'big.jpg', size: maxPhotoBytes + 1, type: 'image/jpeg' } as File);
    expect(chat.photoProblem.value).toBe('try.photo_too_big');
    chat.attach({ name: 'struk.jpg', size: 1000, type: 'image/jpeg' } as File);
    expect(chat.photo.value).toEqual({ name: 'struk.jpg', url: 'blob:receipt' });
    void chat.send('ini struknya');
    expect(chat.items.value[0]).toMatchObject({ kind: 'user', photo: { name: 'struk.jpg' } });
    expect(chat.photo.value).toBeNull();
    chat.attach({ name: 'b.png', size: 1, type: 'image/png' } as File);
    chat.detach();
    expect([chat.photo.value, chat.photoProblem.value]).toEqual([null, '']);
    await vi.runAllTimersAsync();
  });
});

describe('try helpers', () => {
  it('refuses photos by type and size', () => {
    expect(photoError({ size: 1, type: 'image/webp' })).toBe('');
    expect(photoError({ size: maxPhotoBytes, type: 'image/png' })).toBe('');
  });

  it('names every scenario and phase', () => {
    expect(scenarios).toHaveLength(5);
    expect(phaseLabel('awaiting')).toEqual({ key: 'runstatus.awaiting_approval', tone: 'warning' });
    expect(phaseLabel('dropped').tone).toBe('warning');
  });
});
