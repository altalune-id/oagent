import type { Logger } from './log.ts';
import process from 'node:process';

const FORCE_EXIT_MARGIN_MS = 5_000;

export function handleShutdown(log: Logger, graceMs: number, close: () => Promise<void>): void {
  let closing = false;
  const onSignal = (signal: NodeJS.Signals): void => {
    if (closing) {
      log.warn({ status: signal }, 'second signal, exiting now');
      process.exit(1);
    }
    closing = true;
    log.info({ status: signal }, 'shutdown started');
    setTimeout(() => {
      log.error('shutdown timed out');
      process.exit(1);
    }, graceMs + FORCE_EXIT_MARGIN_MS).unref();
    close().then(
      () => {
        log.info('shutdown done');
        process.exit(0);
      },
      (error: unknown) => {
        log.error({ error }, 'shutdown failed');
        process.exit(1);
      },
    );
  };
  process.on('SIGTERM', onSignal);
  process.on('SIGINT', onSignal);
}
