import type { IncomingMessage, Server, ServerResponse } from 'node:http';
import type { Logger } from '../log.ts';
import type { TurnRequest } from './contract.ts';
import { Buffer } from 'node:buffer';
import { createHash, timingSafeEqual } from 'node:crypto';
import { createServer } from 'node:http';
import process from 'node:process';
import { configureLogging } from '@strands-agents/sdk';

import { installGuard } from '../egress/ssrf.ts';
import { createLogger, strandsLogger } from '../log.ts';
import { TurnRefusedError, validateRequest } from './contract.ts';

export const MAX_BODY_BYTES = 8 * 1024 * 1024;
const MIN_TOKEN_BYTES = 32;
const DEFAULT_PORT = 5160;
const DEFAULT_MAX_CONCURRENT = 8;

export type Emit = (event: Record<string, unknown>) => Promise<void>;

export type RunTurn = (request: TurnRequest, emit: Emit, signal: AbortSignal) => Promise<void>;

export interface RuntimeConfig {
  token: string;
  maxConcurrent: number;
  port: number;
}

export interface ServerDeps {
  token: string;
  maxConcurrent: number;
  log: Logger;
  runTurn: RunTurn;
}

function positiveInt(env: Record<string, string | undefined>, name: string, fallback: number, min: number): number {
  const raw = env[name];
  if (raw === undefined || raw === '') {
    return fallback;
  }
  const n = Number(raw);
  if (!Number.isInteger(n) || n < min) {
    throw new Error(`${name} must be an integer of at least ${min}`);
  }
  return n;
}

export function loadConfig(env: Record<string, string | undefined>): RuntimeConfig {
  const token = env.RUNTIME_TOKEN ?? '';
  if (Buffer.byteLength(token) < MIN_TOKEN_BYTES) {
    throw new Error(`RUNTIME_TOKEN must be at least ${MIN_TOKEN_BYTES} bytes`);
  }
  return {
    token,
    maxConcurrent: positiveInt(env, 'RUNTIME_MAX_CONCURRENT', DEFAULT_MAX_CONCURRENT, 1),
    port: positiveInt(env, 'PORT', DEFAULT_PORT, 0),
  };
}

function digest(value: string): Buffer {
  return createHash('sha256').update(value).digest();
}

function authorized(header: string | undefined, want: Buffer): boolean {
  const m = /^Bearer (.+)$/.exec(header ?? '');
  return m !== null && timingSafeEqual(digest(m[1]!), want);
}

function reply(res: ServerResponse, status: number, body?: Record<string, unknown>): void {
  if (body === undefined) {
    res.writeHead(status).end();
    return;
  }
  res.writeHead(status, { 'content-type': 'application/json' }).end(JSON.stringify(body));
}

function readBody(req: IncomingMessage): Promise<Buffer | 'too_large'> {
  return new Promise((resolve, reject) => {
    const chunks: Buffer[] = [];
    let size = 0;
    req.on('data', (chunk: Buffer) => {
      size += chunk.length;
      if (size > MAX_BODY_BYTES) {
        req.removeAllListeners('data');
        resolve('too_large');
        return;
      }
      chunks.push(chunk);
    });
    req.on('end', () => resolve(Buffer.concat(chunks)));
    req.on('error', reject);
  });
}

export function createRuntimeServer(deps: ServerDeps): Server {
  const want = digest(deps.token);
  let active = 0;

  const stream = async (request: TurnRequest, res: ServerResponse): Promise<void> => {
    const ids = { run: request.ids.run, thread: request.ids.thread, attempt: request.ids.attempt };
    const controller = new AbortController();
    res.on('close', () => {
      if (!res.writableFinished) {
        controller.abort();
      }
    });
    const head = (): void => {
      if (!res.headersSent) {
        res.writeHead(200, { 'content-type': 'application/x-ndjson', 'cache-control': 'no-store' });
        res.flushHeaders();
      }
    };
    const emit: Emit = async (event) => {
      if (res.writableEnded || res.destroyed) {
        return;
      }
      head();
      if (res.write(`${JSON.stringify(event)}\n`)) {
        return;
      }
      await new Promise<void>((resolve) => {
        const done = (): void => {
          res.off('drain', done);
          res.off('close', done);
          resolve();
        };
        res.on('drain', done);
        res.on('close', done);
      });
    };
    const started = Date.now();
    deps.log.info('turn started', ids);
    try {
      await deps.runTurn(request, emit, controller.signal);
      deps.log.info('turn ended', { ...ids, ms: Date.now() - started, status: controller.signal.aborted ? 'aborted' : 'ended' });
    } catch (error) {
      if (error instanceof TurnRefusedError && !res.headersSent) {
        deps.log.warn('turn refused', { ...ids, code: error.code });
        reply(res, error.status, { error: error.code });
        return;
      }
      deps.log.error('turn threw', { ...ids, ms: Date.now() - started, error });
    } finally {
      head();
      res.end();
    }
  };

  const handleTurn = async (req: IncomingMessage, res: ServerResponse): Promise<void> => {
    if (!authorized(req.headers.authorization, want)) {
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
        deps.log.warn('turn refused', { code: 'invalid_request', fields: result.fields });
        reply(res, 400, { error: 'invalid_request', fields: result.fields });
        return;
      }
      await stream(result.request, res);
    } finally {
      active--;
    }
  };

  return createServer((req, res) => {
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
      deps.log.error('turn request failed', { error });
      if (!res.headersSent) {
        reply(res, 500);
        return;
      }
      res.end();
    });
  });
}

export async function start(env: Record<string, string | undefined>, runTurn: RunTurn, log: Logger = createLogger()): Promise<Server> {
  installGuard({ env, log: log.warn });
  configureLogging(strandsLogger(log));
  const config = loadConfig(env);
  const server = createRuntimeServer({ token: config.token, maxConcurrent: config.maxConcurrent, log, runTurn });
  await new Promise<void>(resolve => server.listen(config.port, resolve));
  log.info('listening', { port: (server.address() as { port: number }).port });
  process.once('SIGTERM', () => server.close());
  return server;
}
