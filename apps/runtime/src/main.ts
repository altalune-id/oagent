import process from 'node:process';
import { configureLogging } from '@strands-agents/sdk';

import { ConfigError, loadConfig } from './config.ts';
import { installGuard } from './egress/index.ts';
import { createRuntimeServer, listen } from './http/index.ts';
import { createLogger, strandsLogger } from './log.ts';
import { createModels } from './models/index.ts';
import { handleShutdown } from './shutdown.ts';
import { createRunTurn } from './turn/index.ts';

async function main(): Promise<void> {
  let log = createLogger({ logLevel: 'info', logFormat: 'text' });
  try {
    const config = loadConfig(process.env);
    log = createLogger(config);
    installGuard({
      allowPrivate: config.allowPrivate,
      ignoredAllowPrivate: config.ignoredAllowPrivate,
      ignoredProxyVars: config.ignoredProxyVars,
      log: (msg, fields) => log.warn(fields, msg),
    });
    configureLogging(strandsLogger(log));
    const runtime = createRuntimeServer({
      token: config.token,
      maxConcurrent: config.maxConcurrent,
      log,
      runTurn: createRunTurn({ models: createModels(), log }),
    });
    const port = await listen(runtime.server, config.port);
    log.info({ port }, 'listening');
    handleShutdown(log, config.shutdownGraceMs, () => runtime.drain(config.shutdownGraceMs));
  } catch (error) {
    log.error({ error, ...(error instanceof ConfigError && { fields: error.fields }) }, 'start failed');
    process.exit(1);
  }
}

void main();
