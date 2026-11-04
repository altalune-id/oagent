// @vitest-environment happy-dom
import { create } from '@bufbuild/protobuf';
import { flushPromises } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { WhoamiResponseSchema } from '#gen/auth/v1/auth_pb';
import { mountPage } from '#test/mount-page';
import { setClient, setRoute } from '#test/nuxt-globals';
import McpServersPage from '~/pages/o/[org]/projects/[project]/settings/mcp-servers.vue';
import ProvidersPage from '~/pages/o/[org]/projects/[project]/settings/model-providers.vue';
import WebSearchPage from '~/pages/o/[org]/projects/[project]/settings/web-search.vue';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';
import { useProvidersStore } from '~/stores/providers';
import { useWebSearchStore } from '~/stores/webSearch';

const orgId = 'B6rTn1YkM8wQz3XpL5vGd';
const projectId = 'x6MH9WPWjktMwCBKHy4pF';

function seed(permissions = ['project:read', 'project:write']) {
  useOrgsStore().items = [{ id: orgId, slug: 'acme', name: 'Acme', role: 'owner', permissions: [] }];
  useProjectsStore().items = [{ id: projectId, orgId, slug: 'rumah', name: 'Rumah Tangga' }];
  setClient('$authClient', { whoami: vi.fn().mockResolvedValue(create(WhoamiResponseSchema, { permissions })) });
}

const button = (w: Awaited<ReturnType<typeof mountPage>>, text: string) => w.findAll('button').find(b => b.text() === text)!;

beforeEach(() => {
  setActivePinia(createPinia());
  setRoute({ path: '/o/acme/projects/rumah/settings/model-providers', params: { org: 'acme', project: 'rumah' } });
});

describe('model providers page', () => {
  it('lists the sample providers under the Preview badge, with model, price and key status', async () => {
    seed();
    const w = await mountPage(ProvidersPage);
    expect(w.get('.badge').text()).toBe('preview.badge');
    expect(w.get('[role="note"]').text()).toBe('preview.local_note');
    const rows = w.findAll('tbody tr');
    expect(rows).toHaveLength(2);
    expect(rows[0]!.text()).toContain('openai/gpt-4.1-mini, google/gemini-2.5-flash');
    expect(rows[0]!.text()).toContain('providers.more_models|{"n":1}');
    expect(rows[1]!.text()).not.toContain('providers.more_models');
    expect(rows[1]!.text()).toContain('providers.no_price');
    expect(rows[0]!.text()).toContain('providers.key_set_short');
  });

  it('refuses to delete a provider in use and confirms the others', async () => {
    seed();
    const w = await mountPage(ProvidersPage, { attachTo: document.body });
    const menus = w.findAll('tbody tr').map(r => r.findAll('li button').find(b => b.text() === 'common.delete')!);
    await menus[0]!.trigger('click');
    expect(w.text()).toContain('providers.in_use|{"agents":"Yasaku Keuangan v4, Draft of Tagihan"}');
    await menus[1]!.trigger('click');
    await flushPromises();
    const confirm = w.findAll('dialog').find(d => d.text().includes('providers.delete_body'))!;
    await confirm.findAll('button').find(b => b.text() === 'common.delete')!.trigger('click');
    await flushPromises();
    expect(useProvidersStore().items.map(p => p.name)).toEqual(['OpenRouter']);
    w.unmount();
  });

  it('shows the empty state with Add provider', async () => {
    seed();
    await useProvidersStore().load(projectId);
    useProvidersStore().items = [];
    const w = await mountPage(ProvidersPage);
    expect(w.text()).toContain('providers.empty');
    await button(w, 'providers.add').trigger('click');
    expect(w.find('input[type="password"]').exists()).toBe(true);
  });

  it('opens a read-only summary without project:write', async () => {
    seed(['project:read']);
    const w = await mountPage(ProvidersPage);
    expect(w.text()).toContain('project.read_only');
    expect(w.findAll('button').some(b => b.text() === 'providers.add')).toBe(false);
    await button(w, 'OpenRouter').trigger('click');
    expect(w.find('form[novalidate]').exists()).toBe(false);
    expect(w.find('dl').text()).toContain('https://openrouter.ai/api/v1');
  });
});

describe('mcp servers page', () => {
  it('lists yasaku and opens its tools, with the long name refused', async () => {
    seed();
    const w = await mountPage(McpServersPage);
    expect(w.findAll('tbody tr').map(r => r.text().includes('mcpservers.untrusted_badge'))).toEqual([false, true]);
    await button(w, 'mcpservers.tool_count|{"n":13}').trigger('click');
    const dialog = w.findAll('dialog').find(d => d.text().includes('mcpservers.tools_of'))!;
    expect(dialog.findAll('li.list-row')).toHaveLength(13);
    expect(dialog.text()).toContain('yasaku__delete_tx');
    expect(dialog.text()).toContain('mcptools.not_grantable');
  });

  it('keeps the name fixed when editing and probes from the row menu', async () => {
    vi.useFakeTimers();
    seed();
    const w = await mountPage(McpServersPage);
    await w.findAll('li button').find(b => b.text() === 'mcpservers.load_tools')!.trigger('click');
    expect(w.text()).toContain('mcpservers.probing|{"name":"yasaku"}');
    await vi.runAllTimersAsync();
    expect(w.text()).toContain('mcpservers.tool_count|{"n":13}');
    expect(w.get('form input[placeholder="yasaku"]').attributes('disabled')).toBeDefined();
    vi.useRealTimers();
  });
});

describe('mcp server modal', () => {
  it('warns that a new URL needs the secret', async () => {
    seed();
    const w = await mountPage(McpServersPage);
    await button(w, 'yasaku').trigger('click');
    const dialog = w.findAll('dialog').find(d => d.text().includes('mcpservers.trusted_hint'))!;
    expect(dialog.text()).not.toContain('mcpservers.url_changed_secret');
    expect(dialog.find('input[type="password"]').exists()).toBe(false);
    await dialog.get('input[type="url"]').setValue('https://moved.example.id/mcp');
    expect(dialog.text()).toContain('mcpservers.url_changed_secret');
    expect(dialog.find('input[type="password"]').exists()).toBe(true);
  });

  it('offers Replace secret when the server refuses the secret', async () => {
    vi.useFakeTimers();
    seed();
    const w = await mountPage(McpServersPage);
    await button(w, 'tagihan').trigger('click');
    const dialog = w.findAll('dialog').find(d => d.text().includes('mcpservers.trusted_hint'))!;
    await dialog.findAll('button').find(b => b.text() === 'mcpservers.load_tools')!.trigger('click');
    await vi.runAllTimersAsync();
    expect(dialog.get('[role="alert"]').text()).toContain('mcpservers.probe_auth');
    await dialog.findAll('button').find(b => b.text() === 'mcpservers.replace_secret' && b.element.closest('[role="alert"]'))!.trigger('click');
    expect(dialog.find('input[type="password"]').exists()).toBe(true);
    vi.useRealTimers();
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
