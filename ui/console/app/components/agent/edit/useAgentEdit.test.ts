import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { useAgentsStore } from '~/stores/agents';
import { connectError, deferred, setClient, setKnownKeys } from '../../../../test/nuxt-globals';
import { useAgentEdit } from './useAgentEdit';

const agent = { id: 'EW7HBTpNTTPAuNhPZB62M', projectId: 'x6MH9WPWjktMwCBKHy4pF', slug: 'bot', name: 'Bot', description: 'Helps' };

describe('useAgentEdit', () => {
  let updateAgent: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    setActivePinia(createPinia());
    updateAgent = vi.fn().mockResolvedValue({ agent: { ...agent, name: 'Robot', description: 'New' } });
    setClient('$agentClient', { updateAgent });
    useAgentsStore().add(agent);
  });

  it('fills the form from the agent', () => {
    const form = useAgentEdit();
    form.show(agent);
    expect(form.open.value).toBe(true);
    expect(form.name.value).toBe('Bot');
    expect(form.description.value).toBe('Helps');
    expect(form.error.value).toBeNull();
  });

  it('updates the agent in the store and closes', async () => {
    const form = useAgentEdit();
    form.show(agent);
    form.name.value = ' Robot ';
    form.description.value = 'New';
    await form.submit();
    expect(updateAgent).toHaveBeenCalledWith({ id: 'EW7HBTpNTTPAuNhPZB62M', name: 'Robot', description: 'New' });
    expect(useAgentsStore().items[0]).toEqual({ ...agent, name: 'Robot', description: 'New' });
    expect(form.open.value).toBe(false);
    expect(form.name.value).toBe('');
  });

  it('closes without touching the store when the response has no agent', async () => {
    updateAgent.mockResolvedValue({});
    const form = useAgentEdit();
    form.show(agent);
    form.name.value = 'Robot';
    await form.submit();
    expect(useAgentsStore().items[0]).toEqual(agent);
    expect(form.open.value).toBe(false);
  });

  it('does not submit a blank name or while busy', async () => {
    const pending = deferred<object>();
    updateAgent.mockReturnValue(pending.promise);
    const form = useAgentEdit();
    form.show(agent);
    form.name.value = ' ';
    await form.submit();
    expect(updateAgent).not.toHaveBeenCalled();
    form.name.value = 'Robot';
    const first = form.submit();
    await form.submit();
    form.hide();
    expect(form.open.value).toBe(true);
    pending.resolve({});
    await first;
    expect(updateAgent).toHaveBeenCalledTimes(1);
  });

  it('shows a Connect error with its reference, then the fallback for a plain error', async () => {
    setKnownKeys('apperror.GEN900');
    updateAgent.mockRejectedValueOnce(connectError('GEN900')).mockRejectedValueOnce(new Error('boom'));
    const form = useAgentEdit();
    form.show(agent);
    await form.submit();
    expect(form.error.value).toEqual({ message: 'apperror.GEN900', reference: 'error.ref|{"id":"req-1"}' });
    expect(form.open.value).toBe(true);
    await form.submit();
    expect(form.error.value).toEqual({ message: 'error.update_agent' });
    form.hide();
    expect(form.open.value).toBe(false);
    expect(form.error.value).toBeNull();
  });

  it('shows a too-long description under its input and does not send it', async () => {
    const form = useAgentEdit();
    form.show(agent);
    form.description.value = 'd'.repeat(2001);
    await form.submit();
    expect(updateAgent).not.toHaveBeenCalled();
    expect(form.fieldErrors.value).toEqual({ description: 'validation.too_long|{"max":"2000"}' });
  });

  it('shows a server refusal of the description under its input', async () => {
    setKnownKeys('apperror.AGT005');
    updateAgent.mockRejectedValueOnce(connectError('AGT005', { max: '2000' }));
    const form = useAgentEdit();
    form.show(agent);
    await form.submit();
    expect(form.fieldErrors.value).toEqual({ description: 'apperror.AGT005|{"max":"2000"}' });
    expect(form.error.value).toBeNull();
  });
});
