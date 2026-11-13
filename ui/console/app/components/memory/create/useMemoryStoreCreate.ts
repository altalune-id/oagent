import type { ShownError } from '#base/lib/apperror/message';
import type { MemoryStore } from '#gen/memory/v1/memory_pb';
import { create } from '@bufbuild/protobuf';
import { useValidation } from '#base/composables/useValidation';
import { CreateMemoryStoreRequestSchema } from '#gen/memory/v1/memory_pb';
import { useMemoryStoresStore } from '~/stores/memoryStores';

// useMemoryStoreCreate is the create dialog's state; the server picks the only enabled backend.
export function useMemoryStoreCreate(orgId: string, projectId: string) {
  const open = ref(false);
  const name = ref('');
  const rememberApiUsers = ref(false);
  const busy = ref(false);
  const error = ref<ShownError | null>(null);
  const stores = useMemoryStoresStore();
  const validation = useValidation(CreateMemoryStoreRequestSchema, { fields: ['name'], codes: { MEM003: 'name', MEM005: 'name' } });
  const { $memoryClient: client } = useNuxtApp();

  function show() {
    open.value = true;
  }

  function hide() {
    if (busy.value) {
      return;
    }
    open.value = false;
    error.value = null;
    validation.clear();
  }

  async function submit(): Promise<MemoryStore | null> {
    if (busy.value) {
      return null;
    }
    const req = { orgId, projectId, name: name.value.trim(), rememberApiUsers: rememberApiUsers.value };
    if (!validation.check(create(CreateMemoryStoreRequestSchema, req))) {
      error.value = validation.formError.value;
      return null;
    }
    busy.value = true;
    error.value = null;
    try {
      const res = await client.createMemoryStore(req);
      stores.add(res.store!);
      open.value = false;
      name.value = '';
      rememberApiUsers.value = false;
      return res.store!;
    } catch (e) {
      error.value = validation.fromServer(e, 'error.create_memory_store');
      return null;
    } finally {
      busy.value = false;
    }
  }

  return { open, name, rememberApiUsers, busy, error, fieldErrors: validation.errors, show, hide, submit };
}
