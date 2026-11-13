// @vitest-environment happy-dom
import { create } from '@bufbuild/protobuf';
import { flushPromises } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { fakeRuntimeClients, providerPBs } from '~~/test/fixtures/runtime';
import { WhoamiResponseSchema } from '#gen/auth/v1/auth_pb';
import { ModelProviderKind, ModelProviderSchema } from '#gen/modelprovider/v1/modelprovider_pb';
import { mountPage } from '#test/mount-page';
import { connectError, setClient, setKnownKeys, setRoute } from '#test/nuxt-globals';
import McpServersPage from '~/pages/o/[org]/projects/[project]/settings/mcp-servers.vue';
import ProvidersPage from '~/pages/o/[org]/projects/[project]/settings/model-providers.vue';
import WebSearchPage from '~/pages/o/[org]/projects/[project]/settings/web-search.vue';
import { secretRefusedCode, useMcpServersStore } from '~/stores/mcpServers';
import { useModelProvidersStore } from '~/stores/modelProviders';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';
import { useWebSearchStore } from '~/stores/webSearch';

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

describe('web search page', () => {
  it('saves a provider key and confirms removal naming the agents that use it', async () => {
    seed();
    const w = await mountPage(WebSearchPage, { attachTo: document.body });
    expect(w.text()).toContain('websearch.none');
    expect(w.findAll('input[type="radio"]')).toHaveLength(3);
    await w.get('input[type="password"]').setValue('brv-1');
    await w.get('form').trigger('submit');
    expect(useWebSearchStore().setting.provider).toBe('brave');
    expect(w.text()).toContain('websearch.key_set');
    await button(w, 'websearch.remove').trigger('click');
    await flushPromises();
    expect(w.text()).toContain('websearch.remove_body_used|{"agents":"Tagihan v2"}');
    w.unmount();
  });
});
