// @vitest-environment happy-dom
import { create } from '@bufbuild/protobuf';
import { flushPromises } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { WhoamiResponseSchema } from '#gen/auth/v1/auth_pb';
import { mountPage } from '#test/mount-page';
import { setClient, setRoute } from '#test/nuxt-globals';
import ChannelsPage from '~/pages/o/[org]/projects/[project]/agents/[agent]/channels.vue';
import RunsPage from '~/pages/o/[org]/projects/[project]/agents/[agent]/runs/index.vue';
import ThreadPage from '~/pages/o/[org]/projects/[project]/agents/[agent]/threads/[thread].vue';
import ThreadsPage from '~/pages/o/[org]/projects/[project]/agents/[agent]/threads/index.vue';
import ApprovalsPage from '~/pages/o/[org]/projects/[project]/approvals.vue';
import EndUserPage from '~/pages/o/[org]/projects/[project]/end-users/[endUser].vue';
import EndUsersPage from '~/pages/o/[org]/projects/[project]/end-users/index.vue';
import WebSearchPage from '~/pages/o/[org]/projects/[project]/settings/web-search.vue';
import { useOrgsStore } from '~/stores/orgs';
import { useProjectsStore } from '~/stores/projects';

const failing = vi.hoisted(() => new Set<string>());

vi.mock('~/preview/samples', async (original) => {
  const real = await original<Record<string, unknown>>();
  const wrap = (name: string) => (...args: unknown[]) => {
    if (failing.has(name)) {
      throw new Error('boom');
    }
    return (real[name] as (...a: unknown[]) => unknown)(...args);
  };
  return { ...real, ...Object.fromEntries(['sampleApprovals', 'sampleThreads', 'sampleEndUsers', 'sampleWebSearch', 'sampleChannelSettings'].map(n => [n, wrap(n)])) };
});

const orgId = 'B6rTn1YkM8wQz3XpL5vGd';
const projectId = 'x6MH9WPWjktMwCBKHy4pF';
const yasaku = { id: 'Y4sKu8RmT2pLq6WnX9cVb', projectId, slug: 'yasaku-keuangan', name: 'Yasaku Keuangan', description: '', publishedVersion: 4 };

function seed(path: string, params: Record<string, string> = {}) {
  useOrgsStore().items = [{ id: orgId, slug: 'acme', name: 'Acme', role: 'owner', permissions: [] }];
  useProjectsStore().items = [{ id: projectId, orgId, slug: 'rumah', name: 'Rumah Tangga' }];
  setRoute({ path: `/o/acme/projects/rumah${path}`, params: { org: 'acme', project: 'rumah', ...params } });
  setClient('$authClient', { whoami: vi.fn().mockResolvedValue(create(WhoamiResponseSchema, { name: 'Budi', permissions: ['project:write', 'agent:write'] })) });
  setClient('$agentClient', { listAgents: vi.fn().mockResolvedValue({ agents: [yasaku] }), getAgentConfig: vi.fn().mockResolvedValue({}) });
}

const agent = { agent: 'yasaku-keuangan' };
const cases = [
  ['approvals', 'sampleApprovals', '/approvals', {}, ApprovalsPage, 'error.load_approvals'],
  ['conversations', 'sampleThreads', '/agents/yasaku-keuangan/threads', agent, ThreadsPage, 'error.load_threads'],
  ['one conversation', 'sampleThreads', '/agents/yasaku-keuangan/threads/thr_grp3Kd9Q', { ...agent, thread: 'thr_grp3Kd9Q' }, ThreadPage, 'error.load_threads'],
  ['runs', 'sampleThreads', '/agents/yasaku-keuangan/runs', agent, RunsPage, 'error.load_threads'],
  ['end users', 'sampleEndUsers', '/end-users', {}, EndUsersPage, 'error.load_end_users'],
  ['one end user', 'sampleEndUsers', '/end-users/eu_sari2', { endUser: 'eu_sari2' }, EndUserPage, 'error.load_end_users'],
  ['web search', 'sampleWebSearch', '/settings/web-search', {}, WebSearchPage, 'error.load_web_search'],
  ['channels', 'sampleChannelSettings', '/agents/yasaku-keuangan/channels', agent, ChannelsPage, 'error.load_channels'],
] as const;

beforeEach(() => {
  setActivePinia(createPinia());
  failing.clear();
  vi.stubGlobal('definePageMeta', vi.fn());
});

describe('load errors', () => {
  it.each(cases)('shows a retryable error when %s fail to load', async (_name, sample, path, params, page, key) => {
    failing.add(sample);
    seed(path, params);
    const w = await mountPage(page);
    await flushPromises();
    expect(w.get('[role="alert"]').text()).toContain(key);
    failing.clear();
    await w.findAll('button').find(b => b.text() === 'common.retry')!.trigger('click');
    await flushPromises();
    expect(w.find('[role="alert"]').exists()).toBe(false);
  });
});
