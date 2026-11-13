// @vitest-environment happy-dom
import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { flushPromises, RouterLinkStub } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fakeRuntimeClients } from '~~/test/fixtures/runtime';
import { useToast } from '#base/components/ui/toast';
import { WhoamiResponseSchema } from '#gen/auth/v1/auth_pb';
import { MemoryBackend, MemoryStoreSchema } from '#gen/memory/v1/memory_pb';
import { mountPage } from '#test/mount-page';
import { connectError, setClient, setKnownKeys, setRoute } from '#test/nuxt-globals';
import MemoryStoresPage from '~/pages/o/[org]/projects/[project]/memory-stores/index.vue';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

const orgId = 'B6rTn2YkM8wQz3XpR5vGd';
const projectId = 'x6MH9WPWjktMwCBKHy4pF';
const support = 'Ag7pQ2wXk9MmT4vNc8RbZ';
const admin = ['project:read', 'agent:read', 'memory:read', 'memory:write'];
const member = ['project:read', 'agent:read', 'memory:read'];
const people = create(MemoryStoreSchema, { id: 'Ms4tR7wQ2xKp9VbNc3HzL', projectId, name: 'People', backend: MemoryBackend.NATIVE, usedBy: [{ agentId: support, agentName: 'Support' }], createdAt: timestampFromDate(new Date(Date.UTC(2026, 10, 10))) });
const spare = create(MemoryStoreSchema, { id: 'Ms8kW3nV6yHp7ZxR5cJtQ', projectId, name: 'Spare', backend: MemoryBackend.NATIVE, rememberApiUsers: true });

type Mounted = Awaited<ReturnType<typeof mountPage>>;
const openDialog = (w: Mounted) => w.findAll('dialog').find(d => (d.element as HTMLDialogElement).open)!;
const pageButtons = (w: Mounted) => w.findAll('button').filter(b => !b.element.closest('dialog'));
const dialogButton = (w: Mounted, text: string) => openDialog(w).findAll('button').find(b => b.text() === text)!;
const deleteButton = (w: Mounted, name: string) => w.findAll('button').find(b => b.attributes('aria-label') === `memorystores.delete_store|{"name":"${name}"}`);

function seed(permissions = admin, memory: Record<string, ReturnType<typeof vi.fn>> = {}) {
  useOrgsStore().items = [{ id: orgId, slug: 'acme', name: 'Acme', role: 'owner', permissions: [] }];
  useProjectsStore().items = [{ id: projectId, orgId, slug: 'rumah', name: 'Rumah Tangga' }];
  setRoute({ path: '/o/acme/projects/rumah/memory-stores', params: { org: 'acme', project: 'rumah' } });
  setClient('$authClient', { whoami: vi.fn().mockResolvedValue(create(WhoamiResponseSchema, { name: 'Budi', permissions })) });
  setClient('$agentClient', { listAgents: vi.fn().mockResolvedValue({ agents: [{ id: support, projectId, slug: 'support', name: 'Support' }] }) });
  const c = {
    listMemoryStores: vi.fn().mockResolvedValue({ stores: [people, spare], enabledBackends: [MemoryBackend.NATIVE] }),
    createMemoryStore: vi.fn().mockResolvedValue({ store: create(MemoryStoreSchema, { id: 'Ms2bVn7QxK4pRt9KwZc3H', projectId, name: 'Billing', backend: MemoryBackend.NATIVE }) }),
    deleteMemoryStore: vi.fn().mockResolvedValue({}),
    ...memory,
  };
  setClient('$memoryClient', c);
  return c;
}

beforeEach(() => {
  setActivePinia(createPinia());
  fakeRuntimeClients();
});

describe('memory stores page', () => {
  it('lists the stores with the agents using them and their date, with New and delete for admins', async () => {
    const c = seed();
    const w = await mountPage(MemoryStoresPage, { attachTo: document.body });
    await flushPromises();
    expect(c.listMemoryStores).toHaveBeenCalledWith({ projectId });
    const rows = w.findAll('table tbody tr');
    expect(rows.map(r => [r.get('td .font-medium').text(), r.findAll('td')[1]!.text()])).toEqual([['People', 'Support'], ['Spare', 'memorystores.unused']]);
    expect(w.findAllComponents(RouterLinkStub).map(l => l.props('to'))).toContain('/o/acme/projects/rumah/agents/support');
    expect(pageButtons(w).some(b => b.text().includes('memorystores.new'))).toBe(true);
    expect(deleteButton(w, 'People')).toBeDefined();
    expect(rows.map(r => r.find('td .badge').exists() ? r.get('td .badge').text() : '')).toEqual(['', 'memorystores.api_users']);
    expect(rows[0]!.findAll('td')[1]!.get('a').classes()).toContain('min-h-6');
    w.unmount();
  });

  it('is read-only for members: no New, no delete, and a note', async () => {
    seed(member);
    const w = await mountPage(MemoryStoresPage);
    await flushPromises();
    expect(w.text()).toContain('memorystores.read_only');
    expect(pageButtons(w).some(b => b.text().includes('memorystores.new'))).toBe(false);
    expect(w.find('table tbody button').exists()).toBe(false);
  });

  it('creates a store from the dialog, adds the row and says so', async () => {
    const c = seed();
    const w = await mountPage(MemoryStoresPage, { attachTo: document.body });
    await flushPromises();
    await pageButtons(w).find(b => b.text().includes('memorystores.new'))!.trigger('click');
    expect(openDialog(w).text()).toContain('memorystores.remember_api_users_hint');
    const box = openDialog(w).get('input[type="checkbox"]');
    expect((box.element as HTMLInputElement).checked).toBe(false);
    await openDialog(w).get('input[type="text"]').setValue('Billing');
    await box.setValue(true);
    await dialogButton(w, 'common.create').trigger('click');
    await flushPromises();
    expect(c.createMemoryStore).toHaveBeenCalledWith({ orgId, projectId, name: 'Billing', rememberApiUsers: true });
    expect(w.findAll('table tbody tr').map(r => r.get('td .font-medium').text())).toEqual(['People', 'Spare', 'Billing']);
    expect(useToast().toasts.at(-1)).toMatchObject({ kind: 'success', text: 'memorystores.created|{"name":"Billing"}' });
    w.unmount();
  });

  it('blocks deleting a store published agents use, and deletes an unused one after a confirm', async () => {
    setKnownKeys('apperror.MEM004');
    const c = seed();
    const w = await mountPage(MemoryStoresPage, { attachTo: document.body });
    await flushPromises();
    await deleteButton(w, 'People')!.trigger('click');
    expect(openDialog(w).text()).toContain('memorystores.in_use_title|{"name":"People"}');
    expect((dialogButton(w, 'common.delete').element as HTMLButtonElement).disabled).toBe(true);
    expect(openDialog(w).findAllComponents(RouterLinkStub).map(l => l.props('to'))).toEqual(['/o/acme/projects/rumah/agents/support']);
    await dialogButton(w, 'common.cancel').trigger('click');
    await deleteButton(w, 'Spare')!.trigger('click');
    expect(openDialog(w).text()).toContain('memorystores.delete_title|{"name":"Spare"}');
    expect(openDialog(w).text()).toContain('memorystores.delete_body');
    await dialogButton(w, 'common.delete').trigger('click');
    await flushPromises();
    expect(c.deleteMemoryStore).toHaveBeenCalledWith({ id: 'Ms8kW3nV6yHp7ZxR5cJtQ' });
    expect(w.findAll('table tbody tr')).toHaveLength(1);
    expect(useToast().toasts.at(-1)).toMatchObject({ kind: 'success', text: 'memorystores.deleted|{"name":"Spare"}' });
    w.unmount();
  });

  it('names the agents when the server refuses a delete as in use', async () => {
    const c = seed(admin, { listMemoryStores: vi.fn().mockResolvedValue({ stores: [create(MemoryStoreSchema, { ...spare, usedBy: [] })], enabledBackends: [MemoryBackend.NATIVE] }) });
    c.deleteMemoryStore.mockRejectedValue(connectError('MEM004'));
    const w = await mountPage(MemoryStoresPage, { attachTo: document.body });
    await flushPromises();
    await deleteButton(w, 'Spare')!.trigger('click');
    await dialogButton(w, 'common.delete').trigger('click');
    await flushPromises();
    expect(openDialog(w).text()).toContain('memorystores.in_use_now');
    w.unmount();
  });

  it('shows a store whose backend is off (MEM006) in the delete confirm', async () => {
    setKnownKeys('apperror.MEM006');
    const c = seed();
    c.deleteMemoryStore.mockRejectedValue(connectError('MEM006'));
    const w = await mountPage(MemoryStoresPage, { attachTo: document.body });
    await flushPromises();
    await deleteButton(w, 'Spare')!.trigger('click');
    await dialogButton(w, 'common.delete').trigger('click');
    await flushPromises();
    expect(openDialog(w).text()).toContain('apperror.MEM006');
    w.unmount();
  });

  it('starts empty with a New button, and says memory is off when no backend is on', async () => {
    seed(admin, { listMemoryStores: vi.fn().mockResolvedValueOnce({ stores: [], enabledBackends: [MemoryBackend.NATIVE] }).mockResolvedValue({ stores: [], enabledBackends: [] }) });
    const w = await mountPage(MemoryStoresPage);
    await flushPromises();
    expect(w.text()).toContain('memorystores.empty');
    expect(pageButtons(w).filter(b => b.text().includes('memorystores.new'))).toHaveLength(1);
    w.unmount();
    setActivePinia(createPinia());
    seed(admin, { listMemoryStores: vi.fn().mockResolvedValue({ stores: [], enabledBackends: [] }) });
    const off = await mountPage(MemoryStoresPage);
    await flushPromises();
    expect(off.text()).toContain('memorystores.off');
    expect(pageButtons(off).some(b => b.text().includes('memorystores.new'))).toBe(false);
  });

  it('shows a load failure with Retry', async () => {
    setKnownKeys('error.load_memory_stores');
    const c = seed(admin, { listMemoryStores: vi.fn().mockRejectedValueOnce(new Error('offline')).mockResolvedValue({ stores: [], enabledBackends: [MemoryBackend.NATIVE] }) });
    const w = await mountPage(MemoryStoresPage);
    await flushPromises();
    expect(w.get('[role="alert"]').text()).toContain('error.load_memory_stores');
    await pageButtons(w).find(b => b.text() === 'common.retry')!.trigger('click');
    await flushPromises();
    expect(c.listMemoryStores).toHaveBeenCalledTimes(2);
  });
});
