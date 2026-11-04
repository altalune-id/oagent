import type { ProbeResult } from '~/stores/mcpServers';
import { createPinia, setActivePinia } from 'pinia';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { nextTick } from 'vue';
import { sampleProbe } from '~/preview/samples';
import { useMcpServersStore } from '~/stores/mcpServers';
import { useMcpServerForm } from './useMcpServerForm';

describe('useMcpServerForm', () => {
  beforeEach(async () => {
    setActivePinia(createPinia());
    await useMcpServersStore().load('x6MH9WPWjktMwCBKHy4pF');
  });

  it('creates a server after loading its tools', async () => {
    const probe = vi.fn().mockResolvedValue({ kind: 'ok', tools: [{ name: 'a', description: '', hint: 'read_only', args: [] }] });
    const form = useMcpServerForm(probe);
    form.show();
    expect(form.submit()).toBe(false);
    expect(form.errors.value).toEqual({ name: 'mcpservers.name_required', url: 'mcpservers.url_https', secret: 'mcpservers.secret_required' });
    form.name.value = 'kas';
    form.url.value = 'https://kas.example.test/mcp';
    form.auth.value = 'header';
    form.headerName.value = 'X-Api-Key';
    form.secret.value = 's';
    form.timeout.value = 0;
    expect(form.submit()).toBe(false);
    expect(form.errors.value).toEqual({ timeout: 'mcpservers.timeout_range' });
    form.timeout.value = 30;
    await form.loadTools();
    expect(probe).toHaveBeenCalledWith('https://kas.example.test/mcp');
    expect(form.submit()).toBe(true);
    expect(useMcpServersStore().items.at(-1)).toMatchObject({ name: 'kas', auth: 'header', headerName: 'X-Api-Key', secretSet: true, timeoutSeconds: 30, tools: [{ name: 'a' }] });
    expect(form.secret.value).toBe('');
  });

  it('drops the loaded tools when the URL changes, so a save never keeps tools from another host', async () => {
    const probe = vi.fn().mockResolvedValue({ kind: 'ok', tools: [{ name: 'a', description: '', hint: 'read_only', args: [] }] });
    const form = useMcpServerForm(probe);
    form.show();
    form.name.value = 'kas';
    form.url.value = 'https://kas.example.test/mcp';
    form.auth.value = 'none';
    await form.loadTools();
    expect(form.probed.value?.kind).toBe('ok');
    form.url.value = 'https://other.example.test/mcp';
    expect(form.probed.value).toBeNull();
    expect(form.submit()).toBe(true);
    expect(useMcpServersStore().items.at(-1)!.tools).toEqual([]);
  });

  it('drops the seeded tools of an edited server when its URL changes', () => {
    const servers = useMcpServersStore();
    const form = useMcpServerForm(vi.fn());
    form.show(servers.items[0]);
    form.url.value = 'https://moved.example.test/mcp';
    expect(form.urlChanged.value).toBe(true);
    form.secret.value = 's';
    expect(form.submit()).toBe(true);
    expect(servers.items[0]!.tools).toEqual([]);
  });

  it('cancels a running probe when the URL changes', async () => {
    let answer: (r: ProbeResult) => void = () => {};
    const form = useMcpServerForm(() => new Promise((resolve) => {
      answer = resolve;
    }));
    form.show();
    form.url.value = 'https://x.test/mcp';
    const run = form.loadTools();
    form.url.value = 'https://y.test/mcp';
    await nextTick();
    expect(form.probing.value).toBe(false);
    answer({ kind: 'empty' });
    await run;
    expect(form.probed.value).toBeNull();
  });

  it('refuses a taken name, a bad header and a non-https URL before probing', async () => {
    const probe = vi.fn();
    const form = useMcpServerForm(probe);
    form.show();
    form.name.value = 'YaSaku';
    form.url.value = 'http://x.test';
    form.auth.value = 'header';
    form.secret.value = 's';
    await form.loadTools();
    expect(probe).not.toHaveBeenCalled();
    expect(form.errors.value.url).toBe('mcpservers.url_https');
    form.submit();
    expect(form.errors.value).toMatchObject({ name: 'mcpservers.name_taken', headerName: 'mcpservers.header_required' });
  });

  it('keeps the name and secret when editing, and asks for the secret again when the URL or auth changes', () => {
    const servers = useMcpServersStore();
    const yasaku = servers.items[0]!;
    const form = useMcpServerForm(vi.fn());
    form.show(yasaku);
    expect(form.probed.value).toEqual({ kind: 'ok', tools: yasaku.tools });
    expect(form.secretShown.value).toBe(false);
    form.name.value = 'renamed';
    form.trusted.value = false;
    expect(form.submit()).toBe(true);
    expect(servers.items[0]).toMatchObject({ name: 'yasaku', trusted: false, tools: yasaku.tools });
    form.show(servers.items[0]);
    form.url.value = 'https://new.test/mcp';
    expect(form.secretRequired.value).toBe(true);
    form.url.value = yasaku.url;
    form.auth.value = 'none';
    expect(form.needsSecret.value).toBe(false);
    expect(form.submit()).toBe(true);
    expect(servers.items[0]).toMatchObject({ auth: 'none', secretSet: false });
  });

  it('drops a probe answer that arrives after the form closed', async () => {
    let answer: (r: ProbeResult) => void = () => {};
    const form = useMcpServerForm(() => new Promise((resolve) => {
      answer = resolve;
    }));
    form.show();
    form.url.value = 'https://x.test/mcp';
    const run = form.loadTools();
    expect(form.probing.value).toBe(true);
    form.secret.value = 'typed';
    form.hide();
    expect(form.secret.value).toBe('');
    answer({ kind: 'empty' });
    await run;
    expect(form.probed.value).toBeNull();
  });
});

describe('sampleProbe', () => {
  it('answers each failure by URL so every state can be clicked', async () => {
    expect(await sampleProbe('https://10.0.0.2/mcp', 0)).toEqual({ kind: 'private' });
    expect(await sampleProbe('https://down.test/mcp', 0)).toEqual({ kind: 'unreachable' });
    expect(await sampleProbe('https://x.test/401', 0)).toEqual({ kind: 'auth' });
    expect(await sampleProbe('https://empty.test/mcp', 0)).toEqual({ kind: 'empty' });
    expect((await sampleProbe('https://yasaku.altalune.id/mcp', 0)).kind).toBe('ok');
  });
});
