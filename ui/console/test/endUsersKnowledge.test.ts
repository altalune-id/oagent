// @vitest-environment happy-dom
import { create } from '@bufbuild/protobuf';
import { flushPromises, RouterLinkStub } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { fakeRuntimeClients } from '~~/test/fixtures/runtime';
import { useToast } from '#base/components/ui/toast';
import { WhoamiResponseSchema } from '#gen/auth/v1/auth_pb';
import { mountPage } from '#test/mount-page';
import { navigateTo, route, setClient, setRoute } from '#test/nuxt-globals';
import EndUserPage from '~/pages/o/[org]/projects/[project]/end-users/[endUser].vue';
import EndUsersPage from '~/pages/o/[org]/projects/[project]/end-users/index.vue';
import KnowledgePage from '~/pages/o/[org]/projects/[project]/knowledge.vue';
import { useEndUsersStore } from '~/stores/endUsers';
import { useKnowledgeStore } from '~/stores/knowledge';
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

const openDialog = (w: { findAll: (s: string) => { element: Element }[] }) => w.findAll('dialog').find(d => (d.element as HTMLDialogElement).open)!;

function page<T extends { findAll: (s: string) => { element: Element }[] }>(w: T) {
  return w.findAll('button').filter(b => !b.element.closest('dialog')) as ReturnType<T['findAll']>;
}

beforeEach(() => {
  setActivePinia(createPinia());
  fakeRuntimeClients();
});

describe('end users list', () => {
  it('lists people with masked phones and filters by name or identity', async () => {
    seed('/end-users');
    const w = await mountPage(EndUsersPage);
    const rows = w.findAll('tbody tr');
    expect(rows).toHaveLength(3);
    expect(rows[0]!.text()).toContain('endusers.identity_phone|{"value":"+62 800 •••• 0001"}');
    expect(rows[2]!.text()).toContain('endusers.identity_api|{"value":"cust_0001"}');
    expect(w.findAllComponents(RouterLinkStub)[0]!.props('to')).toBe('/o/acme/projects/rumah/end-users/eu_budi1');
    await w.get('input[type="search"]').setValue('sari');
    expect(w.findAll('tbody tr')).toHaveLength(1);
    await w.get('input[type="search"]').setValue('zzz');
    expect(w.text()).toContain('endusers.empty_search|{"query":"zzz"}');
  });

  it('says Off for memories when no agent uses memory, and shows an empty state', async () => {
    seed('/end-users');
    const store = useEndUsersStore();
    await store.load(projectId);
    store.memoryOn = false;
    const w = await mountPage(EndUsersPage);
    expect(w.get('tbody tr').text()).toContain('endusers.memories_off');
    store.items = [];
    await flushPromises();
    expect(w.text()).toContain('endusers.empty');
    expect(w.find('input[type="search"]').exists()).toBe(false);
  });
});

describe('end user detail', () => {
  it('shows identities, conversations and memories with links to the conversation', async () => {
    seed('/end-users/eu_sari2', { endUser: 'eu_sari2' });
    const w = await mountPage(EndUserPage);
    expect(w.get('h1').text()).toBe('Sari');
    expect(w.findAll('.badge').filter(b => b.text() === 'preview.badge')).toHaveLength(1);
    expect(w.get('dl').text()).toContain('+62 800 •••• 0002');
    expect(w.get('dl').text()).toContain('endusers.kind_lid');
    expect(w.get('dl').text()).toContain('•••• 9023');
    const links = w.findAllComponents(RouterLinkStub).map(l => l.props('to'));
    expect(links).toContain('/o/acme/projects/rumah/agents/yasaku-keuangan/threads/thr_grp3Kd9Q');
    expect(w.findAll('.list-row')).toHaveLength(2);
    expect(w.text()).toContain('endusers.delete_stays|{"servers":"yasaku, tagihan"}');
  });

  it('deletes one memory after a confirm', async () => {
    seed('/end-users/eu_sari2', { endUser: 'eu_sari2' });
    const w = await mountPage(EndUserPage);
    await w.get('button[aria-label^="memories.delete_label"]').trigger('click');
    const dialog = openDialog(w);
    expect(dialog.element.textContent).toContain('memories.delete_body|{"text":"Sari suka mencatat belanja pasar di dompet Cash"}');
    await w.findAll('dialog button').find(b => b.element.closest('dialog') === dialog.element && b.text() === 'common.delete')!.trigger('click');
    await flushPromises();
    expect(w.findAll('.list-row')).toHaveLength(1);
  });

  it('deletes the person only after the name is typed, then goes back to the list with a toast', async () => {
    seed('/end-users/eu_sari2', { endUser: 'eu_sari2' });
    const w = await mountPage(EndUserPage);
    await page(w).find(b => b.text() === 'endusers.delete_button|{"name":"Sari"}')!.trigger('click');
    const dialog = openDialog(w);
    const confirm = w.findAll('dialog button').find(b => b.element.closest('dialog') === dialog.element && b.text() === 'endusers.delete_button|{"name":"Sari"}')!;
    expect(confirm.attributes('disabled')).toBeDefined();
    expect(dialog.element.textContent).toContain('endusers.delete_confirm_body|{"servers":"yasaku, tagihan"}');
    const input = w.findAll('dialog input').find(i => i.element.closest('dialog') === dialog.element)!;
    await input.setValue('sari');
    expect(confirm.attributes('disabled')).toBeDefined();
    await input.setValue('Sari ');
    expect(confirm.attributes('disabled')).toBeUndefined();
    await confirm.trigger('click');
    await flushPromises();
    expect(navigateTo).toHaveBeenCalledWith('/o/acme/projects/rumah/end-users');
    expect(useEndUsersStore().byId('eu_sari2')).toBeUndefined();
    expect(useToast().toasts.at(-1)).toMatchObject({ kind: 'success', text: 'endusers.deleted|{"name":"Sari"}' });
  });

  it('says why memories are empty and hides delete actions from read-only members', async () => {
    seed('/end-users/eu_api3', { endUser: 'eu_api3' }, ['project:read']);
    const w = await mountPage(EndUserPage);
    expect(w.text()).toContain('memories.none');
    expect(w.text()).toContain('endusers.read_only');
    expect(page(w).some(b => b.text().startsWith('endusers.delete_button'))).toBe(false);
    useEndUsersStore().memoryOn = false;
    useEndUsersStore().byId('eu_api3')!.threads = [];
    await flushPromises();
    expect(w.text()).toContain('memories.off');
    expect(w.text()).toContain('endusers.threads_empty');
    route.params = { ...route.params, endUser: 'gone' };
    await flushPromises();
    expect(w.text()).toContain('error.end_user_not_found');
  });

  it('is a 404 for an unknown person', async () => {
    seed('/end-users/nope', { endUser: 'nope' });
    const errorHandler = vi.fn();
    await mountPage(EndUserPage, { global: { config: { errorHandler } } });
    expect(errorHandler.mock.calls[0]![0]).toMatchObject({ statusCode: 404 });
  });
});

describe('knowledge sources', () => {
  it('lists sources with status, crawl progress and failure reason', async () => {
    seed('/knowledge');
    const w = await mountPage(KnowledgePage);
    const rows = w.findAll('tbody tr');
    expect(rows).toHaveLength(3);
    expect(rows[0]!.text()).toContain('knowledge.chunks|{"n":38}');
    expect(rows[1]!.text()).toContain('knowledge.pages_progress|{"done":12,"total":40}');
    expect(rows[2]!.text()).toContain('knowledge.failure_robots|{"path":"/kpr"}');
    expect(rows[2]!.text()).toContain('knowledge.unused');
  });

  it('retries a failed source and deletes one after a confirm', async () => {
    seed('/knowledge');
    const w = await mountPage(KnowledgePage);
    const menus = w.findAll('tbody tr').map(r => r.findAll('li button').map(b => b.text()));
    expect(menus[0]).toEqual(['common.delete']);
    expect(menus[2]).toEqual(['knowledge.retry', 'common.delete']);
    await w.findAll('tbody tr')[2]!.findAll('li button')[0]!.trigger('click');
    expect(useKnowledgeStore().items[2]).toMatchObject({ status: 'queued', failure: undefined, pagesTotal: 50 });
    expect(useToast().toasts.at(-1)).toMatchObject({ text: 'knowledge.retried|{"name":"Syarat KPR"}' });
    await w.findAll('tbody tr')[0]!.findAll('li button')[0]!.trigger('click');
    const dialog = openDialog(w);
    expect(dialog.element.textContent).toContain('knowledge.delete_body|{"name":"Panduan kategori yasaku"}');
    await w.findAll('dialog button').find(b => b.element.closest('dialog') === dialog.element && b.text() === 'common.delete')!.trigger('click');
    await flushPromises();
    expect(useKnowledgeStore().items.map(s => s.name)).toEqual(['Promo bank', 'Syarat KPR']);
  });

  it('adds a website after checking the address and page limit', async () => {
    seed('/knowledge');
    const w = await mountPage(KnowledgePage);
    await page(w).find(b => b.text() === 'knowledge.add')!.trigger('click');
    const dialog = openDialog(w);
    const inDialog = (s: string) => w.findAll(s).filter(x => x.element.closest('dialog') === dialog.element);
    const add = inDialog('button').find(b => b.text() === 'knowledge.add')!;
    const [url, pages] = inDialog('form input');
    await pages!.setValue('0');
    await add.trigger('click');
    expect(dialog.element.textContent).toContain('knowledge.url_invalid');
    expect(dialog.element.textContent).toContain('knowledge.max_pages_invalid|{"max":500}');
    await url!.setValue('https://bank.example.id/promo/');
    await pages!.setValue('40');
    await inDialog('form')[0]!.trigger('submit');
    await flushPromises();
    expect(useKnowledgeStore().items[0]).toMatchObject({ name: 'bank.example.id/promo', status: 'queued', pagesTotal: 40 });
    expect(useToast().toasts.at(-1)).toMatchObject({ kind: 'success', text: 'knowledge.added|{"name":"bank.example.id/promo"}' });
    expect(w.findAll('tbody tr')).toHaveLength(4);
  });

  it('keeps document uploads off and closes on cancel', async () => {
    seed('/knowledge');
    const w = await mountPage(KnowledgePage);
    await page(w).find(b => b.text() === 'knowledge.add')!.trigger('click');
    const dialog = openDialog(w);
    const inDialog = (s: string) => w.findAll(s).filter(x => x.element.closest('dialog') === dialog.element);
    await inDialog('input[value="document"]')[0]!.setValue(true);
    expect(dialog.element.textContent).toContain('knowledge.storage_off');
    expect(inDialog('input[type="file"]')[0]!.attributes('disabled')).toBeDefined();
    expect(inDialog('button').find(b => b.text() === 'knowledge.add')!.attributes('disabled')).toBeDefined();
    await inDialog('button').find(b => b.text() === 'common.cancel')!.trigger('click');
    expect(openDialog(w)).toBeUndefined();
  });

  it('needs an embedding model before adding, and shows an empty state', async () => {
    seed('/knowledge');
    const w = await mountPage(KnowledgePage);
    useKnowledgeStore().embeddingReady = false;
    useKnowledgeStore().items = [];
    await flushPromises();
    expect(w.text()).toContain('knowledge.needs_embedding');
    expect(w.text()).toContain('knowledge.empty');
    expect(page(w).find(b => b.text() === 'knowledge.add')!.attributes('disabled')).toBeDefined();
  });

  it('shows no add button to read-only members', async () => {
    seed('/knowledge', {}, ['project:read']);
    const w = await mountPage(KnowledgePage);
    expect(page(w).some(b => b.text() === 'knowledge.add')).toBe(false);
    expect(w.find('tbody li button').exists()).toBe(false);
    expect(w.text()).not.toContain('knowledge.needs_embedding');
  });
});
