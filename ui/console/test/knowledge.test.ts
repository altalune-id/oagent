// @vitest-environment happy-dom
import { create } from '@bufbuild/protobuf';
import { flushPromises, RouterLinkStub } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { docPB, documentPBs, fakeKnowledgeClient, orgId, projectId } from '~~/test/fixtures/knowledge';
import { fakeRuntimeClients } from '~~/test/fixtures/runtime';
import { useToast } from '#base/components/ui/toast';
import { WhoamiResponseSchema } from '#gen/auth/v1/auth_pb';
import { KnowledgeDocumentStatus } from '#gen/knowledge/v1/knowledge_pb';
import { mountPage } from '#test/mount-page';
import { connectError, navigateTo, setClient, setKnownKeys, setRoute } from '#test/nuxt-globals';
import SourcePage from '~/pages/o/[org]/projects/[project]/knowledge/[source].vue';
import KnowledgePage from '~/pages/o/[org]/projects/[project]/knowledge/index.vue';
import { pollIntervalMs, useKnowledgeStore } from '~/stores/knowledge';
import { useModelProvidersStore } from '~/stores/modelProviders';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

const writer = ['project:read', 'knowledge:read', 'knowledge:write', 'provider:read'];
const reader = ['project:read', 'knowledge:read'];
const uploadSource = 'Ks1Upl0adQ8sWx3YcLm7D';
const urlSource = 'Ks2Ur1CrawlTq9Lm4Xw8P';
const failedSource = 'Ks3Fa1ledRp5Nw2Qk7Vb4';

type Mounted = Awaited<ReturnType<typeof mountPage>>;

let client: ReturnType<typeof fakeKnowledgeClient>;

function seed(path = '/knowledge', params: Record<string, string> = {}, permissions = writer) {
  useOrgsStore().items = [{ id: orgId, slug: 'acme', name: 'Acme', role: 'owner', permissions: [] }];
  useProjectsStore().items = [{ id: projectId, orgId, slug: 'rumah', name: 'Rumah Tangga' }];
  setRoute({ path: `/o/acme/projects/rumah${path}`, params: { org: 'acme', project: 'rumah', ...params } });
  setClient('$authClient', { whoami: vi.fn().mockResolvedValue(create(WhoamiResponseSchema, { name: 'Budi', permissions })) });
  setClient('$agentClient', { listAgents: vi.fn().mockResolvedValue({ agents: [{ id: 'Ag7Rf2Tq9Lm4Xw8Pz3Kc1', projectId, slug: 'refund-triage', name: 'Refund Triage', description: '' }] }) });
}

const openDialog = (w: Mounted) => w.findAll('dialog').find(d => (d.element as HTMLDialogElement).open);
const inDialog = (w: Mounted, s: string) => w.findAll(s).filter(x => x.element.closest('dialog') === openDialog(w)!.element);
const pageButtons = (w: Mounted) => w.findAll('button').filter(b => !b.element.closest('dialog'));
const button = (w: Mounted, text: string) => pageButtons(w).find(b => b.text() === text);
const submitSearch = (w: Mounted) => w.get('input[type="search"]').element.closest('form')!.dispatchEvent(new Event('submit'));
const dialogButton = (w: Mounted, text: string) => inDialog(w, 'button').find(b => b.text() === text)!;

function embeddingModels() {
  const providers = useModelProvidersStore();
  providers.items[0]!.models.push({ id: 'openai/text-embedding-3-small', vision: false, embedding: true, prices: { inputMicros: 20_000n, outputMicros: 0n, cacheReadMicros: 0n, cacheWriteMicros: 0n } });
  providers.items[0]!.models.push({ id: 'openai/text-embedding-3-large', vision: false, embedding: true, prices: { inputMicros: 130_000n, outputMicros: 0n, cacheReadMicros: 0n, cacheWriteMicros: 0n } });
  providers.items[1]!.models.push({ id: 'nomic-embed', vision: false, embedding: true });
}

beforeEach(() => {
  setActivePinia(createPinia());
  fakeRuntimeClients();
  client = fakeKnowledgeClient();
  setKnownKeys();
});

afterEach(() => {
  vi.useRealTimers();
});

describe('knowledge page', () => {
  it('lists sources with type, status, failure text, ready counts and the agents that use them, without a Preview badge', async () => {
    setKnownKeys('apperror.KNW014_robots_disallowed');
    seed();
    const w = await mountPage(KnowledgePage);
    expect(client.listKnowledgeSources).toHaveBeenCalledWith({ projectId });
    expect(w.text()).not.toContain('preview.badge');
    const rows = w.findAll('tbody tr');
    expect(rows).toHaveLength(3);
    expect(rows[0]!.text()).toContain('knowledge.kind_upload');
    expect(rows[0]!.text()).toContain('knowledge.source_ready');
    expect(rows[0]!.text()).toContain('knowledge.documents_ready|{"ready":2,"n":3}');
    expect(rows[0]!.text()).toContain('Refund Triage');
    expect(rows[0]!.getComponent(RouterLinkStub).props('to')).toBe(`/o/acme/projects/rumah/knowledge/${uploadSource}`);
    expect(rows[1]!.text()).toContain('knowledge.source_crawling');
    expect(rows[1]!.text()).toContain('https://help.example.com/docs');
    expect(rows[2]!.text()).toContain('apperror.KNW014_robots_disallowed');
    expect(rows[2]!.findAll('td')[3]!.text()).toBe('—');
    expect(rows[2]!.text()).toContain('knowledge.unused');
    expect(w.get('[data-embedding]').text()).toContain('openai/text-embedding-3-small');
    expect(w.get('[data-embedding]').text()).toContain('knowledge.embedding_line|{"provider":"OpenRouter","dims":1536}');
  });

  it('crawls a website again from its menu, with no confirm', async () => {
    seed();
    const w = await mountPage(KnowledgePage);
    const menus = w.findAll('tbody tr').map(r => r.findAll('li button').map(b => b.text()));
    expect(menus).toEqual([['common.delete'], ['knowledge.crawl_again', 'common.delete'], ['knowledge.crawl_again', 'common.delete']]);
    await w.findAll('tbody tr')[2]!.findAll('li button')[0]!.trigger('click');
    await flushPromises();
    expect(client.refreshKnowledgeSource).toHaveBeenCalledWith({ id: failedSource });
    expect(useKnowledgeStore().byId(failedSource)!.status).toBe('crawling');
    expect(useToast().toasts.at(-1)).toMatchObject({ kind: 'success', text: 'knowledge.crawling_again|{"name":"shop.example.com"}' });
    setKnownKeys('apperror.KNW001');
    client.refreshKnowledgeSource.mockRejectedValueOnce(connectError('KNW001'));
    useKnowledgeStore().sources = useKnowledgeStore().sources.map(s => ({ ...s, status: 'idle' as const }));
    await flushPromises();
    await w.findAll('tbody tr')[2]!.findAll('li button')[0]!.trigger('click');
    await flushPromises();
    expect(useToast().toasts.at(-1)).toMatchObject({ kind: 'error', text: 'apperror.KNW001' });
  });

  it('blocks deleting a source that published agents use, and deletes an unused one after a confirm', async () => {
    seed();
    const w = await mountPage(KnowledgePage, { attachTo: document.body });
    await w.findAll('tbody tr')[0]!.findAll('li button').at(-1)!.trigger('click');
    expect(openDialog(w)!.get('h2').text()).toBe('knowledge.in_use_title|{"name":"Refund policy"}');
    expect(openDialog(w)!.findAllComponents(RouterLinkStub).map(l => [l.text(), l.props('to')])).toEqual([['Refund Triage', '/o/acme/projects/rumah/agents/refund-triage']]);
    expect(openDialog(w)!.element.textContent).toContain('knowledge.in_use_body');
    expect(dialogButton(w, 'common.delete').attributes('disabled')).toBeDefined();
    await dialogButton(w, 'common.cancel').trigger('click');
    await w.findAll('tbody tr')[2]!.findAll('li button').at(-1)!.trigger('click');
    expect(openDialog(w)!.element.textContent).toContain('knowledge.delete_body|{"name":"shop.example.com"}');
    client.deleteKnowledgeSource.mockRejectedValueOnce(connectError('KNW008'));
    await dialogButton(w, 'common.delete').trigger('click');
    await flushPromises();
    expect(openDialog(w)!.element.textContent).toContain('knowledge.in_use');
    await dialogButton(w, 'common.delete').trigger('click');
    await flushPromises();
    expect(client.deleteKnowledgeSource).toHaveBeenLastCalledWith({ id: failedSource });
    expect(useKnowledgeStore().sources.map(s => s.name)).toEqual(['Refund policy', 'help.example.com/docs']);
    w.unmount();
  });

  it('adds a website: https only, the name from the host and path, the hint and limits, then opens the source', async () => {
    seed();
    const w = await mountPage(KnowledgePage, { attachTo: document.body });
    await button(w, 'knowledge.add')!.trigger('click');
    const dialog = openDialog(w)!;
    expect(dialog.element.textContent).toContain('knowledge.url_hint');
    expect(dialog.element.textContent).toContain('knowledge.crawl_limits');
    const url = inDialog(w, 'input[type="url"]')[0]!;
    expect(url.attributes('aria-describedby')).toBeTruthy();
    await url.setValue('http://help.example.com');
    await inDialog(w, 'form')[0]!.trigger('submit');
    await flushPromises();
    expect(dialog.element.textContent).toContain('knowledge.url_invalid');
    expect(client.createURLSource).not.toHaveBeenCalled();
    await url.setValue('https://help.example.com/guides/');
    await inDialog(w, 'form')[0]!.trigger('submit');
    await flushPromises();
    expect(client.createURLSource).toHaveBeenCalledWith({ orgId, projectId, name: 'help.example.com/guides', seedUrl: 'https://help.example.com/guides/' });
    expect(navigateTo).toHaveBeenCalledWith('/o/acme/projects/rumah/knowledge/Ks9NewUrlQ8sWx3YcLm7D');
    w.unmount();
  });

  it('shows the per-project source limit (KNW010) in the Add dialog with its number', async () => {
    setKnownKeys('apperror.KNW010_sources_per_project');
    client.createURLSource.mockRejectedValueOnce(connectError('KNW010', { reason: 'sources_per_project', max: '50' }));
    seed();
    const w = await mountPage(KnowledgePage, { attachTo: document.body });
    await button(w, 'knowledge.add')!.trigger('click');
    await inDialog(w, 'input[type="url"]')[0]!.setValue('https://help.example.com/');
    await inDialog(w, 'form')[0]!.trigger('submit');
    await flushPromises();
    expect(openDialog(w)!.get('[role="alert"]').text()).toContain('apperror.KNW010_sources_per_project|{"max":"50"}');
    w.unmount();
  });

  it('says Crawl again came too soon (KNW010) in a toast with its number', async () => {
    setKnownKeys('apperror.KNW010_refresh_too_soon');
    client.refreshKnowledgeSource.mockRejectedValueOnce(connectError('KNW010', { reason: 'refresh_too_soon', max: '10' }));
    seed();
    const w = await mountPage(KnowledgePage);
    await w.findAll('tbody tr')[2]!.findAll('li button')[0]!.trigger('click');
    await flushPromises();
    expect(useToast().toasts.at(-1)).toMatchObject({ kind: 'error', text: 'apperror.KNW010_refresh_too_soon|{"max":"10"}' });
  });

  it('adds an upload source, and keeps Upload off with a note when storage is off', async () => {
    client.listKnowledgeSources.mockResolvedValueOnce({ ...(await client.listKnowledgeSources()), storageConfigured: false });
    seed();
    const w = await mountPage(KnowledgePage, { attachTo: document.body });
    await button(w, 'knowledge.add')!.trigger('click');
    await inDialog(w, 'input[value="upload"]')[0]!.setValue(true);
    expect(openDialog(w)!.element.textContent).toContain('knowledge.storage_off');
    expect(dialogButton(w, 'knowledge.add').attributes('disabled')).toBeDefined();
    useKnowledgeStore().storageConfigured = true;
    await flushPromises();
    expect(dialogButton(w, 'knowledge.add').attributes('disabled')).toBeUndefined();
    await inDialog(w, 'input[type="text"]')[0]!.setValue('Manuals');
    await inDialog(w, 'form')[0]!.trigger('submit');
    await flushPromises();
    expect(client.createUploadSource).toHaveBeenCalledWith({ orgId, projectId, name: 'Manuals' });
    expect(navigateTo).toHaveBeenCalledWith('/o/acme/projects/rumah/knowledge/Ks8NewUplQ8sWx3YcLm7D');
    w.unmount();
  });

  it('picks the first embedding model with no confirm, and shows KNW007 at the field', async () => {
    client.listKnowledgeSources.mockResolvedValueOnce({ ...(await client.listKnowledgeSources()), embeddingModel: undefined });
    seed();
    const w = await mountPage(KnowledgePage, { attachTo: document.body });
    expect(w.text()).toContain('knowledge.embedding_none');
    expect(w.text()).toContain('knowledge.embedding_no_models');
    embeddingModels();
    await flushPromises();
    const select = w.get('section select');
    const options = select.findAll('option').slice(1);
    expect(options.map(o => [o.text(), o.attributes('disabled') !== undefined])).toEqual([
      ['knowledge.embedding_option|{"model":"openai/text-embedding-3-small","provider":"OpenRouter"}', false],
      ['knowledge.embedding_option|{"model":"openai/text-embedding-3-large","provider":"OpenRouter"}', false],
      ['knowledge.embedding_option_unpriced|{"model":"nomic-embed","provider":"Groq"}', true],
    ]);
    setKnownKeys('apperror.KNW007_size');
    client.setEmbeddingModel.mockRejectedValueOnce(connectError('KNW007', { reason: 'size', dims: '384' }));
    await select.setValue(options[0]!.attributes('value'));
    await w.get('section form').trigger('submit');
    await flushPromises();
    expect(w.get('section').text()).toContain('apperror.KNW007_size|{"dims":"384"}');
    expect(openDialog(w)).toBeUndefined();
    await w.get('section form').trigger('submit');
    await flushPromises();
    expect(client.setEmbeddingModel).toHaveBeenLastCalledWith({ projectId, providerId: useModelProvidersStore().items[0]!.id, model: 'openai/text-embedding-3-small' });
    expect(w.get('[data-embedding]').text()).toContain('openai/text-embedding-3-small');
    w.unmount();
  });

  it('warns that changing the model re-embeds everything and stops search, changing nothing until confirmed', async () => {
    seed();
    const w = await mountPage(KnowledgePage, { attachTo: document.body });
    embeddingModels();
    await button(w, 'knowledge.embedding_change')!.trigger('click');
    const select = w.get('section select');
    await select.setValue(select.findAll('option')[2]!.attributes('value'));
    await w.get('section form').trigger('submit');
    await flushPromises();
    expect(openDialog(w)!.element.textContent).toContain('knowledge.reembed_body|{"model":"openai/text-embedding-3-large"}');
    expect(client.setEmbeddingModel).not.toHaveBeenCalled();
    await dialogButton(w, 'common.cancel').trigger('click');
    expect(useKnowledgeStore().embedding!.model).toBe('openai/text-embedding-3-small');
    await w.get('section form').trigger('submit');
    await flushPromises();
    await dialogButton(w, 'knowledge.reembed_confirm').trigger('click');
    await flushPromises();
    expect(client.setEmbeddingModel).toHaveBeenCalledWith({ projectId, providerId: useModelProvidersStore().items[0]!.id, model: 'openai/text-embedding-3-large' });
    await flushPromises();
    expect(document.activeElement).toBe(button(w, 'knowledge.embedding_change')!.element);
    w.unmount();
  });

  it('moves focus to the model list on Change and back to Change on Cancel', async () => {
    seed();
    const w = await mountPage(KnowledgePage, { attachTo: document.body });
    embeddingModels();
    await button(w, 'knowledge.embedding_change')!.trigger('click');
    await flushPromises();
    expect(document.activeElement).toBe(w.get('section select').element);
    expect(w.findAll('section button').find(b => b.text() === 'knowledge.embedding_save')!.attributes('disabled'), 'the current model needs no save').toBeDefined();
    useModelProvidersStore().items[1]!.models.push({ id: 'nomic-embed-2', vision: false, embedding: true });
    await flushPromises();
    const unpriced = w.get('section select').findAll('option').find(o => o.text().includes('nomic-embed-2'))!;
    (w.get('section select').element as HTMLSelectElement).value = unpriced.attributes('value')!;
    await w.get('section select').trigger('change');
    expect(w.findAll('section button').find(b => b.text() === 'knowledge.embedding_save')!.attributes('disabled'), 'an unpriced model can\'t be saved').toBeDefined();
    await button(w, 'common.cancel')!.trigger('click');
    await flushPromises();
    expect(document.activeElement).toBe(button(w, 'knowledge.embedding_change')!.element);
    w.unmount();
  });

  it('says the embedding model\'s provider was deleted, and offers the picker', async () => {
    client.listKnowledgeSources.mockResolvedValueOnce({ ...(await client.listKnowledgeSources()), embeddingModel: { providerId: '', model: 'openai/text-embedding-3-small', dims: 1536, providerMissing: true } });
    seed();
    const w = await mountPage(KnowledgePage);
    embeddingModels();
    await flushPromises();
    expect(w.get('section [role="alert"]').text()).toBe('knowledge.embedding_provider_missing');
    expect(w.find('section select').exists()).toBe(true);
  });

  it('says knowledge is off when no backend is enabled', async () => {
    client.listKnowledgeSources.mockResolvedValueOnce({ sources: [], storageConfigured: false, enabledBackends: [] });
    seed();
    const w = await mountPage(KnowledgePage);
    expect(w.text()).toContain('knowledge.off');
    expect(w.find('table').exists()).toBe(false);
    expect(button(w, 'knowledge.add')).toBeUndefined();
  });

  it('shows the empty state with Add source for writers', async () => {
    client.listKnowledgeSources.mockResolvedValueOnce({ ...(await client.listKnowledgeSources()), sources: [] });
    seed();
    const w = await mountPage(KnowledgePage);
    expect(w.text()).toContain('knowledge.empty_body');
    expect(pageButtons(w).filter(b => b.text() === 'knowledge.add')).toHaveLength(1);
  });

  it('shows a load error with Retry', async () => {
    setKnownKeys('apperror.GEN900');
    client.listKnowledgeSources.mockRejectedValueOnce(connectError('GEN900'));
    seed();
    const w = await mountPage(KnowledgePage);
    expect(w.text()).toContain('apperror.GEN900');
    await button(w, 'common.retry')!.trigger('click');
    await flushPromises();
    expect(w.findAll('tbody tr')).toHaveLength(3);
  });

  it('gives read-only members the lists without add, menus or the model form', async () => {
    seed('/knowledge', {}, reader);
    const w = await mountPage(KnowledgePage);
    expect(w.text()).toContain('knowledge.read_only');
    expect(button(w, 'knowledge.add')).toBeUndefined();
    expect(button(w, 'knowledge.embedding_change')).toBeUndefined();
    expect(w.find('tbody li button').exists()).toBe(false);
  });
});

describe('knowledge polling across pages', () => {
  it('keeps the source page polling when the list page unmounts after the source page set up', async () => {
    vi.useFakeTimers();
    seed();
    const list = await mountPage(KnowledgePage);
    seed(`/knowledge/${uploadSource}`, { source: uploadSource });
    const source = await mountPage(SourcePage);
    list.unmount();
    client.listKnowledgeDocuments.mockClear();
    await vi.advanceTimersByTimeAsync(pollIntervalMs);
    expect(client.listKnowledgeDocuments).toHaveBeenCalledWith({ sourceId: uploadSource });
    source.unmount();
  });

  it('starts polling again once the first embedding model is set', async () => {
    client.listKnowledgeSources.mockResolvedValueOnce({ ...(await client.listKnowledgeSources()), embeddingModel: undefined });
    seed();
    const w = await mountPage(KnowledgePage, { attachTo: document.body });
    embeddingModels();
    await flushPromises();
    const start = vi.spyOn(useKnowledgeStore(), 'startPolling');
    const select = w.get('section select');
    await select.setValue(select.findAll('option')[1]!.attributes('value'));
    await w.get('section form').trigger('submit');
    await flushPromises();
    expect(start).toHaveBeenCalledWith(projectId, '');
    w.unmount();
  });
});

describe('knowledge source page', () => {
  it('lists an upload source\'s documents with status and reasons, and a waiting one links to the model', async () => {
    setKnownKeys('apperror.KNW005', 'apperror.KNW011');
    seed(`/knowledge/${uploadSource}`, { source: uploadSource });
    const w = await mountPage(SourcePage);
    expect(client.listKnowledgeDocuments).toHaveBeenCalledWith({ sourceId: uploadSource });
    expect(w.get('h1').text()).toBe('Refund policy');
    const rows = w.findAll('tbody tr');
    expect(rows.map(r => r.findAll('td')[1]!.text())).toEqual([
      'knowledge.doc_ready',
      'knowledge.doc_waiting_modelapperror.KNW011 knowledge.choose_model',
      'knowledge.doc_failedapperror.KNW005',
    ]);
    expect(rows[1]!.getComponent(RouterLinkStub).props('to')).toBe('/o/acme/projects/rumah/knowledge');
    expect(w.findAll('thead th').map(h => h.text())).not.toContain('knowledge.col_size');
    useKnowledgeStore().documents = { [uploadSource]: [{ ...useKnowledgeStore().documents[uploadSource]![0]!, bytes: 2048, passages: 7 }] };
    await flushPromises();
    expect(w.findAll('thead th').map(h => h.text())).toEqual(expect.arrayContaining(['knowledge.col_size', 'knowledge.col_passages']));
    expect(w.get('tbody tr').text()).toContain('2 KiB');
    expect(w.text()).toContain('knowledge.keep_open');
  });

  it('shows a waiting document as waiting for a model when the project has none, linking to it', async () => {
    client.listKnowledgeSources.mockResolvedValueOnce({ ...(await client.listKnowledgeSources()), embeddingModel: undefined });
    client.listKnowledgeDocuments.mockResolvedValueOnce({ documents: [documentPBs()[1]!].map(d => Object.assign(d, { failure: undefined })) });
    seed(`/knowledge/${uploadSource}`, { source: uploadSource });
    const w = await mountPage(SourcePage);
    const status = w.get('tbody tr').findAll('td')[1]!;
    expect(status.text()).toContain('knowledge.doc_waiting_model');
    expect(status.getComponent(RouterLinkStub).props('to')).toBe('/o/acme/projects/rumah/knowledge');
  });

  it('reloads the source after an upload, so its header and counts are current', async () => {
    seed(`/knowledge/${uploadSource}`, { source: uploadSource });
    const w = await mountPage(SourcePage);
    vi.spyOn(useKnowledgeStore(), 'upload').mockResolvedValue();
    client.listKnowledgeSources.mockClear();
    const input = w.get<HTMLInputElement>('input[type="file"]');
    Object.defineProperty(input.element, 'files', { value: [new File(['x'], 'a.md')], configurable: true });
    await input.trigger('change');
    await flushPromises();
    expect(client.listKnowledgeSources).toHaveBeenCalledWith({ projectId });
  });

  it('uploads chosen files through the store and shows per-file progress', async () => {
    setKnownKeys('apperror.KNW005');
    seed(`/knowledge/${uploadSource}`, { source: uploadSource });
    const w = await mountPage(SourcePage);
    const store = useKnowledgeStore();
    const upload = vi.spyOn(store, 'upload').mockResolvedValue();
    const input = w.get<HTMLInputElement>('input[type="file"]');
    expect(input.attributes('accept')).toBe('.pdf,.docx,.md,.markdown,.txt');
    const file = new File(['%PDF'], 'policy.pdf', { type: 'application/pdf' });
    Object.defineProperty(input.element, 'files', { value: [file, new File(['x'], 'photo.png')], configurable: true });
    await input.trigger('change');
    await flushPromises();
    expect(upload).toHaveBeenCalledWith(uploadSource, [file]);
    expect(w.text()).toContain('photo.png');
    expect(w.text()).toContain('knowledge.reject_type');
    store.uploads = { [uploadSource]: [{ key: 'u8', name: 'scan.pdf', bytes: 9, state: 'done', progress: 1, documentId: 'Kd3Fail0Lm4Xw8Pz3Kc1q' }] };
    await flushPromises();
    expect(w.get('ul.list li').text()).toContain('knowledge.upload_failed');
    expect(w.get('ul.list li').text()).toContain('apperror.KNW005');
    store.uploads = { [uploadSource]: [{ key: 'u9', name: 'manual.pdf', bytes: 2048, state: 'uploading', progress: 0.42 }] };
    await flushPromises();
    expect(w.get('progress').attributes('value')).toBe('42');
    expect(w.text()).toContain('2 KiB');
  });

  it('deletes a document after a confirm', async () => {
    seed(`/knowledge/${uploadSource}`, { source: uploadSource });
    const w = await mountPage(SourcePage, { attachTo: document.body });
    await w.findAll('tbody tr')[2]!.get('button').trigger('click');
    expect(openDialog(w)!.element.textContent).toContain('knowledge.delete_document_body|{"name":"scan.pdf"}');
    await dialogButton(w, 'common.delete').trigger('click');
    await flushPromises();
    expect(client.deleteKnowledgeDocument).toHaveBeenCalledWith({ id: 'Kd3Fail0Lm4Xw8Pz3Kc1q' });
    expect(w.findAll('tbody tr')).toHaveLength(2);
    w.unmount();
  });

  it('shows the storage-off note in place of the upload control, and nothing to change for readers', async () => {
    client.listKnowledgeSources.mockResolvedValueOnce({ ...(await client.listKnowledgeSources()), storageConfigured: false });
    seed(`/knowledge/${uploadSource}`, { source: uploadSource });
    const off = await mountPage(SourcePage);
    expect(off.text()).toContain('knowledge.storage_off');
    expect(off.find('input[type="file"]').exists()).toBe(false);
    setActivePinia(createPinia());
    seed(`/knowledge/${uploadSource}`, { source: uploadSource }, reader);
    const read = await mountPage(SourcePage);
    expect(read.find('input[type="file"]').exists()).toBe(false);
    expect(read.find('tbody button').exists()).toBe(false);
    expect(read.find('input[type="search"]').exists(), 'members with knowledge:read may run test search').toBe(true);
    expect(read.text()).toContain('knowledge.read_only');
  });

  it('names an untitled page\'s delete button by its address', async () => {
    client.listKnowledgeDocuments.mockResolvedValueOnce({ documents: [docPB({ id: 'Kd9', sourceId: uploadSource, title: '', status: KnowledgeDocumentStatus.READY, url: 'https://help.example.com/a' } as never)] });
    seed(`/knowledge/${uploadSource}`, { source: uploadSource });
    const w = await mountPage(SourcePage);
    expect(w.get('tbody button').attributes('aria-label')).toBe('knowledge.delete_document|{"name":"https://help.example.com/a"}');
  });

  it('names a crawled page by its address once, and lets search run once a page is Ready even if the source count lags', async () => {
    client.listKnowledgeDocuments.mockResolvedValueOnce({ documents: [docPB({ id: 'Kd9', sourceId: urlSource, title: '', status: KnowledgeDocumentStatus.READY, url: 'https://help.example.com/docs' } as never)] });
    seed(`/knowledge/${urlSource}`, { source: urlSource });
    const w = await mountPage(SourcePage);
    expect(w.get('tbody tr').text().match(/https:\/\/help\.example\.com\/docs/g)).toHaveLength(1);
    expect(w.get('input[type="search"]').attributes('disabled')).toBeUndefined();
  });

  it('shows a website\'s address, the crawl limits and an empty crawling list, and crawls again', async () => {
    client.listKnowledgeDocuments.mockResolvedValueOnce({ documents: [] });
    seed(`/knowledge/${urlSource}`, { source: urlSource });
    const w = await mountPage(SourcePage);
    expect(w.text()).toContain('https://help.example.com/docs');
    expect(w.text()).toContain('knowledge.crawl_limits');
    expect(w.text()).toContain('knowledge.no_pages_crawling');
    expect(w.text()).toContain('knowledge.search_needs_ready_page');
    expect(button(w, 'knowledge.crawl_again')!.attributes('disabled')).toBeDefined();
  });

  it('shows a failed crawl\'s reason as an alert', async () => {
    setKnownKeys('apperror.KNW014_robots_disallowed');
    client.listKnowledgeDocuments.mockResolvedValueOnce({ documents: [] });
    seed(`/knowledge/${failedSource}`, { source: failedSource });
    const w = await mountPage(SourcePage);
    expect(w.get('[role="alert"]').text()).toBe('apperror.KNW014_robots_disallowed');
    await button(w, 'knowledge.crawl_again')!.trigger('click');
    await flushPromises();
    expect(client.refreshKnowledgeSource).toHaveBeenCalledWith({ id: failedSource });
  });

  it('runs a test search with scores, says when nothing matched, and waits for a ready document', async () => {
    seed(`/knowledge/${uploadSource}`, { source: uploadSource });
    const w = await mountPage(SourcePage);
    await w.get('input[type="search"]').setValue('refund time');
    submitSearch(w);
    await flushPromises();
    expect(client.testSearch).toHaveBeenCalledWith({ sourceId: uploadSource, query: 'refund time' });
    const hit = w.get('ol li');
    expect(hit.text()).toContain('refund-policy.pdf · Timing');
    expect(hit.text()).toContain('knowledge.score|{"score":"0.83"}');
    expect(hit.text()).toContain('knowledge.page|{"n":2}');
    expect(hit.text()).toContain('Refunds take 5 working days.');
    client.testSearch.mockResolvedValueOnce({ hits: [] });
    submitSearch(w);
    await flushPromises();
    expect(w.text()).toContain('knowledge.no_hits');
    client.testSearch.mockResolvedValueOnce({ hits: [{ text: 'old', title: 'a', heading: '', page: 0, url: '', score: 0.5 }] });
    submitSearch(w);
    await flushPromises();
    setKnownKeys('apperror.GEN900');
    client.testSearch.mockRejectedValueOnce(connectError('GEN900'));
    submitSearch(w);
    await flushPromises();
    expect(w.find('ol').exists()).toBe(false);
    expect(w.text()).not.toContain('knowledge.no_hits');
    expect(w.text()).toContain('apperror.GEN900');
    useKnowledgeStore().sources = useKnowledgeStore().sources.map(s => ({ ...s, ready: 0 }));
    useKnowledgeStore().documents = { [uploadSource]: [] };
    await flushPromises();
    expect(w.get('input[type="search"]').attributes('disabled')).toBeDefined();
    expect(w.text()).toContain('knowledge.search_needs_ready');
  });

  it('answers an unknown source with a 404', async () => {
    seed('/knowledge/KsNopeQ8sWx3YcLm7Dxyz', { source: 'KsNopeQ8sWx3YcLm7Dxyz' });
    const errorHandler = vi.fn();
    await mountPage(SourcePage, { global: { config: { errorHandler } } });
    expect(errorHandler.mock.calls[0]![0]).toMatchObject({ statusCode: 404, statusMessage: 'error.knowledge_source_not_found' });
  });
});
