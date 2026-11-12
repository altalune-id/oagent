import type { AddressInfo } from 'node:net';
import { readFileSync } from 'node:fs';
import { createServer } from 'node:http';
import { fileURLToPath } from 'node:url';
import { afterEach, describe, expect, it } from 'vitest';

import { ModelProvider } from '../contract/index.ts';
import { createModels } from '../models/index.ts';
import { captureLogger } from '../test/logger.ts';
import { createRunTurn } from '../turn/index.ts';
import { createRuntimeServer } from './index.ts';

const TOKEN = 'abort-test-bearer-'.padEnd(40, 'x');
const golden = readFileSync(fileURLToPath(new URL('../../../../internal/kernel/runtime/testdata/turn/plain_reply.request.json', import.meta.url)), 'utf8');
const closers: (() => void)[] = [];

afterEach(() => {
  closers.splice(0).forEach(close => close());
});

function within<T>(work: Promise<T>, ms: number, what: string): Promise<T> {
  return Promise.race([work, new Promise<never>((_, reject) => setTimeout(() => reject(new Error(`timed out: ${what}`)), ms))]);
}

async function listen(server: ReturnType<typeof createServer>): Promise<string> {
  await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve));
  closers.push(() => server.closeAllConnections());
  closers.push(() => server.close());
  return `http://127.0.0.1:${(server.address() as AddressInfo).port}`;
}

async function hangingModel(): Promise<{ base: string, path: Promise<string>, closed: Promise<void> }> {
  let seen!: (path: string) => void;
  let gone!: () => void;
  const path = new Promise<string>((resolve) => {
    seen = resolve;
  });
  const closed = new Promise<void>((resolve) => {
    gone = resolve;
  });
  const server = createServer((req, res) => {
    res.on('close', () => gone());
    seen(req.url ?? '');
    res.writeHead(200, { 'content-type': 'text/event-stream' });
    res.write(': thinking\n\n');
  });
  return { base: await listen(server), path, closed };
}

describe('closing /turn', () => {
  for (const c of [
    { provider: ModelProvider.OPENAI_COMPATIBLE, suffix: '/v1', path: '/v1/chat/completions' },
    { provider: ModelProvider.ANTHROPIC, suffix: '', path: '/v1/messages' },
  ]) {
    it(`aborts the in-flight ${c.provider} model call itself`, async () => {
      const model = await hangingModel();
      const lines: string[] = [];
      const log = captureLogger(lines);
      const { server } = createRuntimeServer({
        token: TOKEN,
        maxConcurrent: 2,
        log,
        runTurn: createRunTurn({ models: createModels(), log }),
      });
      const runtime = await listen(server);
      const request = JSON.parse(golden) as { agent: { model: Record<string, unknown> } };
      request.agent.model.baseUrl = model.base + c.suffix;
      request.agent.model.provider = c.provider;

      const client = new AbortController();
      const turn = fetch(`${runtime}/turn`, {
        method: 'POST',
        headers: { authorization: `Bearer ${TOKEN}` },
        body: JSON.stringify(request),
        signal: client.signal,
      }).then(async res => res.text()).catch(() => 'aborted');

      expect(await within(model.path, 3000, 'the model call starts')).toBe(c.path);
      client.abort();
      await within(model.closed, 3000, 'the model connection closes after /turn closes');
      expect(await turn).toBe('aborted');
    });
  }
});
