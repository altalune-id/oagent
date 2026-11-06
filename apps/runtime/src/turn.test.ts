import type { AddressInfo } from 'node:net';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';

import { createLogger } from './log.ts';
import { createRuntimeServer } from './server.ts';

const TOKEN = 'secret-bearer-'.padEnd(40, 'x');
const SECRETS = ['sk-test', 'tok-test', TOKEN];
const body = readFileSync(fileURLToPath(new URL('../../../internal/kernel/runtime/testdata/turn/tool_call.request.json', import.meta.url)), 'utf8');

describe('turn logging', () => {
  it('never writes a secret from the request, the bearer or an error', async () => {
    const lines: string[] = [];
    const server = createRuntimeServer({
      token: TOKEN,
      maxConcurrent: 8,
      log: createLogger(line => lines.push(line)),
      runTurn: async (req) => {
        throw new Error(`model refused key ${req.agent.model.apiKey} and ${req.mcpServers[0]?.auth.value}`);
      },
    });
    await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve));
    const base = `http://127.0.0.1:${(server.address() as AddressInfo).port}`;
    const post = (b: string, auth = `Bearer ${TOKEN}`) =>
      fetch(`${base}/turn`, { method: 'POST', headers: { authorization: auth }, body: b }).then(r => r.text());

    await post(body);
    const bad = JSON.parse(body) as { agent: { model: { apiKey: unknown } } };
    bad.agent.model.apiKey = { leaked: 'sk-test' };
    await post(JSON.stringify(bad));
    await post(body, 'Bearer sk-test');
    server.close();

    expect(lines.length).toBeGreaterThan(0);
    const all = lines.join('\n');
    for (const secret of SECRETS) {
      expect(all).not.toContain(secret);
    }
    expect(all).toContain('"run":"Rn5Q8wYk2HhT3mVz7XcLp"');
  });
});
