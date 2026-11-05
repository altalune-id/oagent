import type { McpServer, ServerInput } from './mcpServers';
import { create } from '@bufbuild/protobuf';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { errorCode } from '#base/lib/apperror/message';
import { McpServerAuthKind, McpServerSchema, McpServerToolSchema } from '#gen/mcpserver/v1/mcpserver_pb';
import { connectError, deferred, setClient, setKnownKeys } from '#test/nuxt-globals';
import { createRequest, secretRefusedCode, toMcpServer, toMcpTool, updateRequest, useMcpServersStore } from './mcpServers';

const orgId = 'V3StGXR8Z5jdHj6BmyT2a';
const projectId = 'x6MH9WPWjktMwCBKHy4pF';
const serverPB = create(McpServerSchema, { id: 'UxVy4UK3vmrHzPZ273hCA', orgId, projectId, name: 'yasaku', url: 'https://yasaku.test/mcp', authKind: McpServerAuthKind.BEARER, authSet: true, timeoutMs: 10_000, trusted: true });
const server: McpServer = { id: 'UxVy4UK3vmrHzPZ273hCA', orgId, projectId, name: 'yasaku', url: 'https://yasaku.test/mcp', auth: 'bearer', headerName: '', secretSet: true, trusted: true, timeoutMs: 10_000, tools: [] };
const input: ServerInput = { name: ' yasaku ', url: ' https://yasaku.test/mcp ', auth: 'bearer', headerName: 'X-Ignored', secret: '  tok-1  ', trusted: true, timeoutMs: 10_000 };
const toolsPB = [
  create(McpServerToolSchema, { name: 'record_expense', description: 'Record an expense', readOnly: false, grantable: true, requireApproval: true, inputSchema: { type: 'object', properties: { confirm: { type: 'boolean' }, amount: { type: 'integer' }, note: { type: 'string' }, odd: 1 } } }),
  create(McpServerToolSchema, { name: 'list_wallets', description: 'List wallets', readOnly: true, grantable: true, requireApproval: false }),
  create(McpServerToolSchema, { name: 'export_all_transactions_as_spreadsheet_for_accountant_review', readOnly: true, grantable: false, requireApproval: false, inputSchema: { properties: [] } }),
];

describe('mcp server mapping', () => {
  it('copies the fields it needs and never a credential', () => {
    expect(toMcpServer(serverPB)).toEqual(server);
    expect(toMcpServer(create(McpServerSchema, { ...serverPB, authKind: McpServerAuthKind.HEADER, authHeader: 'X-Api-Key' }))).toMatchObject({ auth: 'header', headerName: 'X-Api-Key' });
    expect(toMcpServer(create(McpServerSchema, { ...serverPB, authKind: McpServerAuthKind.NONE })).auth).toBe('none');
  });

  it('reads the suggested approval, grantability and argument types of a tool', () => {
    expect(toolsPB.map(toMcpTool)).toEqual([
      { name: 'record_expense', description: 'Record an expense', hint: 'changes', args: [{ name: 'confirm', type: 'boolean' }, { name: 'amount', type: 'number' }, { name: 'note', type: 'string' }, { name: 'odd', type: 'string' }], grantable: true, requireApproval: true },
      { name: 'list_wallets', description: 'List wallets', hint: 'read_only', args: [], grantable: true, requireApproval: false },
      { name: 'export_all_transactions_as_spreadsheet_for_accountant_review', description: '', hint: 'read_only', args: [], grantable: false, requireApproval: false },
    ]);
  });

  it('trims the request, sends a header name only for header auth and a secret only when typed', () => {
    expect(createRequest(orgId, projectId, input)).toMatchObject({ orgId, projectId, name: 'yasaku', url: 'https://yasaku.test/mcp', authKind: McpServerAuthKind.BEARER, authHeader: '', authValue: 'tok-1', timeoutMs: 10_000, trusted: true });
    expect(createRequest(orgId, projectId, { ...input, auth: 'none' }).authValue).toBe('');
    expect(createRequest(orgId, projectId, { ...input, auth: 'header', headerName: ' X-Api-Key ' }).authHeader).toBe('X-Api-Key');
    const update = updateRequest(server.id, { ...input, secret: ' ' });
    expect(update).toMatchObject({ id: server.id, url: 'https://yasaku.test/mcp' });
    expect(update.authValue).toBeUndefined();
    expect('name' in update).toBe(false);
    expect(updateRequest(server.id, { ...input, auth: 'none' }).authValue).toBeUndefined();
    expect(updateRequest(server.id, input).authValue).toBe('tok-1');
  });
});

describe('useMcpServersStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('lists the servers of a project', async () => {
    const listMcpServers = vi.fn().mockResolvedValue({ mcpServers: [serverPB] });
    setClient('$mcpServerClient', { listMcpServers });
    const store = useMcpServersStore();
    await store.load(projectId);
    expect(listMcpServers).toHaveBeenCalledWith({ projectId });
    expect(store.items).toEqual([server]);
  });

  it('treats a missing list as empty and shows a load error', async () => {
    setClient('$mcpServerClient', { listMcpServers: vi.fn().mockResolvedValue({}) });
    const store = useMcpServersStore();
    await store.load(projectId);
    expect(store.items).toEqual([]);
    setKnownKeys('apperror.GEN900');
    setClient('$mcpServerClient', { listMcpServers: vi.fn().mockRejectedValue(connectError('GEN900')) });
    setActivePinia(createPinia());
    const failing = useMcpServersStore();
    await failing.load(projectId);
    expect(failing.error).toEqual({ message: 'apperror.GEN900', reference: 'error.ref|{"id":"req-1"}' });
  });

  it('shares an in-flight load, skips a loaded project and drops stale answers', async () => {
    const first = deferred<{ mcpServers: typeof serverPB[] }>();
    const second = deferred<never>();
    const listMcpServers = vi.fn().mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise).mockResolvedValue({ mcpServers: [] });
    setClient('$mcpServerClient', { listMcpServers });
    const store = useMcpServersStore();
    await store.ensureLoaded('');
    const a = store.ensureLoaded(projectId);
    const b = store.ensureLoaded(projectId);
    expect(listMcpServers).toHaveBeenCalledTimes(1);
    const stale = store.load(projectId);
    await store.load(projectId);
    first.resolve({ mcpServers: [serverPB] });
    second.reject(new Error('boom'));
    await Promise.all([a, b, stale]);
    expect(store.items).toEqual([]);
    expect(store.error).toBeNull();
    await store.ensureLoaded(projectId);
    expect(listMcpServers).toHaveBeenCalledTimes(3);
  });

  it('lists tools with suggested approval and the ones that cannot be granted, once', async () => {
    const listMcpServerTools = vi.fn().mockResolvedValue({ tools: toolsPB });
    setClient('$mcpServerClient', { listMcpServerTools });
    const store = useMcpServersStore();
    store.items = [server];
    await Promise.all([store.ensureTools(server.id), store.ensureTools(server.id)]);
    await store.ensureTools(server.id);
    expect(listMcpServerTools).toHaveBeenCalledTimes(1);
    expect(listMcpServerTools).toHaveBeenCalledWith({ id: server.id });
    expect(store.items[0]!.tools.map(t => [t.name, t.requireApproval, t.grantable])).toEqual([['record_expense', true, true], ['list_wallets', false, true], ['export_all_transactions_as_spreadsheet_for_accountant_review', false, false]]);
    expect(store.toolsLoading[server.id]).toBe(false);
  });

  it('marks a server that refused the credentials, apart from an unreachable one', async () => {
    setKnownKeys('apperror.MCS005');
    const listMcpServerTools = vi.fn().mockRejectedValueOnce(connectError(secretRefusedCode)).mockRejectedValueOnce(connectError('MCS005'));
    setClient('$mcpServerClient', { listMcpServerTools });
    const store = useMcpServersStore();
    await store.loadTools(server.id);
    expect(store.toolsRefused[server.id]).toBe(true);
    await store.loadTools(server.id);
    expect(store.toolsRefused[server.id]).toBe(false);
  });

  it('shows an unreachable server (MCS005) and lets it be tried again', async () => {
    setKnownKeys('apperror.MCS005');
    const listMcpServerTools = vi.fn().mockRejectedValueOnce(connectError('MCS005')).mockResolvedValue({});
    setClient('$mcpServerClient', { listMcpServerTools });
    const store = useMcpServersStore();
    await store.loadTools('GneZ23456789abcdefghj');
    expect(store.toolsError.GneZ23456789abcdefghj).toEqual({ message: 'apperror.MCS005', reference: 'error.ref|{"id":"req-1"}' });
    await store.ensureTools('GneZ23456789abcdefghj');
    expect(listMcpServerTools).toHaveBeenCalledTimes(2);
    expect(store.toolsError.GneZ23456789abcdefghj).toBeNull();
  });

  it('creates a server with the trimmed secret and keeps no secret in state', async () => {
    const createMcpServer = vi.fn().mockResolvedValue({ mcpServer: serverPB });
    setClient('$mcpServerClient', { createMcpServer });
    const store = useMcpServersStore();
    expect(await store.add(orgId, projectId, input)).toEqual(server);
    expect(createMcpServer.mock.calls[0]![0].authValue).toBe('tok-1');
    expect(JSON.stringify(store.$state)).not.toContain('tok-1');
  });

  it('updates without a secret, keeps auth_set and lists tools again', async () => {
    const updateMcpServer = vi.fn().mockResolvedValue({ mcpServer: create(McpServerSchema, { ...serverPB, trusted: false }) });
    const listMcpServerTools = vi.fn().mockResolvedValue({ tools: toolsPB });
    setClient('$mcpServerClient', { updateMcpServer, listMcpServerTools });
    const store = useMcpServersStore();
    store.items = [server];
    await store.ensureTools(server.id);
    const saved = await store.update(server.id, { ...input, secret: '', trusted: false });
    expect(updateMcpServer.mock.calls[0]![0].authValue).toBeUndefined();
    expect(saved).toMatchObject({ secretSet: true, trusted: false, tools: [] });
    await store.ensureTools(server.id);
    expect(listMcpServerTools).toHaveBeenCalledTimes(2);
  });

  it('passes on MCS006 when the URL changes without a secret, keeping the old row', async () => {
    setClient('$mcpServerClient', { updateMcpServer: vi.fn().mockRejectedValue(connectError('MCS006', { reason: 'secret_required' })) });
    const store = useMcpServersStore();
    store.items = [server];
    const err = await store.update(server.id, { ...input, url: 'https://moved.test/mcp', secret: '' }).catch(e => e);
    expect(errorCode(err)).toBe('MCS006');
    expect(store.items).toEqual([server]);
  });

  it('deletes a server, and keeps it when it is in use (MCS004)', async () => {
    const deleteMcpServer = vi.fn().mockRejectedValueOnce(connectError('MCS004')).mockResolvedValue({});
    setClient('$mcpServerClient', { deleteMcpServer });
    const store = useMcpServersStore();
    store.items = [server];
    expect(errorCode(await store.remove(server.id).catch(e => e))).toBe('MCS004');
    expect(store.items).toEqual([server]);
    await store.remove(server.id);
    expect(deleteMcpServer).toHaveBeenLastCalledWith({ id: server.id });
    expect(store.items).toEqual([]);
  });
});
