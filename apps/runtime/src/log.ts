import process from 'node:process';

const ALLOWED = new Set([
  'run',
  'thread',
  'agent',
  'agentVersion',
  'attempt',
  'status',
  'code',
  'ms',
  'fields',
  'host',
  'hosts',
  'vars',
  'server',
  'port',
  'error',
]);
const MAX_STRING = 200;

export type Fields = Record<string, unknown>;

export interface Logger {
  info: (msg: string, fields?: Fields) => void;
  warn: (msg: string, fields?: Fields) => void;
  error: (msg: string, fields?: Fields) => void;
}

function errorKind(value: unknown): string {
  return value instanceof Error ? value.name : typeof value;
}

function clean(value: unknown): unknown {
  if (typeof value === 'string') {
    return value.slice(0, MAX_STRING);
  }
  if (typeof value === 'number' || typeof value === 'boolean') {
    return value;
  }
  if (Array.isArray(value) && value.every(v => typeof v === 'string')) {
    return value.map(v => v.slice(0, MAX_STRING));
  }
  return undefined;
}

export function createLogger(write: (line: string) => void = line => process.stdout.write(`${line}\n`)): Logger {
  const emit = (level: string, msg: string, fields: Fields = {}): void => {
    const out: Fields = { level, msg };
    for (const [key, value] of Object.entries(fields)) {
      const v = key === 'error' ? errorKind(value) : ALLOWED.has(key) ? clean(value) : undefined;
      if (v !== undefined) {
        out[key] = v;
      }
    }
    write(JSON.stringify(out));
  };
  return {
    info: (msg, fields) => emit('info', msg, fields),
    warn: (msg, fields) => emit('warn', msg, fields),
    error: (msg, fields) => emit('error', msg, fields),
  };
}

export interface StrandsLogger {
  debug: (...args: unknown[]) => void;
  info: (...args: unknown[]) => void;
  warn: (...args: unknown[]) => void;
  error: (...args: unknown[]) => void;
}

export function strandsLogger(log: Logger): StrandsLogger {
  return {
    debug: () => {},
    info: () => {},
    warn: () => log.warn('strands warning'),
    error: () => log.error('strands error'),
  };
}
