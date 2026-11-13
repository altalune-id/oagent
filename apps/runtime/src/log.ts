import type { DestinationStream, Logger as PinoLogger } from 'pino';
import type { Config } from './config.ts';
import { destination, pino, stdTimeFunctions } from 'pino';
import pretty from 'pino-pretty';

const ALLOWED = new Set([
  'run',
  'thread',
  'agent',
  'agentVersion',
  'attempt',
  'status',
  'code',
  'ms',
  'stopReason',
  'inputTokens',
  'outputTokens',
  'parts',
  'fields',
  'host',
  'hosts',
  'vars',
  'server',
  'port',
  'error',
]);
const MAX_STRING = 200;

export type Logger = PinoLogger;

export type LogOptions = Pick<Config, 'logLevel' | 'logFormat'>;

function errorKind(value: unknown): string {
  return value instanceof Error ? value.name : typeof value;
}

function trim(value: unknown): unknown {
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

function clean(fields: Record<string, unknown> | undefined): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(fields ?? {})) {
    const v = key === 'error' ? errorKind(value) : ALLOWED.has(key) ? trim(value) : undefined;
    if (v !== undefined) {
      out[key] = v;
    }
  }
  return out;
}

export function createLogger(opts: LogOptions, dest: DestinationStream = destination({ dest: 1, sync: true })): Logger {
  const out = opts.logFormat === 'text' ? pretty({ destination: dest, colorize: false, singleLine: true, errorLikeObjectKeys: [], sync: true }) : dest;
  const logger = pino({
    level: opts.logLevel,
    base: undefined,
    messageKey: 'msg',
    timestamp: stdTimeFunctions.isoTime,
    formatters: {
      level: label => ({ level: label }),
      bindings: clean,
      log: clean,
    },
  }, out);
  const child = logger.child;
  // SECURITY: pino skips formatters.bindings for children, so each child is given the allow-list itself.
  logger.child = function (this: Logger, bindings, options) {
    return child.call(this, bindings, { ...options, formatters: { ...options?.formatters, bindings: clean } });
  } as Logger['child'];
  return logger;
}

export interface StrandsLogger {
  debug: (...args: unknown[]) => void;
  info: (...args: unknown[]) => void;
  warn: (...args: unknown[]) => void;
  error: (...args: unknown[]) => void;
}

export function strandsLogger(log: Logger): StrandsLogger {
  return {
    debug: () => log.debug('strands debug'),
    info: () => log.debug('strands info'),
    warn: () => log.warn('strands warning'),
    error: () => log.error('strands error'),
  };
}
