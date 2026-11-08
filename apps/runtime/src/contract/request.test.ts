import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

import { CONTRACT_VERSION, validateRequest } from './request.ts';

const goldens = fileURLToPath(new URL('../../../../internal/kernel/runtime/testdata/turn/', import.meta.url));
const requests = readdirSync(goldens).filter(f => f.endsWith('.request.json')).sort();

function golden(name: string): Record<string, unknown> {
  return JSON.parse(readFileSync(join(goldens, name), 'utf8')) as Record<string, unknown>;
}

describe('validateRequest', () => {
  it('finds the shared request goldens', () => {
    expect(requests.length).toBeGreaterThanOrEqual(9);
  });

  it.each(requests)('accepts the golden %s', (name) => {
    const result = validateRequest(golden(name));
    expect(result.ok ? 'ok' : result.fields).toBe('ok');
  });

  it('refuses an unknown top-level field', () => {
    const result = validateRequest({ ...golden('plain_reply.request.json'), extra: true });
    expect(result).toEqual({ ok: false, fields: ['extra'] });
  });

  it('refuses an unknown nested field', () => {
    const req = golden('tool_call.request.json');
    const agent = req.agent as { model: Record<string, unknown> };
    agent.model.proxy = 'http://proxy';
    const result = validateRequest(req);
    expect(result).toEqual({ ok: false, fields: ['agent.model.proxy'] });
  });

  it('accepts contract versions 1 and 2 during the rollout and refuses any other', () => {
    expect(CONTRACT_VERSION).toBe(2);
    expect(validateRequest({ ...golden('plain_reply.request.json'), contractVersion: 1 }).ok).toBe(true);
    expect(validateRequest({ ...golden('plain_reply.request.json'), contractVersion: 2 }).ok).toBe(true);
    expect(validateRequest({ ...golden('plain_reply.request.json'), contractVersion: 3 })).toEqual({ ok: false, fields: ['contractVersion'] });
  });

  it('reports field paths and never the values', () => {
    const req = golden('tool_call.request.json');
    const agent = req.agent as { model: Record<string, unknown> };
    agent.model.apiKey = 42;
    const servers = req.mcpServers as { auth: Record<string, unknown> }[];
    servers[0]!.auth.kind = 'tok-test-kind';
    const result = validateRequest(req);
    expect(result.ok).toBe(false);
    const text = JSON.stringify(result);
    expect(text).toContain('agent.model.apiKey');
    expect(text).toContain('mcpServers.0.auth.kind');
    expect(text).not.toContain('tok-test');
    expect(text).not.toContain('42');
  });

  it('refuses history tool-call arguments that are not a JSON object', () => {
    const req = golden('rebuild_from_history.request.json');
    const history = req.history as { toolCalls?: { function: { arguments: string } }[] }[];
    const i = history.findIndex(m => m.toolCalls);
    history[i]!.toolCalls![0]!.function.arguments = '{not json';
    expect(validateRequest(req)).toEqual({ ok: false, fields: [`history.${i}.toolCalls.0.function.arguments`] });
    history[i]!.toolCalls![0]!.function.arguments = '[1]';
    expect(validateRequest(req).ok).toBe(false);
    history[i]!.toolCalls![0]!.function.arguments = '';
    expect(validateRequest(req).ok).toBe(true);
  });

  it('accepts agentVersion 0 for a draft pin and refuses a negative one', () => {
    const req = golden('plain_reply.request.json');
    (req.ids as { agentVersion: number }).agentVersion = 0;
    expect(validateRequest(req).ok).toBe(true);
    (req.ids as { agentVersion: number }).agentVersion = -1;
    expect(validateRequest(req)).toEqual({ ok: false, fields: ['ids.agentVersion'] });
  });

  it('accepts maxSteps 0 as the runtime default and refuses a negative one', () => {
    const req = golden('plain_reply.request.json');
    (req.agent as { maxSteps: number }).maxSteps = 0;
    expect(validateRequest(req).ok).toBe(true);
    (req.agent as { maxSteps: number }).maxSteps = -1;
    expect(validateRequest(req)).toEqual({ ok: false, fields: ['agent.maxSteps'] });
  });

  it('accepts continue only without a snapshot and right after a tool result', () => {
    const ok = golden('resume_after_crash.request.json');
    expect(validateRequest(ok).ok).toBe(true);
    const withSnapshot = { ...golden('resume_after_crash.request.json'), snapshot: golden('resume.request.json').snapshot };
    expect(validateRequest(withSnapshot)).toEqual({ ok: false, fields: ['input.kind'] });
    const notAfterTool = golden('resume_after_crash.request.json');
    (notAfterTool.history as unknown[]).pop();
    expect(validateRequest(notAfterTool)).toEqual({ ok: false, fields: ['input.kind'] });
    const extra = golden('resume_after_crash.request.json');
    extra.input = { kind: 'continue', message: { id: 'm', role: 'user', content: 'x' } };
    expect(validateRequest(extra)).toEqual({ ok: false, fields: ['input.message'] });
  });

  it('refuses a non-object body', () => {
    expect(validateRequest('hello')).toEqual({ ok: false, fields: [''] });
  });
});
