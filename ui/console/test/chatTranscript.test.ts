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
