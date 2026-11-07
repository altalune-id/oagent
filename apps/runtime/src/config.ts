import { Buffer } from 'node:buffer';
import { z } from 'zod';

const MIN_TOKEN_BYTES = 32;
const PROXY_VARS = ['HTTP_PROXY', 'HTTPS_PROXY', 'NODE_USE_ENV_PROXY', 'http_proxy', 'https_proxy'] as const;
const HOST_PORT = /^(?:\[([^\]]+)\]|([^:]+)):(\d+)$/;

function unset(value: unknown): unknown {
  return value === '' ? undefined : value;
}

function int(min: number, fallback: number, max = Number.MAX_SAFE_INTEGER) {
  return z.preprocess(unset, z.coerce.number().int().min(min).max(max).default(fallback));
}

function oneOf<const T extends readonly [string, ...string[]]>(values: T, fallback: T[number]) {
  return z.preprocess(unset, z.enum(values).default(fallback));
}

const allowList = z.string().default('').transform((raw, ctx) => {
  const out = new Set<string>();
  for (const entry of raw.split(',').map(s => s.trim()).filter(Boolean)) {
    const m = HOST_PORT.exec(entry);
    if (!m) {
      ctx.addIssue({ code: 'custom', message: `"${entry}" is not host:port` });
      continue;
    }
    out.add(`${(m[1] ?? m[2])!.toLowerCase()}:${m[3]}`);
  }
  return out;
});

const EnvSchema = z.object({
  RUNTIME_TOKEN: z.string().refine(v => Buffer.byteLength(v) >= MIN_TOKEN_BYTES, `must be at least ${MIN_TOKEN_BYTES} bytes`),
  PORT: int(0, 5160, 65535),
  RUNTIME_MAX_CONCURRENT: int(1, 8),
  RUNTIME_ENV: oneOf(['development', 'production'], 'production'),
  RUNTIME_ALLOW_PRIVATE: allowList,
  RUNTIME_SHUTDOWN_GRACE_MS: int(0, 25_000),
  RUNTIME_LOG_LEVEL: oneOf(['debug', 'info', 'warn', 'error'], 'info'),
  RUNTIME_LOG_FORMAT: oneOf(['text', 'json'], 'text'),
  HTTP_PROXY: z.string().optional(),
  HTTPS_PROXY: z.string().optional(),
  NODE_USE_ENV_PROXY: z.string().optional(),
  http_proxy: z.string().optional(),
  https_proxy: z.string().optional(),
}).transform(e => ({
  token: e.RUNTIME_TOKEN,
  port: e.PORT,
  maxConcurrent: e.RUNTIME_MAX_CONCURRENT,
  env: e.RUNTIME_ENV,
  allowPrivate: e.RUNTIME_ENV === 'development' ? e.RUNTIME_ALLOW_PRIVATE : new Set<string>(),
  ignoredAllowPrivate: e.RUNTIME_ENV === 'production' ? [...e.RUNTIME_ALLOW_PRIVATE] : [],
  ignoredProxyVars: PROXY_VARS.filter(v => e[v]),
  shutdownGraceMs: e.RUNTIME_SHUTDOWN_GRACE_MS,
  logLevel: e.RUNTIME_LOG_LEVEL,
  logFormat: e.RUNTIME_LOG_FORMAT,
}));

export type Config = Readonly<z.output<typeof EnvSchema>>;

export class ConfigError extends Error {
  readonly fields: string[];

  constructor(fields: string[]) {
    super(`invalid config: ${fields.join(', ')}`);
    this.name = 'ConfigError';
    this.fields = fields;
  }
}

export function loadConfig(env: Record<string, string | undefined>): Config {
  const parsed = EnvSchema.safeParse(env);
  if (!parsed.success) {
    throw new ConfigError([...new Set(parsed.error.issues.map(i => i.path.join('.')))]);
  }
  return Object.freeze(parsed.data);
}
