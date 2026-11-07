import type { ServerResponse } from 'node:http';
import type { Emit, RunTurn, TurnRequest } from '../contract/index.ts';
import type { Logger } from '../log.ts';

import { TurnRefusedError } from '../contract/index.ts';
import { reply } from './reply.ts';

export interface StreamDeps {
  log: Logger;
  runTurn: RunTurn;
}

export async function streamTurn(request: TurnRequest, res: ServerResponse, deps: StreamDeps): Promise<void> {
  const log = deps.log.child({ run: request.ids.run, thread: request.ids.thread, attempt: request.ids.attempt });
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
  log.info('turn started');
  try {
    await deps.runTurn(request, emit, controller.signal);
    log.info({ ms: Date.now() - started, status: controller.signal.aborted ? 'aborted' : 'ended' }, 'turn ended');
  } catch (error) {
    if (error instanceof TurnRefusedError && !res.headersSent) {
      log.warn({ code: error.code }, 'turn refused');
      reply(res, error.status, { error: error.code });
      return;
    }
    log.error({ ms: Date.now() - started, error }, 'turn threw');
  } finally {
    head();
    res.end();
  }
}
