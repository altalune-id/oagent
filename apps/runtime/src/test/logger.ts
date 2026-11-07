import type { Logger, LogOptions } from '../log.ts';
import { Writable } from 'node:stream';

import { createLogger } from '../log.ts';

export function captureLogger(lines: string[] = [], opts: LogOptions = { logLevel: 'info', logFormat: 'json' }): Logger {
  return createLogger(opts, new Writable({
    write(chunk: Uint8Array | string, _encoding, done) {
      lines.push(String(chunk).trimEnd());
      done();
    },
  }));
}
