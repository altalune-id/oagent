import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { connectError, deferred, setClient, setKnownKeys } from '../../test/nuxt-globals';
import { useAgentsStore } from './agents';

const bot = { id: 'EW7HBTpNTTPAuNhPZB62M', projectId: 'x6MH9WPWjktMwCBKHy4pF', slug: 'bot', name: 'Bot', description: 'Helps' };
const other = { id: 'JpEPMcBEqr9DpQYJeDkjj', projectId: 'KdrF3t3FHtppBPmMgj4sy', slug: 'other', name: 'Other', description: '' };

describe('useAgentsStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('loads the agents of a project', async () => {
    const listAgents = vi.fn().mockResolvedValue({ agents: [bot, other] });
    setClient('$agentClient', { listAgents });
    const agents = useAgentsStore();
    await agents.load('x6MH9WPWjktMwCBKHy4pF');
    expect(listAgents).toHaveBeenCalledWith({ projectId: 'x6MH9WPWjktMwCBKHy4pF' });
    expect(agents.items).toEqual([bot, other]);
    expect(agents.forProject('x6MH9WPWjktMwCBKHy4pF')).toEqual([bot]);
  });

  it('treats a missing list as empty', async () => {
    setClient('$agentClient', { listAgents: vi.fn().mockResolvedValue({}) });
    const agents = useAgentsStore();
    await agents.load('x6MH9WPWjktMwCBKHy4pF');
    expect(agents.items).toEqual([]);
  });

  it('shows a Connect error with its reference', async () => {
    setKnownKeys('apperror.GEN900');
    setClient('$agentClient', { listAgents: vi.fn().mockRejectedValue(connectError('GEN900')) });
    const agents = useAgentsStore();
    await agents.load('x6MH9WPWjktMwCBKHy4pF');
    expect(agents.error).toEqual({ message: 'apperror.GEN900', reference: 'error.ref|{"id":"req-1"}' });
    expect(agents.loading).toBe(false);
  });

  it('shows the fallback for a plain error', async () => {
    setClient('$agentClient', { listAgents: vi.fn().mockRejectedValue(new Error('boom')) });
    const agents = useAgentsStore();
    await agents.load('x6MH9WPWjktMwCBKHy4pF');
    expect(agents.error).toEqual({ message: 'error.load_agents' });
  });

  it('shares an in-flight load and skips a project already loaded or empty', async () => {
    const pending = deferred<{ agents: typeof bot[] }>();
    const listAgents = vi.fn(() => pending.promise);
    setClient('$agentClient', { listAgents });
    const agents = useAgentsStore();
    await agents.ensureLoaded('');
    const first = agents.ensureLoaded('x6MH9WPWjktMwCBKHy4pF');
    const second = agents.ensureLoaded('x6MH9WPWjktMwCBKHy4pF');
    expect(listAgents).toHaveBeenCalledTimes(1);
    pending.resolve({ agents: [bot] });
    await Promise.all([first, second]);
    await agents.ensureLoaded('x6MH9WPWjktMwCBKHy4pF');
    expect(listAgents).toHaveBeenCalledTimes(1);
  });

  it('adds, updates and removes agents', () => {
    setClient('$agentClient', {});
    const agents = useAgentsStore();
    agents.add(bot);
    agents.add(bot);
    agents.add(other);
    expect(agents.items).toHaveLength(2);
    agents.update({ id: 'EW7HBTpNTTPAuNhPZB62M', name: 'Robot', description: 'New' });
    expect(agents.items[0]).toEqual({ ...bot, name: 'Robot', description: 'New' });
    expect(agents.items[1]).toEqual(other);
    agents.remove('EW7HBTpNTTPAuNhPZB62M');
    expect(agents.items).toEqual([other]);
  });
});
