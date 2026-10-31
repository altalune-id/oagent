import { describe, expect, it, vi } from 'vitest';
import { callTool, errorText } from './bridge';

describe('bridge', () => {
  it('returns structuredContent, or throws the refusal text', async () => {
    const app = { callServerTool: vi.fn().mockResolvedValueOnce({ structuredContent: { ok: 1 } }).mockResolvedValueOnce({ isError: true, content: [{ type: 'text', text: 'PRJ001 not found' }] }).mockResolvedValueOnce({}) };
    await expect(callTool(app, 'project_get', { id: 'p' })).resolves.toEqual({ ok: 1 });
    expect(app.callServerTool).toHaveBeenCalledWith({ name: 'project_get', arguments: { id: 'p' } });
    await expect(callTool(app, 'project_get', {})).rejects.toThrow('PRJ001 not found');
    await expect(callTool(app, 'project_get', {})).resolves.toEqual({});
  });

  it('falls back to a plain message when a refusal has no text', () => {
    expect(errorText({ isError: true })).toBe('The tool call failed.');
  });

  it('shows OAgent\'s error envelope in the console\'s words with its code, never the raw JSON', () => {
    const text = (body: unknown) => errorText({ isError: true, content: [{ type: 'text', text: JSON.stringify(body) }] });
    expect(text({ code: 'ORG001', message: 'org not found', request_id: 'r1' })).toBe('Organization not found (ORG001)');
    expect(text({ code: 'WHK002', message: 'invalid webhook url', meta: { reason: 'https_required' } })).toMatch(/\(WHK002\)$/);
    expect(text({ code: 'WHK002', message: 'invalid webhook url', meta: { reason: 'https_required' } })).not.toContain('{');
    expect(text({ code: 'GEN007', message: 'Forbidden' })).toBe('Forbidden (GEN007)');
    expect(text({ code: 'ZZZ999', message: 'Something odd' })).toBe('Something odd (ZZZ999)');
    expect(text({ code: 'ZZZ999' })).toBe('The tool call failed. (ZZZ999)');
    expect(text({ message: 'no code' })).toBe('{"message":"no code"}');
    expect(errorText({ isError: true, content: [{ type: 'text', text: 'requires scope "member:write"' }] })).toBe('requires scope "member:write"');
  });
});
