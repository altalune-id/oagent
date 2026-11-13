// @vitest-environment happy-dom
import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { flushPromises, RouterLinkStub } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { fakeRuntimeClients } from '~~/test/fixtures/runtime';
import { useToast } from '#base/components/ui/toast';
import { WhoamiResponseSchema } from '#gen/auth/v1/auth_pb';
import { EndUserSchema, EndUserThreadSchema, IdentityKind } from '#gen/enduser/v1/enduser_pb';
import { MemoryBackend, MemorySchema, MemoryStoreSchema } from '#gen/memory/v1/memory_pb';
import { mountPage } from '#test/mount-page';
import { connectError, route, setClient, setKnownKeys, setRoute } from '#test/nuxt-globals';
import EndUserPage from '~/pages/o/[org]/projects/[project]/end-users/[endUser].vue';
import EndUsersPage from '~/pages/o/[org]/projects/[project]/end-users/index.vue';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

const orgId = 'B6rTn1YkM8wQz3XpL5vGd';
const projectId = 'x6MH9WPWjktMwCBKHy4pF';
const owner = ['project:read', 'project:write'];

function seed(path: string, params: Record<string, string> = {}, permissions = owner) {
  useOrgsStore().items = [{ id: orgId, slug: 'acme', name: 'Acme', role: 'owner', permissions: [] }];
  useProjectsStore().items = [{ id: projectId, orgId, slug: 'rumah', name: 'Rumah Tangga' }];
  setRoute({ path: `/o/acme/projects/rumah${path}`, params: { org: 'acme', project: 'rumah', ...params } });
  setClient('$authClient', { whoami: vi.fn().mockResolvedValue(create(WhoamiResponseSchema, { name: 'Budi', permissions })) });
}

beforeEach(() => {
  setActivePinia(createPinia());
  fakeRuntimeClients();
});

describe('end users list', () => {
  const at = (min: number) => timestampFromDate(new Date(Date.UTC(2026, 10, 5, 5, min)));
  const sari = create(EndUserSchema, { id: 'Eu5sR2kP8mQx4StV7nBcW', projectId, displayName: 'Sari', identityKind: IdentityKind.WA_PHONE, identityHint: '90', firstSeenAt: at(1), lastSeenAt: at(9), threadCount: 2n });
  const anon = create(EndUserSchema, { id: 'Eu7tH3kQ9wXr2MnV5bCzD', projectId, identityKind: IdentityKind.WA_LID, identityHint: '4417', threadCount: 1n });

  it('lists people by name or masked identity, with no preview, and searches on the server', async () => {
    vi.useFakeTimers();
    const c = { listEndUsers: vi.fn().mockResolvedValueOnce({ endUsers: [sari, anon], nextPageToken: '' }).mockResolvedValue({ endUsers: [], nextPageToken: '' }) };
    setClient('$endUserClient', c);
    seed('/end-users');
    const w = await mountPage(EndUsersPage);
    await flushPromises();
    expect(w.find('.badge-info').exists()).toBe(false);
    expect(w.text()).not.toContain('preview');
    const rows = w.findAll('tbody tr');
    expect(rows[0]!.text()).toContain('Sari');
    expect(rows[0]!.text()).toContain('person.kind_wa_phone …90');
    expect(rows[1]!.text()).toContain('person.masked|{"kind":"person.kind_wa_lid","hint":"4417"}');
    expect(w.findAllComponents(RouterLinkStub)[0]!.props('to')).toBe(`/o/acme/projects/rumah/end-users/${sari.id}`);
    await w.get('input[type="search"]').setValue(' zzz ');
    await vi.advanceTimersByTimeAsync(300);
    await flushPromises();
    expect(c.listEndUsers).toHaveBeenLastCalledWith({ projectId, query: 'zzz', pageSize: 200 });
    expect(w.text()).toContain('endusers.empty_search|{"query":"zzz"}');
    vi.useRealTimers();
  });

  it('shows an empty project, and a failure with retry', async () => {
    setKnownKeys('error.load_end_users');
    const c = { listEndUsers: vi.fn().mockRejectedValueOnce(new Error('offline')).mockResolvedValue({ endUsers: [], nextPageToken: '' }) };
    setClient('$endUserClient', c);
    seed('/end-users');
    const w = await mountPage(EndUsersPage);
    await flushPromises();
    expect(w.get('[role="alert"]').text()).toContain('error.load_end_users');
    await w.findAll('button').find(b => b.text() === 'common.retry')!.trigger('click');
    await flushPromises();
    expect(w.text()).toContain('endusers.empty');
  });
});

describe('end user detail', () => {
  const agentId = 'Ag7pQ2wXk9MmT4vNc8RbZ';
  const sari = create(EndUserSchema, { id: 'Eu5sR2kP8mQx4StV7nBcW', projectId, displayName: '', identityKind: IdentityKind.WA_PHONE, identityHint: '90', firstSeenAt: timestampFromDate(new Date(Date.UTC(2026, 10, 1))), threadCount: 2n });
  const threads = [
    create(EndUserThreadSchema, { id: 'Th7sK2pQ9mXr4StV8nBcW', agentId, kind: 'dm', lastActivityAt: timestampFromDate(new Date(Date.UTC(2026, 10, 5))) }),
    create(EndUserThreadSchema, { id: 'Th9wQ2mXr4StV8nBcW7sK', agentId: 'Ag9zZ9zZ9zZ9zZ9zZ9zZ9', kind: 'group' }),
  ];

  function clients(get: ReturnType<typeof vi.fn>) {
    setClient('$endUserClient', { getEndUser: get });
    setClient('$agentClient', { listAgents: vi.fn().mockResolvedValue({ agents: [{ id: agentId, projectId, slug: 'yasaku', name: 'Yasaku' }] }) });
  }

  it('shows the person\'s label, identity, first and last seen, and their threads by agent, read-only', async () => {
    const get = vi.fn().mockResolvedValue({ endUser: sari, threads });
    clients(get);
    seed(`/end-users/${sari.id}`, { endUser: sari.id });
    const w = await mountPage(EndUserPage);
    await flushPromises();
    expect(get).toHaveBeenCalledWith({ id: sari.id });
    expect(w.get('h1').text()).toBe('person.masked|{"kind":"person.kind_wa_phone","hint":"90"}');
    expect(w.get('dl').text()).toContain('person.kind_wa_phone …90');
    const items = w.findAll('li');
    expect(items[0]!.text()).toContain('threads.kind_whatsapp_dm');
    expect(items[0]!.text()).toContain('Yasaku');
    expect(w.findAllComponents(RouterLinkStub).map(l => l.props('to'))).toContain(`/o/acme/projects/rumah/agents/yasaku/threads/${threads[0]!.id}`);
    expect(items[1]!.text()).toContain('endusers.agent_gone');
    expect(items[1]!.text()).toContain('threads.kind_whatsapp_group');
    expect(w.findAll('button').filter(b => !b.element.closest('dialog'))).toHaveLength(0);
    expect(w.text()).not.toContain('preview');
  });

  it('says when they have no threads, and shows a missing person as not found', async () => {
    const get = vi.fn().mockResolvedValueOnce({ endUser: sari, threads: [] }).mockRejectedValue(connectError('EUS001'));
    clients(get);
    seed(`/end-users/${sari.id}`, { endUser: sari.id });
    const w = await mountPage(EndUserPage);
    await flushPromises();
    expect(w.text()).toContain('endusers.threads_empty');
    route.params = { ...route.params, endUser: 'Eu9zZ9zZ9zZ9zZ9zZ9zZ9' };
    await flushPromises();
    expect(w.get('h1').text()).toBe('error.end_user_not_found');
  });

  it('shows a failure with retry', async () => {
    setKnownKeys('error.load_end_users');
    const get = vi.fn().mockRejectedValueOnce(new Error('offline')).mockResolvedValue({ endUser: sari, threads: [] });
    clients(get);
    seed(`/end-users/${sari.id}`, { endUser: sari.id });
    const w = await mountPage(EndUserPage);
    await flushPromises();
    expect(w.get('[role="alert"]').text()).toContain('error.load_end_users');
    expect(w.get('h1').classes()).toContain('sr-only');
    await w.findAll('button').find(b => b.text() === 'common.retry')!.trigger('click');
    await flushPromises();
    expect(get).toHaveBeenCalledTimes(2);
    expect(w.find('[role="alert"]').exists()).toBe(false);
  });
});

describe('end user memories', () => {
  const support = 'Ag7pQ2wXk9MmT4vNc8RbZ';
  const billing = 'Ag3kL8pQ2wXr7MnV5bCzD';
  const sari = create(EndUserSchema, { id: 'Eu5sR2kP8mQx4StV7nBcW', projectId, displayName: 'Sari', identityKind: IdentityKind.WA_PHONE, identityHint: '90', threadCount: 1n });
  const day = (d: number) => timestampFromDate(new Date(Date.UTC(2026, 10, d)));
  const fact = (id: string, text: string, agentId = support, sourceRunId = 'Rn2wQ8mX4vZ9tB3nC7kPq', d = 10) => create(MemorySchema, { id, text, agentId, createdAt: day(d), sourceRunId });
  const admin = ['project:read', 'project:write', 'memory:read', 'memory:write'];
  const people = 'Ms4tR7wQ2xKp9VbNc3HzL';
  const archive = 'Ms8kW3nV6yHp7ZxR5cJtQ';
  const storesPB = [create(MemoryStoreSchema, { id: people, projectId, name: 'People', backend: MemoryBackend.NATIVE }), create(MemoryStoreSchema, { id: archive, projectId, name: 'Archive', backend: MemoryBackend.NATIVE })];
  const long = `Account number ends in 4421 and ${'the card is blue and kept in the drawer '.repeat(4)}`;
  const openDialog = () => [...document.querySelectorAll('dialog')].find(d => d.open)!;
  const confirmIn = (label: string) => ([...openDialog().querySelectorAll('button')].find(b => b.textContent?.trim() === label) as HTMLButtonElement).click();

  function clients(memory: Record<string, ReturnType<typeof vi.fn>> = {}) {
    setClient('$endUserClient', { getEndUser: vi.fn().mockResolvedValue({ endUser: sari, threads: [] }) });
    setClient('$agentClient', { listAgents: vi.fn().mockResolvedValue({ agents: [{ id: support, projectId, slug: 'support', name: 'Support' }, { id: billing, projectId, slug: 'billing', name: 'Billing' }] }) });
    const c = {
      listMemoryStores: vi.fn().mockResolvedValue({ stores: storesPB, enabledBackends: [MemoryBackend.NATIVE] }),
      listMemories: vi.fn().mockResolvedValue({ memories: [fact('f1', 'Prefers replies in Indonesian'), fact('f2', long, billing, '', 8)], nextPageToken: '', total: 2 }),
      deleteMemory: vi.fn().mockResolvedValue({}),
      clearMemories: vi.fn().mockResolvedValue({ deleted: 2 }),
      ...memory,
    };
    setClient('$memoryClient', c);
    return c;
  }

  async function open(permissions = admin) {
    seed(`/end-users/${sari.id}`, { endUser: sari.id }, permissions);
    const w = await mountPage(EndUserPage, { attachTo: document.body });
    await flushPromises();
    return w;
  }

  const rows = (w: Awaited<ReturnType<typeof open>>) => w.findAll('table tbody tr');

  it('lists what agents remember about the person, newest first, with agent, date and source run', async () => {
    const c = clients();
    const w = await open();
    expect(c.listMemories).toHaveBeenCalledWith({ storeId: people, endUserId: sari.id, pageSize: 50 });
    const headings = w.findAll('h2').map(h => h.text());
    expect(headings.indexOf('endusers.memories')).toBe(headings.indexOf('endusers.about') + 1);
    expect(w.text()).toContain('endusers.memories_desc|{"n":2}');
    expect(rows(w).map(r => r.findAll('td').slice(0, 2).map(td => td.text()))).toEqual([['Prefers replies in Indonesian', 'Support'], [long.trim(), 'Billing']]);
    const link = w.findAllComponents(RouterLinkStub).find(l => l.props('to') === '/o/acme/projects/rumah/agents/support/runs/Rn2wQ8mX4vZ9tB3nC7kPq');
    expect(link!.text()).toBe('endusers.view_run');
    expect(link!.classes()).toContain('min-h-6');
    const names = w.findAll('table tbody button').map(b => b.attributes('aria-label'));
    expect(names).toEqual(['endusers.memory_delete|{"text":"Prefers replies in Indonesian"}', expect.stringMatching(/^endusers\.memory_delete\|\{"text":"Account number ends in 4421.*…"\}$/)]);
    w.unmount();
  });

  it('opens the first memory store, and reloads from the first page on another store', async () => {
    const c = clients();
    const w = await open();
    const select = w.get('select[data-field="memory-store"]');
    expect(select.findAll('option').map(o => o.text())).toEqual(['People', 'Archive']);
    expect((select.element as HTMLSelectElement).value).toBe(people);
    await select.setValue(archive);
    await flushPromises();
    expect(c.listMemories).toHaveBeenLastCalledWith({ storeId: archive, endUserId: sari.id, pageSize: 50 });
    w.unmount();
  });

  it('says the project has no memory stores yet, links to them, and loads nothing', async () => {
    const c = clients({ listMemoryStores: vi.fn().mockResolvedValue({ stores: [], enabledBackends: [MemoryBackend.NATIVE] }) });
    const w = await open();
    expect(w.text()).toContain('endusers.memories_no_stores');
    expect(w.findAllComponents(RouterLinkStub).map(l => l.props('to'))).toContain('/o/acme/projects/rumah/memory-stores');
    expect(c.listMemories).not.toHaveBeenCalled();
    expect(w.find('select[data-field="memory-agent"]').exists()).toBe(false);
    w.unmount();
  });

  it('shows a store whose backend is off (MEM006) as the section error', async () => {
    setKnownKeys('apperror.MEM006');
    clients({ listMemories: vi.fn().mockRejectedValue(connectError('MEM006')) });
    const w = await open();
    expect(w.get('[role="alert"]').text()).toContain('apperror.MEM006');
    w.unmount();
  });

  it('shows a deleted run as text, not as a link', async () => {
    clients();
    const w = await open();
    expect(rows(w)[1]!.text()).toContain('endusers.run_gone');
    expect(w.findAllComponents(RouterLinkStub).filter(l => String(l.props('to')).includes('/agents/billing/runs/'))).toHaveLength(0);
    w.unmount();
  });

  it('filters by agent and reloads from the first page', async () => {
    const c = clients();
    const w = await open();
    await w.get('select[data-field="memory-agent"]').setValue(billing);
    await flushPromises();
    expect(c.listMemories).toHaveBeenLastCalledWith({ storeId: people, endUserId: sari.id, agentId: billing, pageSize: 50 });
    w.unmount();
  });

  it('loads 50 more with Load more, and hides it on the last page', async () => {
    const c = clients({ listMemories: vi.fn().mockResolvedValueOnce({ memories: [fact('f1', 'One')], nextPageToken: 'p2', total: 2 }).mockResolvedValue({ memories: [fact('f2', 'Two')], nextPageToken: '', total: 2 }) });
    const w = await open();
    await w.findAll('button').find(b => b.text() === 'approvals.load_more')!.trigger('click');
    await flushPromises();
    expect(c.listMemories).toHaveBeenLastCalledWith({ storeId: people, endUserId: sari.id, pageSize: 50, pageToken: 'p2' });
    expect(rows(w)).toHaveLength(2);
    expect(w.findAll('button').some(b => b.text() === 'approvals.load_more')).toBe(false);
    w.unmount();
  });

  it('deletes one memory after a confirm quoting it, in place, with a toast', async () => {
    const c = clients();
    const w = await open();
    const del = rows(w)[1]!.get('button[aria-label^="endusers.memory_delete|"]');
    expect(del.classes()).toContain('max-sm:size-11');
    await del.trigger('click');
    expect(openDialog().textContent).toContain('endusers.memory_delete_title');
    expect(openDialog().textContent).toContain('Account number ends in 4421');
    expect(openDialog().textContent).not.toContain(long.trim());
    expect(openDialog().textContent).toContain('…');
    confirmIn('common.delete');
    await flushPromises();
    expect(c.deleteMemory).toHaveBeenCalledWith({ storeId: people, endUserId: sari.id, id: 'f2' });
    expect(rows(w)).toHaveLength(1);
    expect(useToast().toasts.at(-1)).toMatchObject({ kind: 'success', text: 'endusers.memory_deleted' });
    w.unmount();
  });

  it('says a memory was already gone when the delete finds nothing, and refreshes the list', async () => {
    const c = clients({ deleteMemory: vi.fn().mockRejectedValue(connectError('MEM001')) });
    const w = await open();
    await rows(w)[0]!.get('button[aria-label^="endusers.memory_delete|"]').trigger('click');
    confirmIn('common.delete');
    await flushPromises();
    expect(c.listMemories).toHaveBeenCalledTimes(2);
    expect(useToast().toasts.at(-1)).toMatchObject({ kind: 'info', text: 'endusers.memory_gone' });
    w.unmount();
  });

  it('clears one agent\'s memories with a confirm naming the agent and the person, and all agents without a filter, then says how many were cleared', async () => {
    const c = clients();
    const w = await open();
    await w.get('select[data-field="memory-agent"]').setValue(support);
    await flushPromises();
    await w.findAll('button').find(b => b.text() === 'endusers.memories_clear')!.trigger('click');
    expect(openDialog().textContent).toContain('endusers.memories_clear_agent_title|{"n":2,"agent":"Support","name":"Sari","store":"People"}');
    expect(openDialog().textContent).toContain('endusers.memories_clear_agent_body|{"agent":"Support"}');
    confirmIn('endusers.memories_clear');
    await flushPromises();
    expect(c.clearMemories).toHaveBeenLastCalledWith({ storeId: people, endUserId: sari.id, agentId: support });
    expect(useToast().toasts.at(-1)).toMatchObject({ kind: 'success', text: 'endusers.memories_cleared|{"n":2}' });
    await w.get('select[data-field="memory-agent"]').setValue('');
    await flushPromises();
    await w.findAll('button').find(b => b.text() === 'endusers.memories_clear')!.trigger('click');
    expect(openDialog().textContent).toContain('endusers.memories_clear_title|{"n":2,"name":"Sari","store":"People"}');
    confirmIn('endusers.memories_clear');
    await flushPromises();
    expect(c.clearMemories).toHaveBeenLastCalledWith({ storeId: people, endUserId: sari.id });
    w.unmount();
  });

  it('hides delete and clear without memory:write, and the section without memory:read', async () => {
    const c = clients();
    const member = await open(['project:read', 'memory:read']);
    expect(rows(member)).toHaveLength(2);
    expect(member.find('button[aria-label^="endusers.memory_delete|"]').exists()).toBe(false);
    expect(member.findAll('button').some(b => b.text() === 'endusers.memories_clear' && !b.element.closest('dialog'))).toBe(false);
    member.unmount();
    setActivePinia(createPinia());
    c.listMemories.mockClear();
    const reader = await open(['project:read']);
    expect(reader.findAll('h2').map(h => h.text())).not.toContain('endusers.memories');
    expect(c.listMemories).not.toHaveBeenCalled();
    reader.unmount();
  });

  it('shows nothing remembered yet, and a load failure inside the section with retry, keeping the person\'s facts', async () => {
    setKnownKeys('error.load_memories');
    const c = clients({ listMemories: vi.fn().mockResolvedValueOnce({ memories: [], nextPageToken: '', total: 0 }).mockRejectedValueOnce(new Error('offline')).mockResolvedValue({ memories: [], nextPageToken: '', total: 0 }) });
    const w = await open();
    expect(w.text()).toContain('endusers.memories_empty');
    expect(w.findAll('button').some(b => b.text() === 'endusers.memories_clear' && !b.element.closest('dialog'))).toBe(false);
    await w.get('select[data-field="memory-agent"]').setValue(support);
    await flushPromises();
    expect(w.get('[role="alert"]').text()).toContain('error.load_memories');
    expect(w.text()).toContain('endusers.about');
    await w.findAll('button').find(b => b.text() === 'common.retry')!.trigger('click');
    await flushPromises();
    expect(c.listMemories).toHaveBeenCalledTimes(3);
    w.unmount();
  });
});
