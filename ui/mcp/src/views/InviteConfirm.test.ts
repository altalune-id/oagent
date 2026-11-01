// @vitest-environment happy-dom
import type { ToolCaller } from './invite';
import { flushPromises, mount } from '@vue/test-utils';
import { describe, expect, it, vi } from 'vitest';
import InviteConfirm from './InviteConfirm.vue';

const data = {
  invite: { id: 'inv1', email: 'dina@x.io', role: 'member', status: 'draft', invitedByName: 'Olga' },
  orgId: 'org1',
  orgName: 'Acme',
  consoleUrl: 'https://console.example.com/o/acme/members',
};

function view(extra: Record<string, unknown>, reply: Awaited<ReturnType<ToolCaller['callServerTool']>>) {
  const app: ToolCaller = { callServerTool: vi.fn(async () => reply) };
  return mount(InviteConfirm, { props: { data: { ...data, ...extra } }, global: { provide: { mcpApp: app } } });
}

describe('invite confirm view', () => {
  it('sends from the console, not here, when this caller cannot confirm', () => {
    const w = view({ canConfirm: false }, {});
    expect(w.text()).not.toContain('Confirm and send');
    expect(w.text()).toContain('Send this one from Members in the console.');
    expect(w.text()).toContain('https://console.example.com/o/acme/members');
    expect(w.text()).toContain('Discard');
  });

  it('turns a GEN007 refusal into the console handoff, never the raw error', async () => {
    const w = view({}, { isError: true, content: [{ type: 'text', text: '{"code":"GEN007","message":"Forbidden"}' }] });
    await w.findAll('button').find(b => b.text().includes('Confirm and send'))!.trigger('click');
    await flushPromises();
    expect(w.text()).not.toContain('GEN007');
    expect(w.text()).not.toContain('Confirm and send');
    expect(w.text()).toContain('Send this one from Members in the console.');
  });
});
