import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

import { CONTRACT_VERSION, MAX_PASSAGE_CHARS, MAX_PASSAGES, validateRequest } from './request.ts';

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

  it('defaults the model provider to openai_compatible and keeps anthropic', () => {
    const plain = validateRequest(golden('plain_reply.request.json'));
    expect(plain.ok && plain.request.agent.model.provider).toBe('openai_compatible');
    const anthropic = validateRequest(golden('anthropic_reply.request.json'));
    expect(anthropic.ok && anthropic.request.agent.model.provider).toBe('anthropic');
  });

  it('refuses an unknown model provider', () => {
    const req = golden('plain_reply.request.json');
    (req.agent as { model: Record<string, unknown> }).model.provider = 'bedrock';
    expect(validateRequest(req)).toEqual({ ok: false, fields: ['agent.model.provider'] });
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

  it('accepts contract versions 1 to 3 during the rollout and refuses any other', () => {
    expect(CONTRACT_VERSION).toBe(3);
    for (const v of [1, 2, 3]) {
      expect(validateRequest({ ...golden('plain_reply.request.json'), contractVersion: v }).ok, `v${v}`).toBe(true);
    }
    expect(validateRequest({ ...golden('plain_reply.request.json'), contractVersion: 4 })).toEqual({ ok: false, fields: ['contractVersion'] });
  });

  it('takes tool approval specs on v3 and allowedTools before it, never both', () => {
    const v3 = golden('tool_call.request.json');
    v3.contractVersion = 3;
    const server = (v3.mcpServers as Record<string, unknown>[])[0]!;
    delete server.allowedTools;
    server.tools = [{ name: 'record_expense', approval: { mode: 'conditional', freeWhen: [{ arg: 'confirm', values: [false], absent: true }] } }];
    expect(validateRequest(v3).ok).toBe(true);

    expect(validateRequest({ ...v3, contractVersion: 2 })).toEqual({ ok: false, fields: ['mcpServers.0'] });
    const allowed = structuredClone(v3);
    const allowedServer = (allowed.mcpServers as Record<string, unknown>[])[0]!;
    delete allowedServer.tools;
    allowedServer.allowedTools = ['record_expense'];
    expect(validateRequest({ ...allowed, contractVersion: 2 }).ok).toBe(true);
    expect(validateRequest({ ...allowed, contractVersion: 3 })).toEqual({ ok: false, fields: ['mcpServers.0'] });
    const both = structuredClone(v3);
    (both.mcpServers as Record<string, unknown>[])[0]!.allowedTools = ['record_expense'];
    expect(validateRequest(both)).toEqual({ ok: false, fields: ['mcpServers.0'] });
  });

  it('refuses a conditional spec without conditions, an unknown mode and an unknown spec field', () => {
    const base = golden('tool_call.request.json');
    base.contractVersion = 3;
    const server = (base.mcpServers as Record<string, unknown>[])[0]!;
    delete server.allowedTools;
    for (const approval of [{ mode: 'conditional' }, { mode: 'conditional', freeWhen: [] }, { mode: 'sometimes' }, { mode: 'never', extra: 1 }]) {
      server.tools = [{ name: 'record_expense', approval }];
      expect(validateRequest(base).ok, JSON.stringify(approval)).toBe(false);
    }
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

  it('accepts resume only with a snapshot', () => {
    expect(validateRequest(golden('resume.request.json')).ok).toBe(true);
    const bare = { ...golden('resume.request.json'), snapshot: null };
    expect(validateRequest(bare)).toEqual({ ok: false, fields: ['snapshot'] });
  });

  it('refuses a non-object body', () => {
    expect(validateRequest('hello')).toEqual({ ok: false, fields: [''] });
  });

  it('takes knowledge on v3 as an optional field of at most 10 strict passages', () => {
    const p = { text: 'Refunds take 7 days.', title: 'Refund policy', heading: 'Timing', page: 2, url: 'https://help.example.com/r' };
    const withKnowledge = (knowledge: unknown) => ({ ...golden('plain_reply.request.json'), knowledge });
    expect(CONTRACT_VERSION).toBe(3);
    expect(validateRequest(golden('plain_reply.request.json')).ok).toBe(true);
    expect(validateRequest(withKnowledge([p])).ok).toBe(true);
    expect(validateRequest(withKnowledge([])).ok).toBe(true);
    expect(validateRequest(withKnowledge([{ text: 'x', title: 't' }])).ok).toBe(true);
    expect(MAX_PASSAGES).toBe(10);
    expect(validateRequest(withKnowledge(Array.from({ length: MAX_PASSAGES }).fill(p))).ok).toBe(true);
    expect(validateRequest(withKnowledge(Array.from({ length: MAX_PASSAGES + 1 }).fill(p)))).toEqual({ ok: false, fields: ['knowledge'] });
    expect(validateRequest(withKnowledge([{ ...p, score: 0.9 }]))).toEqual({ ok: false, fields: ['knowledge.0.score'] });
    expect(validateRequest(withKnowledge([{ ...p, text: 'x'.repeat(MAX_PASSAGE_CHARS + 1) }]))).toEqual({ ok: false, fields: ['knowledge.0.text'] });
    expect(validateRequest(withKnowledge([{ ...p, page: 0 }]))).toEqual({ ok: false, fields: ['knowledge.0.page'] });
  });

  it('takes an optional helper model with the model shape', () => {
    const helper = { baseUrl: 'https://helper.example.com/v1', apiKey: 'sk-helper', modelId: 'small' };
    const withHelper = (helperModel: unknown) => {
      const req = golden('plain_reply.request.json');
      (req.agent as Record<string, unknown>).helperModel = helperModel;
      return validateRequest(req);
    };
    const plain = withHelper(helper);
    expect(plain.ok && plain.request.agent.helperModel?.provider).toBe('openai_compatible');
    const anthropic = withHelper({ ...helper, provider: 'anthropic' });
    expect(anthropic.ok && anthropic.request.agent.helperModel?.provider).toBe('anthropic');
    expect(withHelper({ ...helper, prices: {} })).toEqual({ ok: false, fields: ['agent.helperModel.prices'] });
    expect(withHelper({ baseUrl: helper.baseUrl, modelId: helper.modelId })).toEqual({ ok: false, fields: ['agent.helperModel.apiKey'] });
  });

  it('takes optional helper prices next to the budget prices', () => {
    const prices = { input: 1, output: 2, cacheRead: 0, cacheWrite: 0 };
    const withHelperPrices = (helperPrices: unknown) => {
      const req = golden('limit_reached.request.json');
      (req.limits as { budget: Record<string, unknown> }).budget.helperPrices = helperPrices;
      return validateRequest(req);
    };
    expect(withHelperPrices(prices).ok).toBe(true);
    expect(withHelperPrices({ ...prices, embedding: 1 })).toEqual({ ok: false, fields: ['limits.budget.helperPrices.embedding'] });
    expect(withHelperPrices({ ...prices, input: -1 })).toEqual({ ok: false, fields: ['limits.budget.helperPrices.input'] });
  });

  it('refuses a helper model under a budget without helper prices', () => {
    const helper = { baseUrl: 'https://helper.example.com/v1', apiKey: 'sk-helper', modelId: 'small' };
    const budgeted = golden('limit_reached.request.json');
    (budgeted.agent as Record<string, unknown>).helperModel = helper;
    expect(validateRequest(budgeted)).toEqual({ ok: false, fields: ['limits.budget.helperPrices'] });
    (budgeted.limits as { budget: Record<string, unknown> }).budget.helperPrices = { input: 1, output: 2, cacheRead: 0, cacheWrite: 0 };
    expect(validateRequest(budgeted).ok).toBe(true);
    const unbudgeted = golden('plain_reply.request.json');
    (unbudgeted.agent as Record<string, unknown>).helperModel = helper;
    expect(validateRequest(unbudgeted).ok).toBe(true);
  });

  it('takes optional guardrails with a key prefix', () => {
    const withGuardrails = (guardrails: unknown) => validateRequest({ ...golden('plain_reply.request.json'), guardrails });
    const ok = withGuardrails({ keyPrefix: 'oagent_' });
    expect(ok.ok && ok.request.guardrails?.keyPrefix).toBe('oagent_');
    expect(withGuardrails({}).ok).toBe(true);
    for (const keyPrefix of ['', 'a b', 'a.b_', 'x'.repeat(33)]) {
      expect(withGuardrails({ keyPrefix }), keyPrefix).toEqual({ ok: false, fields: ['guardrails.keyPrefix'] });
    }
    expect(withGuardrails({ keyPrefix: 'x'.repeat(32) }).ok).toBe(true);
    expect(withGuardrails({ mode: 'builtin' })).toEqual({ ok: false, fields: ['guardrails.mode'] });
  });

  it.each(requests)('adds none of the optional fields to the golden %s', (name) => {
    const result = validateRequest(golden(name));
    expect(result.ok).toBe(true);
    if (result.ok) {
      expect(result.request.agent.helperModel).toBeUndefined();
      expect(result.request.guardrails).toBeUndefined();
      expect(result.request.limits.budget?.helperPrices).toBeUndefined();
      expect('helperModel' in result.request.agent || 'guardrails' in result.request).toBe(false);
    }
  });
});
