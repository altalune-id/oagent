import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { useAgentsStore } from '~/stores/agents';
import { connectError, deferred, setClient, setKnownKeys } from '../../../../test/nuxt-globals';
import { useAgentCreate } from './useAgentCreate';

const agent = { id: 'EW7HBTpNTTPAuNhPZB62M', projectId: 'x6MH9WPWjktMwCBKHy4pF', slug: 'bot', name: 'Bot', description: 'Helps' };

describe('useAgentCreate', () => {
  let createAgent: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    setActivePinia(createPinia());
    createAgent = vi.fn().mockResolvedValue({ agent });
    setClient('$agentClient', { createAgent });
  });

  it('creates an agent, adds it to the store and resets the form', async () => {
    const form = useAgentCreate('V3StGXR8Z5jdHj6BmyT2a', 'x6MH9WPWjktMwCBKHy4pF');
    form.show();
    form.name.value = '  Bot  ';
    form.slug.value = ' bot ';
    form.description.value = 'Helps';
    await form.submit();
    expect(createAgent).toHaveBeenCalledWith({ orgId: 'V3StGXR8Z5jdHj6BmyT2a', projectId: 'x6MH9WPWjktMwCBKHy4pF', slug: 'bot', name: 'Bot', description: 'Helps' });
    expect(useAgentsStore().items).toEqual([agent]);
    expect(form.open.value).toBe(false);
    expect([form.name.value, form.slug.value, form.description.value]).toEqual(['', '', '']);
    expect(form.busy.value).toBe(false);
  });

  it('keeps the form open when the response has no agent', async () => {
    createAgent.mockResolvedValue({});
    const form = useAgentCreate('V3StGXR8Z5jdHj6BmyT2a', 'x6MH9WPWjktMwCBKHy4pF');
    form.show();
    form.name.value = 'Bot';
    await form.submit();
    expect(form.open.value).toBe(true);
    expect(form.name.value).toBe('Bot');
    expect(useAgentsStore().items).toEqual([]);
  });

  it('does not submit a blank name', async () => {
    const form = useAgentCreate('V3StGXR8Z5jdHj6BmyT2a', 'x6MH9WPWjktMwCBKHy4pF');
    form.name.value = '   ';
    await form.submit();
    expect(createAgent).not.toHaveBeenCalled();
  });

  it('does not submit twice while busy, and cannot be hidden while busy', async () => {
    const pending = deferred<{ agent: typeof agent }>();
    createAgent.mockReturnValue(pending.promise);
    const form = useAgentCreate('V3StGXR8Z5jdHj6BmyT2a', 'x6MH9WPWjktMwCBKHy4pF');
    form.show();
    form.name.value = 'Bot';
    const first = form.submit();
    expect(form.busy.value).toBe(true);
    await form.submit();
    form.hide();
    expect(form.open.value).toBe(true);
    pending.resolve({ agent });
    await first;
    expect(createAgent).toHaveBeenCalledTimes(1);
  });

  it('shows a Connect error with its reference, cleared by hide and show', async () => {
    setKnownKeys('apperror.AGT001');
    createAgent.mockRejectedValue(connectError('AGT001', { slug: 'bot' }));
    const form = useAgentCreate('V3StGXR8Z5jdHj6BmyT2a', 'x6MH9WPWjktMwCBKHy4pF');
    form.show();
    form.name.value = 'Bot';
    await form.submit();
    expect(form.error.value).toEqual({ message: 'apperror.AGT001|{"slug":"bot"}', reference: 'error.ref|{"id":"req-1"}' });
    expect(form.open.value).toBe(true);
    form.hide();
    expect(form.open.value).toBe(false);
    expect(form.error.value).toBeNull();
  });

  it('shows the fallback for a plain error', async () => {
    createAgent.mockRejectedValue(new Error('boom'));
    const form = useAgentCreate('V3StGXR8Z5jdHj6BmyT2a', 'x6MH9WPWjktMwCBKHy4pF');
    form.name.value = 'Bot';
    await form.submit();
    expect(form.error.value).toEqual({ message: 'error.create_agent' });
    form.show();
    expect(form.error.value).toBeNull();
  });

  it('shows a too-long description under its input and does not send it', async () => {
    const form = useAgentCreate('V3StGXR8Z5jdHj6BmyT2a', 'x6MH9WPWjktMwCBKHy4pF');
    form.show();
    form.name.value = 'Bot';
    form.description.value = 'd'.repeat(2001);
    await form.submit();
    expect(createAgent).not.toHaveBeenCalled();
    expect(form.fieldErrors.value).toEqual({ description: 'validation.too_long|{"max":"2000"}' });
  });

  it('shows a server refusal of the description under its input', async () => {
    setKnownKeys('apperror.AGT005');
    createAgent.mockRejectedValueOnce(connectError('AGT005', { max: '2000' }));
    const form = useAgentCreate('V3StGXR8Z5jdHj6BmyT2a', 'x6MH9WPWjktMwCBKHy4pF');
    form.show();
    form.name.value = 'Bot';
    await form.submit();
    expect(form.fieldErrors.value).toEqual({ description: 'apperror.AGT005|{"max":"2000"}' });
    expect(form.error.value).toBeNull();
  });
});
