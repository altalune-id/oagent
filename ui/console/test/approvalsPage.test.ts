// @vitest-environment happy-dom
import type { MessageInitShape } from '@bufbuild/protobuf';
import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { flushPromises, RouterLinkStub } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useToast } from '#base/components/ui/toast';
import { ApprovalSchema } from '#gen/approval/v1/approval_pb';
import { WhoamiResponseSchema } from '#gen/auth/v1/auth_pb';
import { mountPage } from '#test/mount-page';
import { connectError, setClient, setRoute } from '#test/nuxt-globals';
import ApprovalsPage from '~/pages/o/[org]/projects/[project]/approvals.vue';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

const orgId = 'B6rTn1YkM8wQz3XpL5vGd';
const projectId = 'x6MH9WPWjktMwCBKHy4pF';
const yasaku = { id: 'Y4sKu8RmT2pLq6WnX9cVb', projectId, slug: 'yasaku-keuangan', name: 'Yasaku Keuangan', description: '', publishedVersion: 4 };
const inMinutes = (n: number) => timestampFromDate(new Date(Date.now() + n * 60_000));
function pb(id: string, over: MessageInitShape<typeof ApprovalSchema> = {}) {
  return create(ApprovalSchema, {
    id,
    projectId,
    agentId: yasaku.id,
    threadId: 'Th7sK2pQ9mXr4StV8nBcW',
    runId: 'Rn2wQ8mX4vZ9tB3nC7kPq',
    status: 'pending',
    channel: 'whatsapp',
    tools: ['yasaku__record_expense'],
    preview: 'yasaku · record_expense\namount: 40000\nwallet: Cash',
    requesterLabel: 'Sari',
    createdAt: inMinutes(-2),
    expiresAt: inMinutes(23 * 60 + 1),
    ...over,
  });
}

function seed(permissions = ['agent:read', 'approval:decide'], query: Record<string, string> = {}, over: Record<string, unknown> = {}) {
  useOrgsStore().items = [{ id: orgId, slug: 'acme', name: 'Acme', role: 'owner', permissions: [] }];
  useProjectsStore().items = [{ id: projectId, orgId, slug: 'rumah', name: 'Rumah Tangga' }];
  setRoute({ path: '/o/acme/projects/rumah/approvals', params: { org: 'acme', project: 'rumah' }, query });
  setClient('$authClient', { whoami: vi.fn().mockResolvedValue(create(WhoamiResponseSchema, { name: 'Budi', permissions })) });
  setClient('$agentClient', { listAgents: vi.fn().mockResolvedValue({ agents: [yasaku] }) });
  const c = {
    listApprovals: vi.fn(async ({ status }: { status: string }) => (status === 'pending'
      ? { approvals: [pb('Ap1')], nextPageToken: '', pendingCount: 1n }
      : status === 'expired'
        ? { approvals: [pb('Ex1', { status: 'expired' })], nextPageToken: '', pendingCount: 1n }
        : { approvals: [pb('Ok1', { status: 'approved', decidedByLabel: 'Sari', decidedVia: 'whatsapp' }), pb('No1', { status: 'rejected', decidedByLabel: 'Budi', decidedVia: 'mcp', decisionText: 'salah' })], nextPageToken: 'p2', pendingCount: 1n })),
    getApproval: vi.fn(async () => ({ approval: pb('Ap1', { status: 'approved', decidedByLabel: 'Sari', decidedVia: 'whatsapp' }) })),
    decideApproval: vi.fn(async ({ id, approved }: { id: string, approved: boolean }) => ({ approval: pb(id, { status: approved ? 'approved' : 'rejected', decidedByLabel: 'Budi', decidedVia: 'console' }) })),
    ...over,
  };
  setClient('$approvalClient', c);
  return c;
}

const button = (w: Awaited<ReturnType<typeof mountPage>>, text: string) => w.findAll('button').find(b => b.text().includes(text));

beforeEach(() => {
  setActivePinia(createPinia());
  vi.stubGlobal('definePageMeta', vi.fn());
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe('approvals page', () => {
  it('lists pending requests from the server as the shared card, with the preview, people, countdown and links, and no preview badge', async () => {
    const c = seed();
    const w = await mountPage(ApprovalsPage);
    await flushPromises();
    expect(c.listApprovals).toHaveBeenCalledWith({ projectId, status: 'pending', pageToken: '', pageSize: 50 });
    expect(w.findAll('nav a').map(a => a.text())).toEqual(['approvals.tab_pending 1', 'approvals.tab_decided', 'approvals.tab_expired']);
    expect(w.text()).not.toContain('preview.badge');
    const summary = w.get('summary').text();
    expect(summary).toContain('Yasaku Keuangan · Sari · approvals.channel_whatsapp');
    expect(summary).toContain('approvals.expires_in_h|{"n":23}');
    const card = w.get('details article');
    expect(card.get('h3').text()).toBe('yasaku · record_expense');
    expect(card.findAll('dt').map(d => d.text())).toEqual(['amount', 'wallet']);
    expect(card.find('.badge-warning').exists()).toBe(false);
    const links = w.findAllComponents(RouterLinkStub).map(l => l.props('to'));
    expect(links).toEqual(expect.arrayContaining([
      '/o/acme/projects/rumah/agents/yasaku-keuangan/threads/Th7sK2pQ9mXr4StV8nBcW',
      '/o/acme/projects/rumah/agents/yasaku-keuangan/runs/Rn2wQ8mX4vZ9tB3nC7kPq',
    ]));
  });

  it.each([['pending'], ['decided']])('links no thread or run on %s when the agent is unknown, instead of putting its public id in the path', async (tab) => {
    seed(['agent:read', 'approval:decide'], { tab });
    setClient('$agentClient', { listAgents: vi.fn().mockResolvedValue({ agents: [] }) });
    const w = await mountPage(ApprovalsPage);
    await flushPromises();
    expect(w.text()).toContain('yasaku · record_expense');
    expect(w.findAllComponents(RouterLinkStub).map(l => String(l.props('to'))).filter(to => to.includes('/agents/'))).toEqual([]);
  });

  it('leaves out a missing requester instead of an empty slot', async () => {
    seed(undefined, {}, { listApprovals: vi.fn().mockResolvedValue({ approvals: [pb('Ap1', { requesterLabel: '' })], nextPageToken: '', pendingCount: 1n }) });
    const w = await mountPage(ApprovalsPage);
    await flushPromises();
    expect(w.get('summary').text()).toContain('Yasaku Keuangan · approvals.channel_whatsapp');
  });

  it('warns in the countdown under an hour, at minute granularity', async () => {
    seed(undefined, {}, { listApprovals: vi.fn().mockResolvedValue({ approvals: [pb('Ap1', { expiresAt: inMinutes(4) })], nextPageToken: '', pendingCount: 1n }) });
    const w = await mountPage(ApprovalsPage);
    await flushPromises();
    const soon = w.get('details article .badge-warning');
    expect(soon.text()).toMatch(/^approvals\.expires_in_min\|\{"n":[45]\}$/);
  });

  it('approves through the RPC, says so, and moves focus to the list', async () => {
    const c = seed();
    const w = await mountPage(ApprovalsPage, { attachTo: document.body });
    await flushPromises();
    await button(w, 'approvalcard.approve_run')!.trigger('click');
    await flushPromises();
    expect(c.decideApproval).toHaveBeenCalledWith({ id: 'Ap1', approved: true, text: '' });
    expect(useToast().toasts.at(-1)).toMatchObject({ kind: 'success', text: 'approvals.approved_toast|{"title":"yasaku · record_expense"}' });
    expect(w.text()).toContain('approvals.empty_pending');
    expect(document.activeElement?.textContent).toContain('approvals.empty_pending');
    w.unmount();
  });

  it('keeps a card decided elsewhere in place with who decided it, and says the reason was not sent', async () => {
    seed(undefined, {}, { decideApproval: vi.fn().mockRejectedValue(connectError('APR003', { status: 'approved' })) });
    const w = await mountPage(ApprovalsPage);
    await flushPromises();
    await button(w, 'approvalcard.reject')!.trigger('click');
    await w.get('details article input').setValue('salah');
    await button(w, 'approvalcard.reject_send')!.trigger('click');
    await flushPromises();
    const card = w.get('details article');
    expect(card.text()).toContain('approvalcard.approved|{"by":"Sari"} · approvalcard.via_whatsapp');
    expect(card.get('[data-testid="card-note"]').text()).toBe('approvals.reason_not_sent');
    expect(card.findAll('button').some(b => b.text().includes('approvalcard.approve_run'))).toBe(false);
    await button(w, 'approvals.dismiss')!.trigger('click');
    expect(w.text()).toContain('approvals.empty_pending');
  });

  it('says a request expired before the decision, and shows other failures as an error toast', async () => {
    seed(undefined, {}, { decideApproval: vi.fn().mockRejectedValueOnce(connectError('APR003', { status: 'expired' })) });
    const w = await mountPage(ApprovalsPage);
    await flushPromises();
    await button(w, 'approvalcard.approve_run')!.trigger('click');
    await flushPromises();
    expect(w.get('[data-testid="card-note"]').text()).toBe('approvals.expired_before');

    setActivePinia(createPinia());
    seed(undefined, {}, { decideApproval: vi.fn().mockRejectedValue(new Error('offline')) });
    const x = await mountPage(ApprovalsPage);
    await flushPromises();
    await button(x, 'approvalcard.approve_run')!.trigger('click');
    await flushPromises();
    expect(useToast().toasts.at(-1)).toMatchObject({ kind: 'error', text: 'error.decide_approval' });
  });

  it('shows an approval that is gone as an info toast', async () => {
    seed(undefined, {}, { decideApproval: vi.fn().mockRejectedValue(connectError('APR002')) });
    const w = await mountPage(ApprovalsPage);
    await flushPromises();
    await button(w, 'approvalcard.approve_run')!.trigger('click');
    await flushPromises();
    expect(useToast().toasts.at(-1)).toMatchObject({ kind: 'info' });
  });

  it('answers an API conversation\'s approval only from its app: no decide buttons, even with approval:decide', async () => {
    seed(undefined, {}, { listApprovals: vi.fn().mockResolvedValue({ approvals: [pb('Api1', { channel: 'api' }), pb('Wa1')], nextPageToken: '', pendingCount: 2n }) });
    const w = await mountPage(ApprovalsPage);
    await flushPromises();
    const [api, wa] = w.findAll('details article');
    expect(w.find('details section').exists(), 'a card is not a landmark, so two cards for one tool never repeat a landmark name').toBe(false);
    expect(api!.text()).toContain('approvals.answered_by_app');
    expect(api!.findAll('button').some(b => b.text().includes('approvalcard.approve_run'))).toBe(false);
    expect(wa!.findAll('button').some(b => b.text().includes('approvalcard.approve_run'))).toBe(true);
  });

  it('shows no decision buttons without approval:decide', async () => {
    seed(['agent:read', 'agent:write']);
    const w = await mountPage(ApprovalsPage);
    await flushPromises();
    expect(w.text()).toContain('approvals.cant_decide');
    expect(w.findAll('button').some(b => b.text().includes('approvalcard.approve_run'))).toBe(false);
  });

  it('lists decided requests with who decided and where, loads more, and falls back to Pending for an unknown tab', async () => {
    const c = seed(undefined, { tab: 'decided' });
    const w = await mountPage(ApprovalsPage);
    await flushPromises();
    const rows = w.findAll('tbody tr');
    expect(rows.map(r => r.text().includes('approvalcard.approved|{"by":"Sari"} · approvalcard.via_whatsapp'))).toEqual([true, false]);
    expect(rows[1]!.text()).toContain('approvalcard.rejected|{"by":"Budi"} · approvalcard.via_mcp');
    expect(rows[1]!.text()).toContain('“salah”');
    await button(w, 'approvals.load_more')!.trigger('click');
    await flushPromises();
    expect(c.listApprovals).toHaveBeenLastCalledWith({ projectId, status: 'decided', pageToken: 'p2', pageSize: 50 });
    setRoute({ path: '/o/acme/projects/rumah/approvals', params: { org: 'acme', project: 'rumah' }, query: { tab: 'expired' } });
    await flushPromises();
    expect(w.get('tbody').text()).toContain('approvalcard.expired');
    expect(w.get('tbody').text()).toContain('approvals.expired_at|');
    expect(w.get('tbody').text()).not.toContain('approvals.decided|');
    setRoute({ path: '/o/acme/projects/rumah/approvals', params: { org: 'acme', project: 'rumah' }, query: { tab: 'bogus' } });
    await flushPromises();
    expect(w.findAll('nav a')[0]!.attributes('aria-current')).toBe('page');
  });

  it('refreshes Pending every 15 s: announces new requests once and leaves a card being typed in alone', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const c = seed();
    const w = await mountPage(ApprovalsPage, { attachTo: document.body });
    await flushPromises();
    await button(w, 'approvalcard.reject')!.trigger('click');
    w.get('[data-approval="Ap1"]').trigger('focusin');
    c.listApprovals.mockResolvedValue({ approvals: [pb('New1'), pb('New2')], nextPageToken: '', pendingCount: 2n });
    await vi.advanceTimersByTimeAsync(15_000);
    await flushPromises();
    expect(w.findAll('li[data-approval]').map(l => l.attributes('data-approval'))).toEqual(['New1', 'New2', 'Ap1']);
    expect(w.get('p[role="status"].sr-only').text()).toBe('approvals.new_count|{"n":2}');
    expect(w.findAll('nav a')[0]!.text()).toBe('approvals.tab_pending 2');
    w.unmount();
  });
});
