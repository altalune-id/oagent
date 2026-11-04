// @vitest-environment happy-dom
import { mount } from '@vue/test-utils';
import { describe, expect, it } from 'vitest';
import ChatTranscript from '~/components/try/ChatTranscript.vue';
import { sampleRun } from '~/preview/samples';

const at = new Date();
const items = [
  { id: 1, kind: 'user' as const, text: 'beli kopi', speaker: 'Budi', at },
  { id: 2, kind: 'tool' as const, name: 'yasaku__list_wallets', args: {}, state: 'done' as const },
  { id: 3, kind: 'approval' as const, approval: sampleRun().approval },
  { id: 4, kind: 'agent' as const, text: 'Tersimpan', streaming: false, at },
];

function sides(viewer: 'person' | 'agent') {
  const w = mount(ChatTranscript, { props: { items, agentName: 'Yasaku', youName: 'You', viewer } });
  const chats = w.findAll('.chat');
  return {
    user: chats[0]!.classes(),
    agent: chats[1]!.classes(),
    tool: w.get('details').classes(),
    card: w.get('section').element.parentElement!.className,
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
});
