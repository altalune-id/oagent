import { create } from '@bufbuild/protobuf';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { MemoryStoreSchema } from '#gen/memory/v1/memory_pb';
import { connectError, deferred, setClient, setKnownKeys } from '#test/nuxt-globals';
import { useMemoryStoresStore } from '~/stores/memoryStores';
import { useMemoryStoreCreate } from './useMemoryStoreCreate';

const orgId = 'V3StGXR8Z5jdHj6BmyT2a';
const projectId = 'x6MH9WPWjktMwCBKHy4pF';
const created = create(MemoryStoreSchema, { id: 'Ms4tR7wQ2xKp9VbNc3HzL', projectId, name: 'People' });

describe('useMemoryStoreCreate', () => {
  let createMemoryStore: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    setActivePinia(createPinia());
    createMemoryStore = vi.fn().mockResolvedValue({ store: created });
    setClient('$memoryClient', { createMemoryStore });
  });

  it('creates a store on the only enabled backend, adds it, closes and returns it', async () => {
    const form = useMemoryStoreCreate(orgId, projectId);
    form.show();
    expect(form.open.value).toBe(true);
    form.name.value = ' People ';
    const store = await form.submit();
    expect(form.rememberApiUsers.value).toBe(false);
    expect(createMemoryStore).toHaveBeenCalledWith({ orgId, projectId, name: 'People', rememberApiUsers: false });
    expect(store?.id).toBe('Ms4tR7wQ2xKp9VbNc3HzL');
    expect(useMemoryStoresStore().items.map(s => s.name)).toEqual(['People']);
    expect([form.open.value, form.name.value]).toEqual([false, '']);
  });

  it('sends Remember API users when it is turned on, and resets it after create', async () => {
    const form = useMemoryStoreCreate(orgId, projectId);
    form.show();
    form.name.value = 'Integrator';
    form.rememberApiUsers.value = true;
    await form.submit();
    expect(createMemoryStore).toHaveBeenCalledWith({ orgId, projectId, name: 'Integrator', rememberApiUsers: true });
    expect(form.rememberApiUsers.value).toBe(false);
  });

  it('checks the name before sending, and puts a taken name under the field', async () => {
    setKnownKeys('apperror.MEM005');
    const form = useMemoryStoreCreate(orgId, projectId);
    form.show();
    expect(await form.submit()).toBeNull();
    expect(createMemoryStore).not.toHaveBeenCalled();
    expect(form.fieldErrors.value.name).toBeTruthy();
    createMemoryStore.mockRejectedValueOnce(connectError('MEM005', { name: 'People' }));
    form.name.value = 'People';
    expect(await form.submit()).toBeNull();
    expect(form.fieldErrors.value.name).toBe('apperror.MEM005|{"name":"People"}');
    expect(form.open.value).toBe(true);
  });

  it('shows another failure as the form error, sends once while busy, and keeps the dialog while busy', async () => {
    setKnownKeys('error.create_memory_store');
    const slow = deferred<unknown>();
    createMemoryStore.mockReturnValueOnce(slow.promise);
    const form = useMemoryStoreCreate(orgId, projectId);
    form.show();
    form.name.value = 'People';
    const first = form.submit();
    expect(await form.submit()).toBeNull();
    form.hide();
    expect(form.open.value).toBe(true);
    slow.reject(new Error('offline'));
    await first;
    expect(createMemoryStore).toHaveBeenCalledOnce();
    expect(form.error.value?.message).toBe('error.create_memory_store');
    form.hide();
    expect([form.open.value, form.error.value]).toEqual([false, null]);
  });
});
