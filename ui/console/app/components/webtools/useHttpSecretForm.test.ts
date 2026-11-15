import type { HttpSecret } from '~/stores/webTools';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { useToast } from '#base/components/ui/toast';
import { ErrorDetailSchema, ViolationSchema } from '#gen/apperror/v1/error_pb';
import { connectError, setClient, setKnownKeys } from '#test/nuxt-globals';
import { useWebToolsStore } from '~/stores/webTools';
import { useHttpSecretForm } from './useHttpSecretForm';

const projectId = 'x6MH9WPWjktMwCBKHy4pF';
const saved: HttpSecret = { id: 'UxVy4UK3vmrHzPZ273hCA', host: 'api.example.com', header: 'x-api-key' };

describe('useHttpSecretForm', () => {
  let setHTTPSecret: ReturnType<typeof vi.fn>;
  let deleteHTTPSecret: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    setActivePinia(createPinia());
    setHTTPSecret = vi.fn(async (req: { host: string, header: string }) => ({ secret: { id: saved.id, host: req.host, header: req.header.toLowerCase() } }));
    deleteHTTPSecret = vi.fn().mockResolvedValue({});
    setClient('$webToolClient', { setHTTPSecret, deleteHTTPSecret });
  });

  function adding() {
    const form = useHttpSecretForm(projectId);
    form.showAdd();
    form.host.value = ' API.Example.com ';
    form.header.value = ' X-Api-Key ';
    form.value.value = 's3cret';
    return form;
  }

  it('adds a secret with a trimmed, lowercase host', async () => {
    const form = adding();
    expect(form.open.value).toBe(true);
    expect(await form.submit()).toBe(true);
    expect(setHTTPSecret).toHaveBeenCalledWith({ projectId, host: 'api.example.com', header: 'X-Api-Key', value: 's3cret' });
    expect(useWebToolsStore().secrets).toEqual([{ ...saved, updatedAt: undefined }]);
    expect(useToast().toasts.at(-1)?.text).toBe('webtools.secret_saved');
    expect(form.open.value).toBe(false);
    expect(form.value.value).toBe('');
  });

  it('refuses adding a host and header that already have a secret, and still replaces it', async () => {
    useWebToolsStore().secrets = [saved];
    const form = adding();
    form.host.value = 'API.example.com';
    form.header.value = 'X-API-KEY';
    expect(await form.submit()).toBe(false);
    expect(form.errors.value).toEqual({ header: 'webtools.secret_exists' });
    expect(setHTTPSecret).not.toHaveBeenCalled();
    form.header.value = 'authorization';
    expect(await form.submit()).toBe(true);
    useWebToolsStore().secrets = [saved];
    form.showReplace(saved);
    form.value.value = 'new';
    expect(await form.submit()).toBe(true);
    expect(setHTTPSecret).toHaveBeenLastCalledWith({ projectId, host: 'api.example.com', header: 'x-api-key', value: 'new' });
  });

  it('keeps only the host name of a pasted URL', async () => {
    const form = adding();
    form.host.value = 'https://API.example.com:8443/v1/orders';
    expect(await form.submit()).toBe(true);
    expect(setHTTPSecret).toHaveBeenCalledWith({ projectId, host: 'api.example.com', header: 'X-Api-Key', value: 's3cret' });
  });

  it('refuses empty inputs before sending', async () => {
    const form = useHttpSecretForm(projectId);
    form.showAdd();
    expect(await form.submit()).toBe(false);
    expect(Object.keys(form.errors.value).sort()).toEqual(['header', 'host', 'value']);
    expect(setHTTPSecret).not.toHaveBeenCalled();
  });

  it('replaces only the value of a saved secret', async () => {
    const form = useHttpSecretForm(projectId);
    form.showReplace(saved);
    expect([form.editing.value, form.host.value, form.header.value, form.value.value]).toEqual([saved, 'api.example.com', 'x-api-key', '']);
    form.host.value = 'other.example.com';
    form.value.value = 'new';
    expect(await form.submit()).toBe(true);
    expect(setHTTPSecret).toHaveBeenCalledWith({ projectId, host: 'api.example.com', header: 'x-api-key', value: 'new' });
  });

  it('places field refusals under their inputs and shows the others on the form', async () => {
    setKnownKeys('apperror.WEB005', 'apperror.WEB003');
    const domain = new ConnectError('bad', Code.InvalidArgument, undefined, [{
      desc: ErrorDetailSchema,
      value: create(ErrorDetailSchema, { code: 'GEN004', meta: { field: 'header', reason: 'invalid' }, violations: [create(ViolationSchema, { field: 'header', reason: 'invalid' })] }),
    }]);
    setHTTPSecret
      .mockRejectedValueOnce(domain)
      .mockRejectedValueOnce(connectError('WEB005', { max: '50' }))
      .mockRejectedValueOnce(connectError('WEB003'));
    const form = adding();
    expect(await form.submit()).toBe(false);
    expect(Object.keys(form.errors.value)).toEqual(['header']);
    expect(form.formError.value).toBeNull();
    expect(await form.submit()).toBe(false);
    expect(form.formError.value?.message).toBe('apperror.WEB005|{"max":"50"}');
    expect(await form.submit()).toBe(false);
    expect(form.formError.value?.message).toBe('apperror.WEB003');
    expect(form.open.value).toBe(true);
  });

  it('deletes a secret through a confirm', async () => {
    const store = useWebToolsStore();
    store.secrets = [saved];
    const form = useHttpSecretForm(projectId);
    form.deleting.ask(saved);
    expect(await form.deleting.confirm()).toBe(true);
    expect(deleteHTTPSecret).toHaveBeenCalledWith({ id: saved.id });
    expect(store.secrets).toEqual([]);
    expect(useToast().toasts.at(-1)?.text).toBe('webtools.secret_deleted');
  });

  it('treats a secret already gone as deleted', async () => {
    deleteHTTPSecret.mockRejectedValueOnce(connectError('WEB004'));
    const store = useWebToolsStore();
    store.secrets = [saved];
    const form = useHttpSecretForm(projectId);
    form.deleting.ask(saved);
    expect(await form.deleting.confirm()).toBe(true);
    expect(store.secrets).toEqual([]);
    expect(useToast().toasts.at(-1)?.text).toBe('webtools.secret_deleted');
  });

  it('keeps the confirm open on another refusal', async () => {
    deleteHTTPSecret.mockRejectedValueOnce(new Error('boom'));
    useWebToolsStore().secrets = [saved];
    const form = useHttpSecretForm(projectId);
    form.deleting.ask(saved);
    expect(await form.deleting.confirm()).toBe(false);
    expect(form.deleting.error.value).toEqual({ message: 'error.delete_http_secret' });
  });

  it('closes and clears on hide', () => {
    const form = adding();
    form.hide();
    expect([form.open.value, form.host.value, form.header.value, form.value.value]).toEqual([false, '', '', '']);
  });
});
