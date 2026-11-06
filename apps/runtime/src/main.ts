import process from 'node:process';

import { start } from './http/server.ts';
import { createLogger } from './log.ts';
import { ModelClients } from './models/clients.ts';
import { createRunTurn } from './turn/agui.ts';

const log = createLogger();
start(process.env, createRunTurn({ models: new ModelClients(), log }), log).catch((error: unknown) => {
  log.error('start failed', { error });
  process.exit(1);
});
