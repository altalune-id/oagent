import type { IncomingMessage, Server, ServerResponse } from 'node:http';
import type { AddressInfo } from 'node:net';
import type { RunTurn } from '../contract/index.ts';
import type { Logger } from '../log.ts';
import { createServer } from 'node:http';

import { validateRequest } from '../contract/index.ts';
import { bearerCheck } from './auth.ts';
import { MAX_BODY_BYTES, readBody } from './body.ts';
import { reply } from './reply.ts';
import { streamTurn } from './stream.ts';

export { MAX_BODY_BYTES } from './body.ts';

export interface ServerDeps {
  token: string;
  maxConcurrent: number;
  log: Logger;
  runTurn: RunTurn;
}

export interface RuntimeServer {
  server: Server;
  drain: (graceMs: number) => Promise<void>;
}

export function createRuntimeServer(deps: ServerDeps): RuntimeServer {
  const authorized = bearerCheck(deps.token);
  let active = 0;
  let draining = false;
  let onIdle: (() => void) | undefined;

  const handleTurn = async (req: IncomingMessage, res: ServerResponse): Promise<void> => {
    if (!authorized(req.headers.authorization)) {
      reply(res, 401);
      return;
    }
    if (Number(req.headers['content-length'] ?? 0) > MAX_BODY_BYTES) {
      res.setHeader('connection', 'close');
      reply(res, 413);
      return;
    }
    if (active >= deps.maxConcurrent) {
      reply(res, 503);
      return;
    }
    active++;
    try {
      const body = await readBody(req);
      if (body === 'too_large') {
        res.setHeader('connection', 'close');
        reply(res, 413);
        req.destroy();
        return;
      }
      let raw: unknown;
      try {
        raw = JSON.parse(body.toString('utf8'));
      } catch {
        reply(res, 400, { error: 'invalid_json' });
        return;
      }
      const result = validateRequest(raw);
      if (!result.ok) {
        deps.log.warn({ code: 'invalid_request', fields: result.fields }, 'turn refused');
        reply(res, 400, { error: 'invalid_request', fields: result.fields });
        return;
      }
      await streamTurn(result.request, res, deps);
    } finally {
      active--;
      if (active === 0) {
        onIdle?.();
      }
    }
  };

  const server = createServer((req, res) => {
    if (draining) {
      res.setHeader('connection', 'close');
      reply(res, 503);
      return;
    }
    const path = (req.url ?? '').split('?')[0];
    if (path === '/ping') {
      res.writeHead(200, { 'content-type': 'text/plain' }).end('ok');
      return;
    }
    if (path !== '/turn') {
      reply(res, 404);
      return;
    }
    if (req.method !== 'POST') {
      reply(res, 405);
      return;
    }
    handleTurn(req, res).catch((error: unknown) => {
      deps.log.error({ error }, 'turn request failed');
      if (!res.headersSent) {
        reply(res, 500);
        return;
      }
      res.end();
    });
  });

  const drain = async (graceMs: number): Promise<void> => {
    draining = true;
    const closed = new Promise<void>(resolve => server.close(() => resolve()));
    server.closeIdleConnections();
    if (active > 0) {
      deps.log.info({ status: 'draining' }, 'waiting for turns');
      await new Promise<void>((resolve) => {
        onIdle = resolve;
        setTimeout(resolve, graceMs).unref();
      });
    }
    if (active > 0) {
      deps.log.warn({ status: 'cut' }, 'grace period over, cutting turns');
    }
    server.closeAllConnections();
    await closed;
  };

  return { server, drain };
}

export async function listen(server: Server, port: number): Promise<number> {
  await new Promise<void>((resolve, reject) => {
    server.once('error', reject);
    server.listen(port, () => {
      server.off('error', reject);
      resolve();
    });
  });
  return (server.address() as AddressInfo).port;
}
