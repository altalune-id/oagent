// @vitest-environment happy-dom
import type { VueWrapper } from '@vue/test-utils';
import { create } from '@bufbuild/protobuf';
import { flushPromises, mount } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import AppModal from '#base/components/ui/AppModal.vue';
import { APIKeySchema } from '#gen/apikey/v1/apikey_pb';
import { WhoamiResponseSchema } from '#gen/auth/v1/auth_pb';
import { WebhookSchema } from '#gen/webhook/v1/webhook_pb';
import { mountPage } from '#test/mount-page';
import { setClient, setRoute } from '#test/nuxt-globals';
import APIKeyCreateModal from '~/components/apikey/create/APIKeyCreateModal.vue';
import APIKeyList from '~/components/apikey/list/APIKeyList.vue';
import MemberList from '~/components/member/MemberList.vue';
import OrgCreateModal from '~/components/org/create/OrgCreateModal.vue';
import OrgFields from '~/components/org/create/OrgFields.vue';
import WebhookCreateModal from '~/components/webhook/create/WebhookCreateModal.vue';
import WebhookList from '~/components/webhook/list/WebhookList.vue';
import AgentsPage from '~/pages/o/[org]/projects/[project]/agents/index.vue';
import ProjectSettings from '~/pages/o/[org]/projects/[project]/settings.vue';
import WebhooksPage from '~/pages/o/[org]/projects/[project]/webhooks/index.vue';
import ProjectsPage from '~/pages/o/[org]/projects/index.vue';
import OrgSettings from '~/pages/o/[org]/settings/index.vue';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

const orgId = 'B6rTn1YkM8wQz3XpL5vGd';
const projectId = 'x6MH9WPWjktMwCBKHy4pF';
const openDialog = () => [...document.querySelectorAll('dialog')].find(d => d.open);

beforeEach(() => {
  setActivePinia(createPinia());
});

describe('org create modal copy', () => {
  it('takes its placeholders and the random-slug help from the locales', () => {
    const w = mount(OrgCreateModal);
    const [name, slug] = w.findAll('input');
    expect(name!.attributes('placeholder')).toBe('org.name_placeholder');
    expect(slug!.attributes('placeholder')).toBe('org.slug_placeholder');
    expect(w.text()).toContain('org.slug_random_help');
    expect(w.text()).not.toContain('generated when empty');
  });
});

describe('org fields', () => {
  it('names each input by its fieldset legend and shows its error', () => {
    const w = mount(OrgFields, { props: { name: '', slug: '', fieldErrors: { slug: 'taken' } } });
    const legends = w.findAll('fieldset > legend.fieldset-legend');
    expect(legends.map(l => l.text())).toEqual(['common.name', 'common.slug']);
    const [name, slug] = w.findAll('input');
    expect(name!.attributes('aria-labelledby')).toBe(legends[0]!.attributes('id'));
    expect(slug!.attributes('aria-labelledby')).toBe(legends[1]!.attributes('id'));
    expect(slug!.attributes('aria-invalid')).toBe('true');
    expect(w.text()).toContain('taken');
  });

  it('is the same block in the modal and on onboarding', () => {
    expect(mount(OrgCreateModal).findComponent(OrgFields).exists()).toBe(true);
  });
});

describe('member list', () => {
  it('collapses row actions into a menu below sm, named for the member', () => {
    const w = mount(MemberList, { props: { rows: [{ userId: 'F5pLm2XwQ8kRt3VnZ7bYc', email: 'budi@acme.test', name: 'Budi', role: 'member', initials: 'B', self: false, roles: ['member'], removable: true }] } });
    const menu = w.get('.dropdown');
    expect(menu.classes()).toEqual(expect.arrayContaining(['dropdown-left', 'sm:hidden']));
    expect(menu.get('[role="button"] .sr-only').text()).toBe('Actions for Budi');
    expect(menu.find('[data-icon="lucide:ellipsis"]').exists()).toBe(true);
    expect(w.findAll('.max-sm\\:hidden button').map(b => b.text())).toContain('Remove');
  });
});

describe('destructive actions ask first', () => {
  it('revokes an API key only after a confirm naming the key', async () => {
    const revokeAPIKey = vi.fn().mockResolvedValue({});
    setClient('$apiKeyClient', { listAPIKeys: vi.fn().mockResolvedValue({ keys: [create(APIKeySchema, { id: 'K9tWq4ZbN2xLm7VcR5pHs', name: 'deploy-bot', kind: 'org', secretHint: 'ab12' })] }), revokeAPIKey });
    const w = await mountPage(APIKeyList, { pageProps: { orgId, canManage: true }, attachTo: document.body });
    await w.get('tbody button').trigger('click');
    expect(revokeAPIKey).not.toHaveBeenCalled();
    expect(openDialog()!.textContent).toContain('apikeys.revoke_confirm');
    expect(openDialog()!.textContent).toContain('apikeys.revoke_body|{"name":"deploy-bot","hint":"ab12"}');
    const confirm = [...openDialog()!.querySelectorAll('button')].find(b => b.textContent!.includes('common.revoke'))!;
    confirm.click();
    await vi.waitFor(() => expect(revokeAPIKey).toHaveBeenCalledWith({ orgId, hint: 'ab12' }));
  });

  it('asks before deleting a webhook', async () => {
    const deleteWebhook = vi.fn().mockResolvedValue({});
    setClient('$webhookClient', { listWebhooks: vi.fn().mockResolvedValue({ webhooks: [create(WebhookSchema, { id: 'H3nVb8XqT1wLp6RkZ2mJc', projectId, url: 'https://hooks.acme.test/oagent', active: true, eventTypes: [] })] }), deleteWebhook });
    const w = await mountPage(WebhookList, { pageProps: { orgId, projectId, canManage: true }, attachTo: document.body });
    await w.findAll('button').find(b => b.text().includes('common.delete'))!.trigger('click');
    expect(deleteWebhook).not.toHaveBeenCalled();
    expect(openDialog()!.textContent).toContain('webhooks.delete_title');
    expect(openDialog()!.textContent).toContain('https://hooks.acme.test/oagent');
  });

  it('asks before deleting an agent', async () => {
    useOrgsStore().items = [{ id: orgId, slug: 'acme', name: 'Acme' }];
    useProjectsStore().items = [{ id: projectId, orgId, slug: 'web', name: 'Web' }];
    setRoute({ path: '/o/acme/projects/web/agents', params: { org: 'acme', project: 'web' } });
    signedInWith('agent:read', 'agent:write');
    const deleteAgent = vi.fn().mockResolvedValue({});
    setClient('$agentClient', { listAgents: vi.fn().mockResolvedValue({ agents: [{ id: 'A7kPq2WxN4mLt8VcR3bZs', projectId, slug: 'support', name: 'Support', description: '' }] }), deleteAgent });
    const w = await mountPage(AgentsPage, { attachTo: document.body });
    await w.findAll('button').find(b => b.text().includes('common.delete'))!.trigger('click');
    expect(deleteAgent).not.toHaveBeenCalled();
    expect(openDialog()!.textContent).toContain('agent.delete_title');
    expect(openDialog()!.textContent).toContain('agent.delete_body|{"name":"Support"}');
  });
});

describe('a one-time secret stays until Done', () => {
  const keyOrgId = 'V3StGXR8Z5jdHj6BmyT2a';

  async function escapeKeepsIt(w: VueWrapper, secret: string) {
    expect(openDialog()!.textContent).toContain(secret);
    expect(openDialog()!.querySelector('.modal-backdrop')).toBeNull();
    await w.get('dialog').trigger('cancel');
    expect(w.getComponent(AppModal).emitted('close')).toBeUndefined();
    expect(openDialog()!.textContent).toContain(secret);
  }

  it('keeps a new API key open on Escape', async () => {
    useProjectsStore().items = [{ id: projectId, orgId: keyOrgId, slug: 'web', name: 'Web' }];
    setClient('$apiKeyClient', { createAPIKey: vi.fn().mockResolvedValue({ plaintext: 'oak_once' }) });
    const w = mount(APIKeyCreateModal, { props: { orgId: keyOrgId }, attachTo: document.body });
    await w.get('button').trigger('click');
    await w.get('dialog input[type="text"]').setValue('deploy');
    await w.findAll('dialog button').find(b => b.text() === 'common.create')!.trigger('click');
    await flushPromises();
    await escapeKeepsIt(w, 'oak_once');
  });

  it('keeps a new webhook signing secret open on Escape', async () => {
    setClient('$webhookClient', { createWebhook: vi.fn().mockResolvedValue({ secret: 'whsec_once' }) });
    const w = mount(WebhookCreateModal, { props: { orgId: keyOrgId, projectId }, attachTo: document.body });
    await w.get('button').trigger('click');
    await w.get('dialog input[type="url"]').setValue('https://hooks.acme.test/oagent');
    await w.get('dialog input[type="checkbox"]').setValue(true);
    await w.findAll('dialog button').find(b => b.text() === 'common.create')!.trigger('click');
    await flushPromises();
    await escapeKeepsIt(w, 'whsec_once');
  });

  it('keeps a rotated webhook secret open on Escape', async () => {
    setClient('$webhookClient', {
      listWebhooks: vi.fn().mockResolvedValue({ webhooks: [create(WebhookSchema, { id: 'H3nVb8XqT1wLp6RkZ2mJc', projectId, url: 'https://hooks.acme.test/oagent', active: true, eventTypes: [] })] }),
      rotateWebhookSecret: vi.fn().mockResolvedValue({ secret: 'whsec_rotated' }),
    });
    const w = await mountPage(WebhookList, { pageProps: { orgId, projectId, canManage: true }, attachTo: document.body });
    await w.findAll('button').find(b => b.text().includes('webhooks.rotate'))!.trigger('click');
    await flushPromises();
    await escapeKeepsIt(w, 'whsec_rotated');
  });
});

function signedInWith(...permissions: string[]) {
  setClient('$authClient', { whoami: vi.fn().mockResolvedValue(create(WhoamiResponseSchema, { source: 'local', userId: 'F5pLm2XwQ8kRt3VnZ7bYc', permissions })) });
}

function inProject(path: string) {
  useOrgsStore().items = [{ id: orgId, slug: 'acme', name: 'Acme', role: 'member' }];
  useProjectsStore().items = [{ id: projectId, orgId, slug: 'web', name: 'Web' }];
  setRoute({ path: `/o/acme/projects/web${path}`, params: { org: 'acme', project: 'web' } });
}

const buttonTexts = (w: { findAll: (s: string) => { text: () => string }[] }) => w.findAll('button').map(b => b.text());

describe('actions follow the caller\'s permissions', () => {
  it('shows API key Revoke only to people who manage keys', async () => {
    setClient('$apiKeyClient', { listAPIKeys: vi.fn().mockResolvedValue({ keys: [create(APIKeySchema, { id: 'K9tWq4ZbN2xLm7VcR5pHs', name: 'deploy-bot', kind: 'org', secretHint: 'ab12' })] }) });
    const w = await mountPage(APIKeyList, { pageProps: { orgId } });
    expect(w.findAll('tbody button')).toHaveLength(0);
  });

  it('makes org settings read-only for people without org:write', async () => {
    useOrgsStore().items = [{ id: orgId, slug: 'acme', name: 'Acme', role: 'member' }];
    setRoute({ path: '/o/acme/settings', params: { org: 'acme' } });
    signedInWith('org:read');
    const w = await mountPage(OrgSettings);
    expect(w.find('form').exists()).toBe(false);
    expect(w.get('dl').text()).toContain('Acme');
    expect(w.text()).toContain('settings.read_only');
  });

  it('keeps org settings editable with org:write', async () => {
    useOrgsStore().items = [{ id: orgId, slug: 'acme', name: 'Acme', role: 'owner' }];
    setRoute({ path: '/o/acme/settings', params: { org: 'acme' } });
    signedInWith('org:read', 'org:write');
    const w = await mountPage(OrgSettings);
    expect(w.find('form').exists()).toBe(true);
    expect(w.text()).not.toContain('settings.read_only');
  });

  it('makes project settings read-only for people without project:write', async () => {
    inProject('/settings');
    signedInWith('project:read');
    const w = await mountPage(ProjectSettings);
    expect(w.find('form').exists()).toBe(false);
    expect(w.text()).toContain('project.read_only');
  });

  it('offers New project and Edit only with project:write', async () => {
    inProject('');
    setRoute({ path: '/o/acme/projects', params: { org: 'acme' } });
    signedInWith('project:read');
    const w = await mountPage(ProjectsPage);
    expect(buttonTexts(w).filter(t => t.includes('project.new') || t.includes('common.edit'))).toEqual([]);
  });

  it('offers no agent changes without agent:write', async () => {
    inProject('/agents');
    signedInWith('agent:read');
    setClient('$agentClient', { listAgents: vi.fn().mockResolvedValue({ agents: [{ id: 'A7kPq2WxN4mLt8VcR3bZs', projectId, slug: 'support', name: 'Support', description: '' }] }) });
    const w = await mountPage(AgentsPage);
    expect(w.text()).toContain('Support');
    expect(buttonTexts(w).filter(t => /agent\.new|common\.(?:edit|delete)/.test(t))).toEqual([]);
  });

  it('offers no webhook changes without webhook:write, only the delivery log', async () => {
    inProject('/webhooks');
    signedInWith('webhook:read');
    setClient('$webhookClient', { listWebhooks: vi.fn().mockResolvedValue({ webhooks: [create(WebhookSchema, { id: 'H3nVb8XqT1wLp6RkZ2mJc', projectId, url: 'https://hooks.acme.test/oagent', active: true, eventTypes: [] })] }) });
    const w = await mountPage(WebhooksPage);
    expect(w.text()).toContain('https://hooks.acme.test/oagent');
    expect(buttonTexts(w).filter(t => /webhooks\.(?:new|ping|rotate|activate|deactivate|retry)|common\.delete/.test(t))).toEqual([]);
  });
});
