import { describe, expect, it } from 'vitest';
import { setupSteps } from './setupSteps';

const base = '/o/acme/projects/rumah';
const none = { agentConfigured: false, tried: false, published: false, whatsappConnected: false };

describe('setupSteps', () => {
  it('starts at the model provider when nothing is set up', () => {
    const steps = setupSteps({ ...none, base, providers: 0, mcpServers: 0 });
    expect(steps.map(s => s.state)).toEqual(['next', 'todo', 'todo', 'todo', 'todo', 'todo']);
    expect(steps.map(s => s.labelKey)).toEqual(['setup.step_provider', 'setup.step_mcp', 'setup.step_agent', 'setup.step_try', 'setup.step_publish', 'setup.step_channel']);
    expect(steps[0]!.to).toBe(`${base}/settings/model-providers`);
    expect(steps[2]!.to).toBe(`${base}/agents`);
  });

  it('points the agent steps at the first agent and marks the first gap as next', () => {
    const steps = setupSteps({ ...none, agentConfigured: true, base, agentSlug: 'yasaku-keuangan', providers: 2, mcpServers: 1 });
    expect(steps.map(s => s.state)).toEqual(['done', 'done', 'done', 'next', 'todo', 'todo']);
    expect(steps.map(s => s.to.slice(base.length))).toEqual(['/settings/model-providers', '/settings/mcp-servers', '/agents/yasaku-keuangan', '/agents/yasaku-keuangan/try', '/agents/yasaku-keuangan', '/agents/yasaku-keuangan/channels']);
  });

  it('never counts later steps as done without a configured agent', () => {
    const steps = setupSteps({ agentConfigured: true, tried: true, published: true, whatsappConnected: true, base, providers: 1, mcpServers: 1 });
    expect(steps.map(s => s.state)).toEqual(['done', 'done', 'next', 'todo', 'todo', 'todo']);
  });
});
