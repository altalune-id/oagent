// @vitest-environment happy-dom
import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { flushPromises, RouterLinkStub } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { fakeRuntimeClients } from '~~/test/fixtures/runtime';
import { WhoamiResponseSchema } from '#gen/auth/v1/auth_pb';
import { EndUserSchema, EndUserThreadSchema, IdentityKind } from '#gen/enduser/v1/enduser_pb';
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
