// @vitest-environment happy-dom
import { create } from '@bufbuild/protobuf';
import { flushPromises } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { useToast } from '#base/components/ui/toast';
import { WhoamiResponseSchema } from '#gen/auth/v1/auth_pb';
import { mountPage } from '#test/mount-page';
import { setClient, setRoute } from '#test/nuxt-globals';
import RunPage from '~/pages/o/[org]/projects/[project]/agents/[agent]/runs/[run].vue';
import RunsPage from '~/pages/o/[org]/projects/[project]/agents/[agent]/runs/index.vue';
import { sampleRun } from '~/preview/samples';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

const run = sampleRun();
const orgId = 'B6rTn1YkM8wQz3XpL5vGd';
const projectId = 'x6MH9WPWjktMwCBKHy4pF';
const yasaku = { id: 'Y4sKu8RmT2pLq6WnX9cVb', projectId, slug: 'yasaku-keuangan', name: 'Yasaku Keuangan', description: '', publishedVersion: 4 };

beforeEach(() => {
  setActivePinia(createPinia());
  useOrgsStore().items = [{ id: orgId, slug: 'acme', name: 'Acme', role: 'owner', permissions: [] }];
  useProjectsStore().items = [{ id: projectId, orgId, slug: 'rumah', name: 'Rumah Tangga' }];
  setClient('$authClient', { whoami: vi.fn().mockResolvedValue(create(WhoamiResponseSchema, { name: 'Budi', permissions: ['agent:read', 'agent:write'] })) });
  setClient('$agentClient', { listAgents: vi.fn().mockResolvedValue({ agents: [yasaku] }) });
  setRoute({ path: `/o/acme/projects/rumah/agents/yasaku-keuangan/runs/${run.id}`, params: { org: 'acme', project: 'rumah', agent: 'yasaku-keuangan', run: run.id } });
});

describe('runs page', () => {
  it('lists one sample run per status with its conversation, detail and usage, filtered by status', async () => {
    const w = await mountPage(RunsPage);
    expect(w.get('.badge').text()).toBe('preview.badge');
    expect(w.get('[role="note"]').text()).toBe('run.preview_note');
    const rows = w.findAll('tbody tr');
    expect(rows).toHaveLength(8);
    expect(rows[1]!.text()).toContain('runstatus.awaiting_approval');
    expect(rows[1]!.text()).toContain('runstatus.detail_awaiting|{"name":"Budi"}');
    expect(rows[1]!.find('a[href]').exists() || rows[1]!.findAll('a').length).toBeTruthy();
    await w.get('select').setValue('failed');
    expect(w.findAll('tbody tr')).toHaveLength(1);
    expect(w.get('tbody').text()).toContain('cust_0001');
  });

  it('shows the empty state for a status without runs', async () => {
    const w = await mountPage(RunsPage);
    const { useThreadsStore } = await import('~/stores/threads');
    useThreadsStore().runs = [];
    await flushPromises();
    expect(w.text()).toContain('runs.empty');
  });
});

describe('run viewer', () => {
  beforeEach(() => {
    vi.stubGlobal('definePageMeta', vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  const stats = (w: Awaited<ReturnType<typeof mountPage>>) => w.findAll('header dl dd').map(d => d.text());
  const button = (w: Awaited<ReturnType<typeof mountPage>>, text: string) => w.findAll('button').find(b => b.text().includes(text));

  it('shows status, duration, cost and steps inline, a link to the conversation, and every event in the minimap', async () => {
    const w = await mountPage(RunPage);
    expect(w.get('h1').text()).toContain(run.id);
    expect(w.get('header p span.font-mono.text-xs').text()).toBe(run.model);
    expect(w.get('header p a').text()).toBe('run.open_thread');
    expect(w.get('[role="note"]').text()).toBe('run.preview_note');
    expect(stats(w)).toEqual(['runstatus.awaiting_approval', '10.8s', '$0.001', 'run.steps_of|{"n":7,"max":12}']);
    expect(w.findAll('button[aria-pressed][aria-label^="run.block_label"]')).toHaveLength(7);
  });

  it('jumps to the approval from the header', async () => {
    const w = await mountPage(RunPage, { attachTo: document.body });
    await button(w, 'run.review_approval')!.trigger('click');
    await flushPromises();
    expect(document.activeElement?.id).toBe('event-e8');
    w.unmount();
  });

  it('shows a model step with its price snapshot and copies a link to an event', async () => {
    const w = await mountPage(RunPage, { attachTo: document.body });
    const model = w.findAll('button[aria-pressed][aria-label^="run.block_label"]').find(b => b.attributes('aria-label')!.includes('run.event_plan'))!;
    await model.trigger('click');
    await flushPromises();
    const inspector = w.get('aside[aria-label="run.inspector"]');
    expect(inspector.get('h2').text()).toBe('run.event_plan');
    expect(inspector.text()).toContain('run.priced_at|{"input":"$0.40","output":"$1.60"}');
    const writeText = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal('navigator', { clipboard: { writeText } });
    await inspector.get('button[aria-label="run.copy_link"]').trigger('click');
    await flushPromises();
    expect(writeText.mock.calls[0]![0]).toMatch(/#event-e2$/);
    expect(useToast().toasts.at(-1)).toMatchObject({ kind: 'success', text: 'common.copied' });
    expect(inspector.findAll('tbody tr').map(r => r.findAll('td').map(d => d.text()).join(' '))).toEqual([
      'yasaku__list_wallets run.tool_calls_failed|{"calls":2,"failed":1}',
      'yasaku__record_expense run.tool_calls|{"calls":1,"seconds":0.5}',
    ]);
    w.unmount();
  });

  it('opens the event named in the link', async () => {
    setRoute({ path: `/o/acme/projects/rumah/agents/yasaku-keuangan/runs/${run.id}`, params: { run: run.id }, hash: '#event-e4' });
    const w = await mountPage(RunPage);
    expect(w.get('aside[aria-label="run.inspector"] h2').text()).toBe('yasaku__list_wallets');
  });

  it('hides an event type from the transcript with its checkbox and shows it again', async () => {
    const w = await mountPage(RunPage);
    const boxes = w.findAll('fieldset input[type="checkbox"]');
    await boxes[3]!.setValue(false);
    expect(w.find('#event-e3').exists()).toBe(false);
    await boxes[3]!.setValue(true);
    expect(w.find('#event-e3').exists()).toBe(true);
  });

  it('approves with the shared card: the run finishes and the totals update', async () => {
    const w = await mountPage(RunPage);
    expect(w.find('input[type="number"]').exists()).toBe(false);
    const card = w.get('#approval section');
    await button(card as never, 'approvalcard.approve_run')!.trigger('click');
    expect(stats(w)[0]).toBe('runstatus.completed');
    expect(stats(w)[1]).toBe('65s');
    expect(card.text()).toContain('approvalcard.approved|{"by":"Budi"} · approvalcard.via_console');
    expect(w.text()).toContain('Tersimpan: Kopi Kenangan');
    expect(button(w, 'run.review_approval')).toBeUndefined();
  });

  it('decides once: a second decision from the card does nothing', async () => {
    const w = await mountPage(RunPage);
    const card = w.findComponent({ name: 'ApprovalCard' });
    card.vm.$emit('approve');
    await flushPromises();
    card.vm.$emit('reject', 'late');
    await flushPromises();
    expect(stats(w)[0]).toBe('runstatus.completed');
    expect(w.text()).toContain('Tersimpan: Kopi Kenangan');
    expect(w.text()).not.toContain('Oke, tidak saya simpan.');
  });

  it('shows the card without decision buttons to people who cannot change agents', async () => {
    setClient('$authClient', { whoami: vi.fn().mockResolvedValue(create(WhoamiResponseSchema, { name: 'Sari', permissions: ['agent:read'] })) });
    const w = await mountPage(RunPage);
    expect(w.get('#approval section').findAll('button').some(b => b.text().includes('approvalcard.approve_run'))).toBe(false);
    expect(w.get('#approval section').text()).toContain('approvalcard.waiting_for|{"name":"Budi"}');
  });

  it('rejects with a reason: the agent replies without saving', async () => {
    const w = await mountPage(RunPage);
    const card = w.get('#approval section');
    await button(card as never, 'approvalcard.reject')!.trigger('click');
    await card.get('input').setValue('salah');
    await button(card as never, 'approvalcard.reject_send')!.trigger('click');
    expect(card.text()).toContain('“salah”');
    expect(w.text()).toContain('Oke, tidak saya simpan.');
  });
});
