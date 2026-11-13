import { create } from '@bufbuild/protobuf';
import { flushPromises } from '@vue/test-utils';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it } from 'vitest';
import { fakeRuntimeClients, serverPBs } from '~~/test/fixtures/runtime';
import { useToast } from '#base/components/ui/toast';
import { McpServerAuthKind, McpServerSchema } from '#gen/mcpserver/v1/mcpserver_pb';
import { connectError, deferred, setKnownKeys } from '#test/nuxt-globals';
import { useMcpServersStore } from '~/stores/mcpServers';
import { useMcpServerForm } from './useMcpServerForm';

const orgId = 'V3StGXR8Z5jdHj6BmyT2a';
const projectId = 'x6MH9WPWjktMwCBKHy4pF';
const localPB = create(McpServerSchema, { id: 'MsYasakuQcaz23456789a', orgId, projectId, name: 'local-yasaku', url: 'http://127.0.0.1:5190/mcp', authKind: McpServerAuthKind.BEARER, authSet: true, timeoutMs: 10_000 });

describe('useMcpServerForm', () => {
  let clients: ReturnType<typeof fakeRuntimeClients>;

  beforeEach(async () => {
    setActivePinia(createPinia());
    clients = fakeRuntimeClients();
    await useMcpServersStore().load(projectId);
  });

  it('adds a server with a trimmed secret and the server\'s default timeout, then lists its tools', async () => {
    clients.server.createMcpServer.mockResolvedValue({ mcpServer: localPB });
    const form = useMcpServerForm(orgId, projectId);
    form.show();
    expect(form.timeout.value, 'a new server starts empty: the server\'s default applies').toBe('');
    expect(form.secretRequired.value).toBe(true);
    expect(await form.submit()).toBe(false);
    expect(form.errors.value).toMatchObject({ name: 'validation.required', secret: 'mcpservers.secret_required' });
    form.name.value = 'local-yasaku';
    form.url.value = 'http://127.0.0.1:5190/mcp';
    form.secret.value = '  tok-1  ';
    expect(await form.submit()).toBe(true);
    expect(clients.server.createMcpServer.mock.calls[0]![0]).toMatchObject({ orgId, projectId, name: 'local-yasaku', url: 'http://127.0.0.1:5190/mcp', authKind: McpServerAuthKind.BEARER, authValue: 'tok-1', timeoutMs: 0 });
    expect(form.secret.value).toBe('');
    expect(form.open.value).toBe(false);
    expect(useToast().toasts.at(-1)?.text).toBe('mcpservers.saved|{"name":"local-yasaku"}');
    await flushPromises();
    expect(clients.server.listMcpServerTools).toHaveBeenCalledWith({ id: localPB.id });
  });

  it('keeps no timeout bounds when the server sends no defaults, or the list fails', async () => {
    setActivePinia(createPinia());
    clients = fakeRuntimeClients();
    clients.server.listMcpServers.mockResolvedValue({ mcpServers: serverPBs() });
    await useMcpServersStore().load(projectId);
    expect(useMcpServerForm(orgId, projectId).defaults.value).toBeNull();
    setActivePinia(createPinia());
    clients = fakeRuntimeClients();
    clients.server.listMcpServers.mockRejectedValue(new Error('offline'));
    await useMcpServersStore().load(projectId);
    const form = useMcpServerForm(orgId, projectId);
    expect(form.defaults.value).toBeNull();
    form.show();
    expect(form.timeout.value).toBe('');
  });

  it('checks the header name, the timeout and the URL shape before sending', async () => {
    const form = useMcpServerForm(orgId, projectId);
    form.show();
    form.name.value = 'ok';
    form.url.value = 'not a url';
    form.auth.value = 'header';
    form.headerName.value = 'bad header';
    form.secret.value = 's';
    form.timeout.value = 25;
    expect(await form.submit()).toBe(false);
    expect(form.errors.value).toMatchObject({ headerName: 'mcpservers.header_required|{"name":"bad header"}', timeout: 'validation.invalid', url: 'validation.absolute_url' });
    form.name.value = 'Not_Valid';
    expect(await form.submit()).toBe(false);
    expect(form.errors.value.name).toBe('validation.invalid_format');
    form.auth.value = 'none';
    expect(form.needsSecret.value).toBe(false);
    expect(form.secretShown.value).toBe(false);
    expect(clients.server.createMcpServer).not.toHaveBeenCalled();
  });

  it('keeps the name fixed and the secret when editing, and needs it again for a new URL or auth kind', async () => {
    const servers = useMcpServersStore();
    const yasaku = servers.items[0]!;
    clients.server.updateMcpServer.mockImplementation(async req => ({ mcpServer: create(McpServerSchema, { ...serverPBs()[0]!, url: req.url, trusted: req.trusted }) }));
    const form = useMcpServerForm(orgId, projectId);
    form.show(yasaku);
    expect(form.timeout.value).toBe(20);
    form.timeout.value = '';
    expect(form.secretShown.value).toBe(false);
    form.name.value = 'renamed';
    form.trusted.value = false;
    expect(await form.submit()).toBe(true);
    const sent = clients.server.updateMcpServer.mock.calls[0]![0];
    expect(sent.timeoutMs, 'an empty timeout on update keeps the stored one').toBe(0);
    expect(sent.authValue).toBeUndefined();
    expect('name' in sent).toBe(false);
    expect(servers.items[0]).toMatchObject({ name: 'yasaku', secretSet: true, trusted: false });
    form.show(servers.items[0]!);
    form.auth.value = 'header';
    expect(form.secretRequired.value).toBe(true);
    form.auth.value = 'bearer';
    form.url.value = 'https://moved.example.id/mcp';
    expect(form.urlChanged.value).toBe(true);
    expect(form.secretRequired.value).toBe(true);
    expect(await form.submit()).toBe(false);
    expect(form.errors.value).toEqual({ secret: 'mcpservers.secret_required' });
  });

  it('places MCS006 by reason: a bad header name under the header, a missing secret under the secret', async () => {
    setKnownKeys('apperror.MCS006_header_name');
    clients.server.updateMcpServer.mockRejectedValue(connectError('MCS006', { reason: 'header_name' }));
    const form = useMcpServerForm(orgId, projectId);
    form.show(useMcpServersStore().items[1]!);
    expect(await form.submit()).toBe(false);
    expect(form.errors.value).toEqual({ headerName: 'apperror.MCS006_header_name' });
    form.headerName.value = 'Host';
    expect(await form.submit()).toBe(false);
    expect(form.errors.value).toEqual({ headerName: 'mcpservers.header_reserved|{"name":"Host"}' });
  });

  it('shows MCS006 under the secret and the name taken (MCS002) under the name', async () => {
    setKnownKeys('apperror.MCS006_secret_required', 'apperror.MCS002');
    clients.server.updateMcpServer.mockRejectedValue(connectError('MCS006', { reason: 'secret_required' }));
    clients.server.createMcpServer.mockRejectedValue(connectError('MCS002', { name: 'yasaku' }));
    const form = useMcpServerForm(orgId, projectId);
    form.show(useMcpServersStore().items[0]!);
    expect(await form.submit()).toBe(false);
    expect(form.errors.value.secret).toBe('apperror.MCS006_secret_required');
    form.show();
    form.name.value = 'yasaku';
    form.url.value = 'https://y.test/mcp';
    form.auth.value = 'none';
    expect(await form.submit()).toBe(false);
    expect(form.errors.value.name).toBe('apperror.MCS002|{"name":"yasaku"}');
    expect(form.formError.value).toBeNull();
  });

  it('ignores a second submit and stays open while saving', async () => {
    const pending = deferred<{ mcpServer: typeof localPB }>();
    clients.server.createMcpServer.mockReturnValue(pending.promise);
    const form = useMcpServerForm(orgId, projectId);
    form.show();
    form.name.value = 'local-yasaku';
    form.url.value = 'http://127.0.0.1:5190/mcp';
    form.auth.value = 'none';
    const saving = form.submit();
    expect(await form.submit()).toBe(false);
    form.hide();
    expect(form.open.value).toBe(true);
    pending.resolve({ mcpServer: localPB });
    expect(await saving).toBe(true);
    form.show();
    form.secret.value = 'typed';
    form.hide();
    expect(form.secret.value).toBe('');
  });
});
