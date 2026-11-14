// @vitest-environment happy-dom
import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import ChatTranscript from '~/components/try/ChatTranscript.vue';
import { kopiApproval } from '~/preview/samples';

const at = new Date();
const items = [
  { id: 1, kind: 'user' as const, text: 'beli kopi', speaker: 'Budi', at },
  { id: 2, kind: 'tool' as const, name: 'yasaku__list_wallets', args: {}, state: 'done' as const },
  { id: 3, kind: 'approval' as const, approval: kopiApproval() },
  { id: 4, kind: 'agent' as const, text: 'Tersimpan', streaming: false, at },
];

function sides(viewer: 'person' | 'agent') {
  const w = mount(ChatTranscript, { props: { items, agentName: 'Yasaku', youName: 'You', viewer } });
  const chats = w.findAll('.chat');
  return {
    user: chats[0]!.classes(),
    agent: chats[1]!.classes(),
    tool: w.get('details').classes(),
    card: w.get('article').element.parentElement!.className,
    userBubble: chats[0]!.get('.chat-bubble').classes(),
  };
}

describe('chat transcript sides', () => {
  it('puts the tester on the right and the agent, its tools and approvals on the left in Try it', () => {
    const s = sides('person');
    expect(s.user).toContain('chat-end');
    expect(s.agent).toContain('chat-start');
    expect(s.tool).toContain('self-start');
    expect(s.card).toContain('self-start');
    expect(s.userBubble).toContain('bg-base-300');
  });

  it('puts the people on the left and the agent, its tools and approvals on the right in a conversation', () => {
    const s = sides('agent');
    expect(s.user).toContain('chat-start');
    expect(s.agent).toContain('chat-end');
    expect(s.tool).toContain('self-end');
    expect(s.card).toContain('self-end');
    expect(s.userBubble).not.toContain('bg-base-300');
  });

  it('names a tool chip by its visible text, spaced, with the toggle hint for screen readers only', () => {
    const w = mount(ChatTranscript, { props: { items, agentName: 'Yasaku', youName: 'You' } });
    const chip = w.get('summary');
    expect(chip.attributes('aria-label')).toBeUndefined();
    expect(chip.text()).toMatch(/^yasaku__list_wallets\s+try\.tool_done\s*try\.tool_chip_hint$/);
    expect(chip.get('.sr-only').text()).toBe('try.tool_chip_hint');
  });

  it('shows a run under one cent as less than a cent with the exact cost as its title, and an unpriced run as Unpriced', () => {
    const run = { id: 'r1', threadId: 't1', threadTitle: 'T', status: 'completed' as const, startedAt: at, seconds: 14, tokens: 413, costCents: 0.043, estimated: true };
    const runItems = [{ id: 1, kind: 'run' as const, run }, { id: 2, kind: 'run' as const, run: { ...run, id: 'r2', costCents: 0, unpriced: true, estimated: false } }];
    const w = mount(ChatTranscript, { props: { items: runItems, agentName: 'Yasaku', youName: 'You', viewer: 'agent' } });
    const priced = w.find('[title="$0.00043"]');
    expect(priced.text()).toBe('< $0.01');
    expect(w.text()).toContain('thread.tokens_estimated');
    expect(w.text()).toContain('thread.unpriced');
  });

  it('labels a run on the unpublished draft as Draft, and only that run', () => {
    const run = { id: 'r1', threadId: 't1', threadTitle: 'T', status: 'completed' as const, startedAt: at, seconds: 1, tokens: 10, costCents: 1, draft: true };
    const w = mount(ChatTranscript, { props: { items: [{ id: 1, kind: 'run' as const, run }, { id: 2, kind: 'run' as const, run: { ...run, id: 'r2', draft: false } }], agentName: 'Yasaku', youName: 'You', viewer: 'agent' } });
    const rows = w.findAll('a');
    expect(rows.map(r => r.find('.badge').exists() && r.get('.badge').text())).toEqual(['run.draft', false]);
  });
});

describe('chat transcript voice notes', () => {
  const user = (voice: object | undefined, extra: object = {}) => ({ id: 1, kind: 'user' as const, text: '', speaker: 'Budi', at, ...(voice ? { voice } : {}), ...extra });
  const mountOne = (item: ReturnType<typeof user>) => mount(ChatTranscript, { props: { items: [item], agentName: 'Yasaku', youName: 'You', viewer: 'agent' as const } });

  it('shows a transcribed note as its length and its words, with no badge', () => {
    const w = mountOne(user({ status: 'done', text: 'catat kopi dua puluh ribu', seconds: 14 }, { text: 'struk' }));
    const strip = w.get('[data-voice-strip]');
    expect(strip.text()).toBe('thread.voice_label|{"duration":"0:14"}');
    expect(strip.get('[aria-hidden="true"]').exists()).toBe(true);
    const transcript = w.get('[data-voice-transcript]');
    expect(transcript.text()).toBe('thread.voice_transcriptcatat kopi dua puluh ribu');
    expect(transcript.element.textContent).toBe('thread.voice_transcriptcatat kopi dua puluh ribu');
    expect(transcript.get('.sr-only').text()).toBe('thread.voice_transcript');
    expect(w.get('.chat-bubble').text()).toContain('struk');
    expect(w.find('.chat-header .badge').exists()).toBe(false);
  });

  it('shows why a note was not transcribed as a header badge, tinted only when an owner can fix it', () => {
    const tooLong = mountOne(user({ status: 'skipped', reason: 'too_long' }));
    expect(tooLong.get('[data-voice-strip]').text()).toBe('thread.voice_placeholder');
    expect(tooLong.find('[data-voice-transcript]').exists()).toBe(false);
    expect(tooLong.get('.chat-header .badge').text()).toBe('thread.voice_too_long');
    expect(tooLong.get('.chat-header .badge').classes()).toContain('badge-ghost');
    expect(tooLong.get('.chat-header').classes()).toEqual(expect.arrayContaining(['flex-wrap', 'gap-y-1']));
    const budget = mountOne(user({ status: 'skipped', reason: 'over_budget' })).get('.chat-header .badge');
    expect(budget.text()).toBe('thread.voice_over_budget');
    expect(budget.classes()).toEqual(expect.arrayContaining(['badge-soft', 'badge-warning']));
    expect(budget.classes()).not.toContain('badge-ghost');
    expect(mountOne(user({ status: 'failed', reason: 'something_new' })).get('.chat-header .badge').text()).toBe('thread.voice_unknown');
  });

  it('shows a note with no transcript state as the plain strip, with no badge', () => {
    const w = mountOne(user({}));
    expect(w.get('[data-voice-strip]').text()).toBe('thread.voice_placeholder');
    expect(w.find('.chat-header .badge').exists()).toBe(false);
    expect(mountOne(user(undefined, { text: 'halo' })).find('[data-voice-strip]').exists()).toBe(false);
  });

  it('hides the strip and the transcript of a deleted message', () => {
    const w = mountOne(user({ status: 'done', text: 'rahasia', seconds: 3 }, { deleted: true }));
    expect(w.find('[data-voice-strip]').exists()).toBe(false);
    expect(w.text()).not.toContain('rahasia');
    expect(w.text()).toContain('chat.message_deleted');
  });
});
