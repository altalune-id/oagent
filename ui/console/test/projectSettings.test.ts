// @vitest-environment happy-dom
import { create } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { flushPromises, RouterLinkStub } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { fakeRuntimeClients, providerPBs } from '~~/test/fixtures/runtime';
import { WhoamiResponseSchema } from '#gen/auth/v1/auth_pb';
import { ModelProviderKind, ModelProviderSchema } from '#gen/modelprovider/v1/modelprovider_pb';
import { HTTPSecretSchema, WebSearchProviderKind, WebSearchProviderSchema } from '#gen/webtool/v1/webtool_pb';
import { mountPage } from '#test/mount-page';
import { connectError, setClient, setKnownKeys, setRoute } from '#test/nuxt-globals';
import McpServersPage from '~/pages/o/[org]/projects/[project]/settings/mcp-servers.vue';
import ProvidersPage from '~/pages/o/[org]/projects/[project]/settings/model-providers.vue';
import WebToolsPage from '~/pages/o/[org]/projects/[project]/settings/web-tools.vue';
import { secretRefusedCode, useMcpServersStore } from '~/stores/mcpServers';
import { useModelProvidersStore } from '~/stores/modelProviders';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

const orgId = 'V3StGXR8Z5jdHj6BmyT2a';
const projectId = 'x6MH9WPWjktMwCBKHy4pF';

function seed(permissions = ['project:read', 'project:write', 'provider:read', 'provider:write', 'mcpserver:read', 'mcpserver:write']) {
  useOrgsStore().items = [{ id: orgId, slug: 'acme', name: 'Acme', role: 'owner', permissions: [] }];
  useProjectsStore().items = [{ id: projectId, orgId, slug: 'rumah', name: 'Rumah Tangga' }];
  setClient('$authClient', { whoami: vi.fn().mockResolvedValue(create(WhoamiResponseSchema, { permissions })) });
}

const button = (w: Awaited<ReturnType<typeof mountPage>>, text: string) => w.findAll('button').find(b => b.text() === text)!;

let clients: ReturnType<typeof fakeRuntimeClients>;

beforeEach(() => {
  setActivePinia(createPinia());
  clients = fakeRuntimeClients();
  setRoute({ path: '/o/acme/projects/rumah/settings/model-providers', params: { org: 'acme', project: 'rumah' } });
});

describe('model providers page', () => {
  it('lists the providers with models, a no-price badge and key status, without a Preview badge', async () => {
    seed();
    const w = await mountPage(ProvidersPage);
    expect(clients.provider.listModelProviders).toHaveBeenCalledWith({ projectId });
    expect(w.find('[role="note"]').exists()).toBe(false);
    const rows = w.findAll('tbody tr');
    expect(rows).toHaveLength(3);
    expect(w.findAll('thead th').map(h => h.text()).slice(0, 2)).toEqual(['common.name', 'providers.kind']);
    expect(rows.map(r => r.findAll('td')[1]!.text())).toEqual(['providers.kind_openai_compatible', 'providers.kind_openai_compatible', 'providers.kind_anthropic']);
    expect(rows[0]!.text()).toContain('openai/gpt-4.1-mini, google/gemini-2.5-flash');
    expect(rows[0]!.text()).not.toContain('providers.no_price');
    expect(rows[1]!.text()).toContain('providers.no_price');
    expect(rows[0]!.text()).toContain('providers.key_set_short');
  });

  it('shows the in-use refusal (PRV005) in the confirm and deletes the others', async () => {
    seed();
    setKnownKeys('apperror.PRV005_delete');
    clients.provider.deleteModelProvider.mockRejectedValueOnce(connectError('PRV005', { reason: 'delete' }));
    const w = await mountPage(ProvidersPage, { attachTo: document.body });
    const remove = (row: number) => w.findAll('tbody tr')[row]!.findAll('li button').find(b => b.text() === 'common.delete')!;
    const confirm = async () => {
      await flushPromises();
      const dialog = w.findAll('dialog').find(d => d.text().includes('providers.delete_body'))!;
      await dialog.findAll('button').find(b => b.text() === 'common.delete')!.trigger('click');
      await flushPromises();
      return dialog;
    };
    await remove(0).trigger('click');
    expect((await confirm()).text()).toContain('providers.in_use');
    expect(useModelProvidersStore().items).toHaveLength(3);
    await remove(1).trigger('click');
    await confirm();
    expect(clients.provider.deleteModelProvider).toHaveBeenLastCalledWith({ id: 'Pv9pHs4YvT7kMq7WcXe2B' });
    expect(useModelProvidersStore().items.map(p => p.name)).toEqual(['OpenRouter', 'Anthropic']);
    w.unmount();
  });

  it('shows the empty state with Add provider, and a write-only key field', async () => {
    seed();
    clients.provider.listModelProviders.mockResolvedValue({ modelProviders: [] });
    const w = await mountPage(ProvidersPage);
    expect(w.text()).toContain('providers.empty');
    await button(w, 'providers.add').trigger('click');
    expect(w.find('input[type="password"]').attributes('autocomplete')).toBe('new-password');
  });

  it('edits with Replace key and shows prices in USD', async () => {
    seed();
    const w = await mountPage(ProvidersPage);
    await button(w, 'OpenRouter').trigger('click');
    const dialog = w.findAll('dialog').find(d => d.text().includes('providers.connection'))!;
    expect(dialog.find('input[type="password"]').exists()).toBe(false);
    expect(dialog.findAll('input[inputmode="decimal"]').map(i => (i.element as HTMLInputElement).value).slice(0, 2)).toEqual(['0.40', '1.60']);
    await dialog.findAll('button').find(b => b.text() === 'providers.replace_key')!.trigger('click');
    expect(dialog.find('input[type="password"]').exists()).toBe(true);
  });

  it('opens a read-only summary without provider:write', async () => {
    seed(['project:read', 'provider:read']);
    const w = await mountPage(ProvidersPage);
    expect(w.text()).toContain('project.read_only');
    expect(w.findAll('button').some(b => b.text() === 'providers.add')).toBe(false);
    await button(w, 'OpenRouter').trigger('click');
    expect(w.find('form[novalidate]').exists()).toBe(false);
    expect(w.find('dl').text()).toContain('https://openrouter.ai/api/v1');
    expect(w.find('dl').text()).toContain('providers.kind');
    expect(w.find('dl').text()).toContain('providers.kind_openai_compatible');
  });

  it('adds an Anthropic provider: the type fills the URL, a /v1 URL warns but still saves', async () => {
    seed();
    clients.provider.createModelProvider.mockImplementation(async (req: { name: string }) => ({ modelProvider: create(ModelProviderSchema, { ...req, id: 'Pv1aNt5RpQ8sWx3YcLm7D', apiKeySet: true }) }));
    const w = await mountPage(ProvidersPage, { attachTo: document.body });
    await button(w, 'providers.add').trigger('click');
    const dialog = w.findAll('dialog').find(d => d.text().includes('providers.connection'))!;
    await dialog.find('select').setValue(String(ModelProviderKind.ANTHROPIC));
    const url = dialog.find('input[type="url"]');
    expect((url.element as HTMLInputElement).value).toBe('https://api.anthropic.com');
    expect(dialog.text()).toContain('providers.url_hint_anthropic');
    expect(dialog.text()).not.toContain('providers.url_v1_warning');
    await url.setValue('https://api.anthropic.com/v1');
    expect(dialog.find('[role="alert"]').text()).toContain('providers.url_v1_warning');
    await dialog.find('input[type="text"]').setValue('Claude');
    await dialog.find('input[type="password"]').setValue('sk-ant');
    await dialog.find('input[placeholder="claude-sonnet-5-5"]').setValue('claude-sonnet-5-5');
    await dialog.findAll('button').find(b => b.text() === 'providers.save')!.trigger('click');
    await flushPromises();
    expect(clients.provider.createModelProvider).toHaveBeenCalledWith(expect.objectContaining({ kind: ModelProviderKind.ANTHROPIC, baseUrl: 'https://api.anthropic.com/v1' }));
    w.unmount();
  });

  it('shows the type read-only with the lock hint when editing', async () => {
    seed();
    const w = await mountPage(ProvidersPage);
    await button(w, 'Anthropic').trigger('click');
    const dialog = w.findAll('dialog').find(d => d.text().includes('providers.connection'))!;
    expect(dialog.find('select').exists()).toBe(false);
    expect(dialog.text()).toContain('providers.kind_anthropic');
    expect(dialog.text()).toContain('providers.kind_locked');
    expect(dialog.text()).toContain('providers.cache_write_hint');
  });

  it('marks a model for embeddings with a hint, and saves the flag; Anthropic models have no Embedding box', async () => {
    seed();
    clients.provider.updateModelProvider.mockImplementation(async (req: object) => ({ modelProvider: create(ModelProviderSchema, { ...providerPBs()[0]!, ...req }) }));
    const w = await mountPage(ProvidersPage, { attachTo: document.body });
    await button(w, 'OpenRouter').trigger('click');
    const dialog = w.findAll('dialog').find(d => d.text().includes('providers.connection'))!;
    const boxes = dialog.findAll('input[type="checkbox"]').filter(c => c.attributes('aria-label')?.startsWith('providers.embedding_for'));
    expect(boxes.map(b => b.attributes('aria-label'))).toEqual(['providers.embedding_for|{"model":"openai/gpt-4.1-mini"}', 'providers.embedding_for|{"model":"google/gemini-2.5-flash"}']);
    expect(dialog.text()).toContain('providers.embedding_hint');
    await boxes[1]!.setValue(true);
    await dialog.findAll('button').find(b => b.text() === 'providers.save')!.trigger('click');
    await flushPromises();
    const sent = clients.provider.updateModelProvider.mock.calls[0]![0] as { models: { id: string, embedding: boolean }[] };
    expect(sent.models.map(m => [m.id, m.embedding])).toEqual([['openai/gpt-4.1-mini', false], ['google/gemini-2.5-flash', true]]);
    w.unmount();
    const again = await mountPage(ProvidersPage);
    await button(again, 'Anthropic').trigger('click');
    const anthropic = again.findAll('dialog').find(d => d.text().includes('providers.connection'))!;
    expect(anthropic.findAll('input[type="checkbox"]').some(c => c.attributes('aria-label')?.startsWith('providers.embedding_for'))).toBe(false);
  });

  it('saves an embedding model with only an input price, and marks a bad price on the price fields, not the model id', async () => {
    seed();
    clients.provider.updateModelProvider.mockImplementation(async (req: object) => ({ modelProvider: create(ModelProviderSchema, { ...providerPBs()[0]!, ...req }) }));
    const w = await mountPage(ProvidersPage, { attachTo: document.body });
    await button(w, 'OpenRouter').trigger('click');
    const dialog = w.findAll('dialog').find(d => d.text().includes('providers.connection'))!;
    const row = dialog.findAll('ul > li')[1]!;
    await row.get('input[aria-label^="providers.embedding_for"]').setValue(true);
    await row.findAll('input[inputmode="decimal"]')[1]!.setValue('');
    await row.findAll('input[inputmode="decimal"]')[0]!.setValue('abc');
    await dialog.findAll('button').find(b => b.text() === 'providers.save')!.trigger('click');
    await flushPromises();
    expect(row.get('input[placeholder]').attributes('aria-invalid')).toBe('false');
    expect(row.findAll('input[inputmode="decimal"]')[0]!.attributes('aria-invalid')).toBe('true');
    expect(clients.provider.updateModelProvider).not.toHaveBeenCalled();
    await row.findAll('input[inputmode="decimal"]')[0]!.setValue('0.02');
    await dialog.findAll('button').find(b => b.text() === 'providers.save')!.trigger('click');
    await flushPromises();
    const sent = clients.provider.updateModelProvider.mock.calls[0]![0] as { models: { id: string, embedding: boolean, prices?: { outputMicros: bigint } }[] };
    expect(sent.models[1]).toMatchObject({ embedding: true, prices: { inputMicros: 20_000n, outputMicros: 0n } });
    w.unmount();
  });

  it('explains a delete refused because the project embeds with the provider', async () => {
    seed();
    setKnownKeys('apperror.PRV005_embedding_model');
    clients.provider.deleteModelProvider.mockRejectedValueOnce(connectError('PRV005', { reason: 'embedding_model' }));
    const w = await mountPage(ProvidersPage, { attachTo: document.body });
    await w.findAll('tbody tr')[0]!.findAll('li button').find(b => b.text() === 'common.delete')!.trigger('click');
    await flushPromises();
    const dialog = w.findAll('dialog').find(d => d.text().includes('providers.delete_body'))!;
    await dialog.findAll('button').find(b => b.text() === 'common.delete')!.trigger('click');
    await flushPromises();
    expect(dialog.text()).toContain('apperror.PRV005_embedding_model');
    expect(dialog.text()).not.toContain('providers.in_use');
    w.unmount();
  });
});

describe('mcp servers page', () => {
  it('lists the servers and opens yasaku\'s tools from ListMcpServerTools, with the long name refused', async () => {
    seed();
    const w = await mountPage(McpServersPage);
    expect(w.findAll('tbody tr').map(r => r.text().includes('mcpservers.untrusted_badge'))).toEqual([false, true]);
    expect(clients.server.listMcpServerTools).not.toHaveBeenCalled();
    await w.findAll('tbody tr')[0]!.findAll('button').find(b => b.text() === 'mcpservers.show_tools')!.trigger('click');
    await flushPromises();
    expect(clients.server.listMcpServerTools).toHaveBeenCalledWith({ id: 'Ms2bVn7QxK4pRt9KwZc3H' });
    const dialog = w.findAll('dialog').find(d => d.text().includes('mcpservers.tools_of'))!;
    expect(dialog.findAll('li.list-row')).toHaveLength(13);
    expect(dialog.text()).toContain('yasaku__delete_tx');
    expect(dialog.text()).toContain('mcptools.not_grantable');
    expect(button(w, 'mcpservers.tool_count|{"n":13}').exists()).toBe(true);
  });

  it('shows an unreachable server (MCS005) and loads again from the row menu', async () => {
    seed();
    setKnownKeys('apperror.MCS005');
    clients.server.listMcpServerTools.mockRejectedValueOnce(connectError('MCS005'));
    const w = await mountPage(McpServersPage);
    await w.findAll('tbody tr')[1]!.findAll('li button').find(b => b.text() === 'mcpservers.load_tools')!.trigger('click');
    await flushPromises();
    const dialog = w.findAll('dialog').find(d => d.text().includes('mcpservers.tools_of'))!;
    expect(dialog.get('[role="alert"]').text()).toContain('apperror.MCS005');
    await dialog.findAll('button').find(b => b.text() === 'mcpservers.load_tools')!.trigger('click');
    await flushPromises();
    expect(dialog.findAll('li.list-row')).toHaveLength(2);
  });

  it('says the server refused the secret and opens the form with Replace secret', async () => {
    seed();
    clients.server.listMcpServerTools.mockRejectedValueOnce(connectError(secretRefusedCode));
    const w = await mountPage(McpServersPage);
    await w.findAll('tbody tr')[0]!.findAll('button').find(b => b.text() === 'mcpservers.show_tools')!.trigger('click');
    await flushPromises();
    const tools = w.findAll('dialog').find(d => d.text().includes('mcpservers.tools_of'))!;
    expect(tools.get('[role="alert"]').text()).toContain('mcpservers.secret_refused');
    await tools.get('[role="alert"]').findAll('button').find(b => b.text() === 'mcpservers.replace_secret')!.trigger('click');
    const form = w.findAll('dialog').find(d => d.text().includes('mcpservers.trusted_hint'))!;
    expect(form.find('input[type="password"]').exists()).toBe(true);
    expect(form.get('input[type="url"]').element.value).toBe('https://yasaku.altalune.id/mcp');
  });

  it('shows the in-use refusal (MCS004) in the confirm', async () => {
    seed();
    setKnownKeys('apperror.MCS004');
    clients.server.deleteMcpServer.mockRejectedValueOnce(connectError('MCS004'));
    const w = await mountPage(McpServersPage, { attachTo: document.body });
    await w.findAll('tbody tr')[0]!.findAll('li button').find(b => b.text() === 'common.delete')!.trigger('click');
    await flushPromises();
    const dialog = w.findAll('dialog').find(d => d.text().includes('mcpservers.delete_body'))!;
    await dialog.findAll('button').find(b => b.text() === 'common.delete')!.trigger('click');
    await flushPromises();
    expect(dialog.text()).toContain('mcpservers.in_use');
    expect(useMcpServersStore().items).toHaveLength(2);
    w.unmount();
  });
});

describe('mcp server modal', () => {
  it('keeps the name fixed and warns that a new URL needs the secret', async () => {
    seed();
    const w = await mountPage(McpServersPage);
    await button(w, 'yasaku').trigger('click');
    const dialog = w.findAll('dialog').find(d => d.text().includes('mcpservers.trusted_hint'))!;
    expect(dialog.get('input[placeholder="yasaku"]').attributes('disabled')).toBeDefined();
    expect(dialog.get('input[type="number"]').attributes('placeholder'), 'editing shows the stored timeout').toBe('20');
    expect(dialog.text()).not.toContain('mcpservers.url_changed_secret');
    expect(dialog.find('input[type="password"]').exists()).toBe(false);
    await dialog.get('input[type="url"]').setValue('https://moved.example.id/mcp');
    expect(dialog.text()).toContain('mcpservers.url_changed_secret');
    expect(dialog.find('input[type="password"]').exists()).toBe(true);
  });

  it('prefills the server\'s default timeout for a new server, bounds it by the server\'s range, and lists tools only after saving', async () => {
    seed();
    const w = await mountPage(McpServersPage);
    await button(w, 'mcpservers.add').trigger('click');
    const dialog = w.findAll('dialog').find(d => d.text().includes('mcpservers.trusted_hint'))!;
    const timeout = dialog.get('input[type="number"]');
    expect((timeout.element as HTMLInputElement).value).toBe('');
    expect(timeout.attributes('placeholder')).toBe('mcpservers.timeout_placeholder|{"n":7}');
    expect([timeout.attributes('min'), timeout.attributes('max')]).toEqual(['2', '18']);
    expect(dialog.text()).toContain('mcpservers.timeout_hint|{"min":2,"max":18,"n":7}');
    expect(dialog.text()).toContain('mcpservers.tools_after_save');
  });
});

describe('web tools page', () => {
  const { BRAVE, EXA, TAVILY } = WebSearchProviderKind;
  const at = timestampFromDate(new Date('2026-11-01T10:00:00Z'));
  const secretPB = create(HTTPSecretSchema, { id: 'UxVy4UK3vmrHzPZ273hCA', host: 'api.example.com', header: 'x-api-key', updatedAt: at });

  function webTools(over: { provider?: object, usedBy?: object[], secrets?: object[] } = {}) {
    const client = {
      getWebSearchProvider: vi.fn().mockResolvedValue({ provider: over.provider, availableKinds: [BRAVE, EXA], usedBy: over.usedBy ?? [] }),
      listHTTPSecrets: vi.fn().mockResolvedValue({ secrets: over.secrets ?? [] }),
      setWebSearchProvider: vi.fn(async (req: { kind: WebSearchProviderKind, pricePerCallMicros: bigint }) => ({ provider: create(WebSearchProviderSchema, { kind: req.kind, keySet: true, pricePerCallMicros: req.pricePerCallMicros, updatedAt: at }) })),
      deleteWebSearchProvider: vi.fn().mockResolvedValue({}),
      setHTTPSecret: vi.fn(async (req: { host: string, header: string }) => ({ secret: create(HTTPSecretSchema, { id: req.host === 'api.example.com' ? 'UxVy4UK3vmrHzPZ273hCA' : 'a6SkMeA9Zj2rjRmCXx2dM', host: req.host, header: req.header.toLowerCase(), updatedAt: at }) })),
      deleteHTTPSecret: vi.fn().mockResolvedValue({}),
    };
    setClient('$webToolClient', client);
    setClient('$agentClient', { listAgents: vi.fn().mockResolvedValue({ agents: [{ id: 'Ag7kLm2NpQ4rSt6UvWx8Y', projectId, slug: 'tagihan', name: 'Tagihan v2', description: '' }] }) });
    return client;
  }

  async function mountWeb(permissions?: string[]) {
    seed(permissions);
    setRoute({ path: '/o/acme/projects/rumah/settings/web-tools', params: { org: 'acme', project: 'rumah' } });
    return mountPage(WebToolsPage, { attachTo: document.body });
  }

  it('starts empty with Tavily marked coming soon and not selectable', async () => {
    const client = webTools();
    const w = await mountWeb();
    expect(client.getWebSearchProvider).toHaveBeenCalledWith({ projectId });
    expect(w.text()).toContain('webtools.no_provider');
    expect(w.text()).toContain('webtools.secrets_empty');
    expect(w.find('[role="note"]').exists()).toBe(false);
    const radios = w.findAll('input[type="radio"]');
    expect(radios.map(r => r.attributes('disabled') !== undefined)).toEqual([false, false, true]);
    const badgeId = radios[2]!.attributes('aria-describedby')!;
    expect(w.get(`#${badgeId}`).text()).toBe('webtools.coming_soon');
    expect(w.text().match(/webtools\.coming_soon/g)).toHaveLength(1);
    w.unmount();
  });

  it('saves a provider with its price and shows the key status', async () => {
    const client = webTools();
    const w = await mountWeb();
    await w.get('input[type="password"]').setValue('bsa-1');
    await w.get('input[inputmode="decimal"]').setValue('5');
    await w.get('form').trigger('submit');
    await flushPromises();
    expect(client.setWebSearchProvider).toHaveBeenCalledWith({ projectId, kind: BRAVE, apiKey: 'bsa-1', pricePerCallMicros: 5000n });
    expect(document.activeElement, 'the first save collapses the form and hands focus to Change').toBe(button(w, 'webtools.change').element);
    expect(w.text()).toContain('webtools.key_set|{"provider":"webtools.brave"}');
    expect(w.text()).toContain('webtools.summary|{"provider":"webtools.brave","price":"$5.00"}');
    expect(w.findAll('input[type="radio"]'), 'a saved provider collapses to its summary').toHaveLength(0);
    await button(w, 'webtools.change').trigger('click');
    await flushPromises();
    const radios = w.findAll('input[type="radio"]');
    expect(document.activeElement).toBe(radios[0]!.element);
    expect(w.find('section input[type="password"]').exists()).toBe(false);
    await radios[1]!.setValue(true);
    expect(w.find('section input[type="password"]').exists(), 'switching kinds needs a key').toBe(true);
    (button(w, 'common.cancel').element as HTMLButtonElement).focus();
    await button(w, 'common.cancel').trigger('click');
    await flushPromises();
    expect(w.findAll('input[type="radio"]')).toHaveLength(0);
    expect(document.activeElement, 'Cancel hands focus to Change').toBe(button(w, 'webtools.change').element);
    await button(w, 'providers.replace_key').trigger('click');
    await flushPromises();
    expect(document.activeElement).toBe(w.get('section input[type="password"]').element);
    await w.get('section input[type="password"]').setValue('bsa-2');
    await w.get('section form').trigger('submit');
    await flushPromises();
    expect(document.activeElement, 'saving a replaced key hands focus to Change').toBe(button(w, 'webtools.change').element);
    w.unmount();
  });

  it('reads a stored kind that is no longer available as no provider', async () => {
    webTools({ provider: create(WebSearchProviderSchema, { kind: TAVILY, keySet: true, pricePerCallMicros: 1000n }) });
    const w = await mountWeb();
    expect(w.text()).toContain('webtools.no_provider');
    expect(w.text()).not.toContain('webtools.key_set');
    expect((w.findAll('input[type="radio"]')[0]!.element as HTMLInputElement).checked).toBe(true);
    w.unmount();
  });

  it('links the agents that use search, names them in the remove confirm, and removes', async () => {
    const client = webTools({
      provider: create(WebSearchProviderSchema, { kind: EXA, keySet: true, pricePerCallMicros: 8000n, updatedAt: at }),
      usedBy: [{ id: 'Ag7kLm2NpQ4rSt6UvWx8Y', name: 'Tagihan v2' }, { id: 'Zz9yXw8VuT7sRq6PoN5mL', name: 'Gone' }],
    });
    const w = await mountWeb();
    expect(w.find('[role="status"].alert-warning').exists()).toBe(false);
    await button(w, 'webtools.change').trigger('click');
    expect((w.get('input[inputmode="decimal"]').element as HTMLInputElement).value).toBe('8.00');
    await button(w, 'common.cancel').trigger('click');
    const used = w.get('[data-test="used-by"]');
    expect(used.findAllComponents(RouterLinkStub)[0]!.classes()).toEqual(expect.arrayContaining(['min-h-6', 'inline-flex']));
    expect(used.findAllComponents(RouterLinkStub).map(a => [a.text(), a.props('to')])).toEqual([['Tagihan v2', '/o/acme/projects/rumah/agents/tagihan']]);
    expect(used.text()).toContain('Gone');
    await button(w, 'webtools.remove').trigger('click');
    await flushPromises();
    const dialog = w.findAll('dialog').find(d => d.text().includes('webtools.remove_title'))!;
    expect(dialog.text()).toContain('webtools.remove_body_used|{"agents":"Tagihan v2, Gone"}');
    await dialog.findAll('button').find(b => b.text() === 'webtools.remove')!.trigger('click');
    await flushPromises();
    expect(client.deleteWebSearchProvider).toHaveBeenCalledWith({ projectId });
    await new Promise(resolve => setTimeout(resolve, 0));
    expect(document.activeElement, 'focus lands on the provider choice, not the page').toBe(w.findAll('input[type="radio"]')[0]!.element);
    const warning = w.get('[role="status"].alert-warning');
    expect(warning.text()).toContain('webtools.no_provider_used|{"agents":"Tagihan v2, Gone"}');
    expect(warning.find('[data-icon="lucide:triangle-alert"]').exists()).toBe(true);
    w.unmount();
  });

  it('loads the provider afresh on every visit, so Used by names every agent', async () => {
    const client = webTools();
    (await mountWeb()).unmount();
    client.getWebSearchProvider.mockResolvedValue({ provider: create(WebSearchProviderSchema, { kind: BRAVE, keySet: true, pricePerCallMicros: 0n }), availableKinds: [BRAVE, EXA], usedBy: [{ id: 'Ag7kLm2NpQ4rSt6UvWx8Y', name: 'Tagihan v2' }] });
    const w = await mountPage(WebToolsPage, { attachTo: document.body });
    expect(client.getWebSearchProvider).toHaveBeenCalledTimes(2);
    expect(w.get('[data-test="used-by"]').text()).toContain('Tagihan v2');
    w.unmount();
  });

  it('lets an admin remove a stored provider whose kind is no longer available', async () => {
    const client = webTools({ provider: create(WebSearchProviderSchema, { kind: TAVILY, keySet: true, pricePerCallMicros: 1000n }) });
    const w = await mountWeb();
    expect(w.text()).toContain('webtools.no_provider');
    await button(w, 'webtools.remove').trigger('click');
    await flushPromises();
    const dialog = w.findAll('dialog').find(d => d.text().includes('webtools.remove_title'))!;
    await dialog.findAll('button').find(b => b.text() === 'webtools.remove')!.trigger('click');
    await flushPromises();
    expect(client.deleteWebSearchProvider).toHaveBeenCalledWith({ projectId });
    expect(w.findAll('button').some(b => b.text() === 'webtools.remove' && !b.element.closest('dialog'))).toBe(false);
    w.unmount();
  });

  it('says no agent uses search when the list is empty', async () => {
    webTools({ provider: create(WebSearchProviderSchema, { kind: BRAVE, keySet: true, pricePerCallMicros: 0n }) });
    const w = await mountWeb();
    expect(w.text()).toContain('webtools.used_by_none');
    w.unmount();
  });

  it('shows read-only members the settings without controls', async () => {
    webTools({ provider: create(WebSearchProviderSchema, { kind: BRAVE, keySet: true, pricePerCallMicros: 0n }), secrets: [secretPB] });
    const w = await mountWeb(['project:read', 'provider:read']);
    expect(w.text()).toContain('project.read_only');
    expect(w.find('form').exists()).toBe(false);
    expect(w.findAll('button').some(b => ['webtools.add_secret', 'webtools.remove', 'providers.replace_key'].includes(b.text()))).toBe(false);
    expect(w.text()).toContain('api.example.com');
    expect(w.findAll('tbody li button')).toHaveLength(0);
    w.unmount();
  });

  it('adds, replaces and deletes an HTTP secret', async () => {
    const client = webTools({ secrets: [secretPB] });
    const w = await mountWeb();
    const row = w.findAll('tbody tr')[0]!;
    expect(row.text()).toContain('api.example.com');
    expect(row.text()).toContain('x-api-key');
    await button(w, 'webtools.add_secret').trigger('click');
    let dialog = w.findAll('dialog').find(d => d.text().includes('webtools.secret_host'))!;
    expect(dialog.get('input[type="password"]').attributes('autocomplete')).toBe('new-password');
    expect(dialog.text()).toContain('webtools.host_punycode');
    await dialog.get('input[placeholder="api.example.com"]').setValue('b.example.com');
    await dialog.get('input[placeholder="X-Api-Key"]').setValue('Authorization');
    await dialog.get('input[type="password"]').setValue('tok');
    await dialog.findAll('button').find(b => b.text() === 'webtools.save_secret')!.trigger('click');
    await flushPromises();
    expect(client.setHTTPSecret).toHaveBeenCalledWith({ projectId, host: 'b.example.com', header: 'Authorization', value: 'tok' });
    expect(w.findAll('tbody tr')).toHaveLength(2);

    await w.findAll('tbody tr')[0]!.findAll('li button').find(b => b.text() === 'webtools.replace_value')!.trigger('click');
    dialog = w.findAll('dialog').find(d => d.text().includes('webtools.secret_host'))!;
    expect(dialog.get('input[placeholder="api.example.com"]').attributes('readonly')).toBeDefined();
    await dialog.get('input[type="password"]').setValue('new');
    await dialog.findAll('button').find(b => b.text() === 'webtools.save_secret')!.trigger('click');
    await flushPromises();
    expect(client.setHTTPSecret).toHaveBeenLastCalledWith({ projectId, host: 'api.example.com', header: 'x-api-key', value: 'new' });
    await new Promise(resolve => setTimeout(resolve, 0));
    expect(document.activeElement?.getAttribute('aria-haspopup'), 'Replace value returns focus to the row menu').toBe('true');
    expect(document.activeElement?.textContent).toContain('webtools.actions_for|{"host":"api.example.com","header":"x-api-key"}');
    await w.findAll('tbody tr')[1]!.findAll('li button').find(b => b.text() === 'webtools.replace_value')!.trigger('click');
    dialog = w.findAll('dialog').find(d => d.text().includes('webtools.secret_host'))!;
    await dialog.get('input[type="password"]').setValue('newer');
    await dialog.findAll('button').find(b => b.text() === 'webtools.save_secret')!.trigger('click');
    await flushPromises();
    await new Promise(resolve => setTimeout(resolve, 0));
    expect(document.activeElement?.textContent, 'focus returns to the replaced row, not the first').toContain('webtools.actions_for|{"host":"b.example.com","header":"authorization"}');

    (document.activeElement as HTMLElement).blur();
    await w.findAll('tbody tr')[0]!.findAll('li button').find(b => b.text() === 'common.delete')!.trigger('click');
    await flushPromises();
    const confirm = w.findAll('dialog').find(d => d.text().includes('webtools.delete_secret_body'))!;
    expect(confirm.text()).toContain('webtools.delete_secret_body|{"header":"x-api-key","host":"api.example.com"}');
    await confirm.findAll('button').find(b => b.text() === 'common.delete')!.trigger('click');
    await flushPromises();
    await new Promise(resolve => setTimeout(resolve, 0));
    expect(document.activeElement, 'focus returns to Add secret').toBe(button(w, 'webtools.add_secret').element);
    expect(client.deleteHTTPSecret).toHaveBeenCalledWith({ id: 'UxVy4UK3vmrHzPZ273hCA' });
    expect(w.findAll('tbody tr')).toHaveLength(1);
    w.unmount();
  });
});
