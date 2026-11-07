import type { LookupAddress } from 'node:dns';
import { lookup as dnsLookup } from 'node:dns/promises';
import { BlockList, isIP } from 'node:net';
import { Agent, buildConnector, setGlobalDispatcher } from 'undici';

const CONNECT_TIMEOUT_MS = 10_000;
const HEADERS_TIMEOUT_MS = 60_000;
const BODY_TIMEOUT_MS = 120_000;

const deny = new BlockList();
for (const [net, bits] of [
  ['0.0.0.0', 8],
  ['10.0.0.0', 8],
  ['100.64.0.0', 10],
  ['127.0.0.0', 8],
  ['169.254.0.0', 16],
  ['172.16.0.0', 12],
  ['192.0.0.0', 24],
  ['192.0.2.0', 24],
  ['192.168.0.0', 16],
  ['198.18.0.0', 15],
  ['198.51.100.0', 24],
  ['203.0.113.0', 24],
  ['224.0.0.0', 4],
  ['240.0.0.0', 4],
  ['255.255.255.255', 32],
] as const) {
  deny.addSubnet(net, bits, 'ipv4');
}
for (const [net, bits] of [
  ['::', 128],
  ['::1', 128],
  ['fc00::', 7],
  ['fe80::', 10],
  ['fec0::', 10],
  ['ff00::', 8],
  ['64:ff9b::', 96],
  ['64:ff9b:1::', 48],
  ['100::', 64],
  ['2001::', 32],
  ['2001:db8::', 32],
  ['2002::', 16],
] as const) {
  deny.addSubnet(net, bits, 'ipv6');
}

export type Log = (msg: string, fields: Record<string, unknown>) => void;

export type Resolve = (hostname: string) => Promise<LookupAddress[]>;

export class SsrfRefusedError extends Error {
  readonly code = 'ESSRF';
  readonly host: string;

  constructor(host: string) {
    super(`ssrf refused ${host}`);
    this.host = host;
    this.name = 'SsrfRefusedError';
  }
}

function unbracket(host: string): string {
  return host.replace(/^\[(.*)\]$/, '$1');
}

export function isPrivateOrLocal(ip: string): boolean {
  const addr = unbracket(ip);
  const family = isIP(addr);
  if (family === 0) {
    return true;
  }
  return deny.check(addr, family === 4 ? 'ipv4' : 'ipv6');
}

export async function checkedLookup(hostname: string, resolve: Resolve): Promise<LookupAddress[]> {
  const addrs = await resolve(hostname);
  if (addrs.length === 0 || addrs.some(a => isPrivateOrLocal(a.address))) {
    throw new SsrfRefusedError(hostname);
  }
  return addrs;
}

function defaultLog(msg: string, fields: Record<string, unknown>): void {
  console.warn(JSON.stringify({ level: 'warn', msg, ...fields }));
}

const defaultResolve: Resolve = hostname => dnsLookup(hostname, { all: true, verbatim: true });

export interface GuardOptions {
  allowPrivate?: ReadonlySet<string>;
  ignoredAllowPrivate?: readonly string[];
  ignoredProxyVars?: readonly string[];
  log?: Log;
  resolve?: Resolve;
}

export type LookupCallback = (err: Error | null, address: string | LookupAddress[], family?: number) => void;

export function guardedLookup(resolve: Resolve, log: Log) {
  return (hostname: string, opts: { all?: boolean }, cb: LookupCallback): void => {
    checkedLookup(hostname, resolve).then(
      (addrs) => {
        if (opts.all) {
          cb(null, addrs);
          return;
        }
        cb(null, addrs[0]!.address, addrs[0]!.family);
      },
      (error: Error) => {
        if (error instanceof SsrfRefusedError) {
          log('ssrf refused', { host: hostname });
        }
        cb(error, []);
      },
    );
  };
}

let installed: Agent | undefined;

export function installGuard(options: GuardOptions = {}): Agent {
  const log = options.log ?? defaultLog;
  const resolve = options.resolve ?? defaultResolve;
  const allow = options.allowPrivate ?? new Set<string>();
  if (options.ignoredAllowPrivate?.length) {
    log('private egress list ignored in production', { hosts: [...options.ignoredAllowPrivate] });
  }
  if (options.ignoredProxyVars?.length) {
    log('proxy env ignored', { vars: [...options.ignoredProxyVars] });
  }
  if (allow.size > 0) {
    log('private egress allowed', { hosts: [...allow] });
  }

  const lookup = guardedLookup(resolve, log);
  const plain = buildConnector({ timeout: CONNECT_TIMEOUT_MS });
  const guarded = buildConnector({ timeout: CONNECT_TIMEOUT_MS, lookup } as buildConnector.BuildOptions);

  const agent = new Agent({
    headersTimeout: HEADERS_TIMEOUT_MS,
    bodyTimeout: BODY_TIMEOUT_MS,
    connect: (opts, cb) => {
      const host = unbracket(opts.hostname).toLowerCase();
      const port = opts.port || (opts.protocol === 'https:' ? '443' : '80');
      if (allow.has(`${host}:${port}`)) {
        plain(opts, cb);
        return;
      }
      if (isIP(host) !== 0) {
        if (isPrivateOrLocal(host)) {
          log('ssrf refused', { host });
          cb(new SsrfRefusedError(host), null);
          return;
        }
        plain(opts, cb);
        return;
      }
      guarded(opts, cb);
    },
  });

  const previous = installed;
  setGlobalDispatcher(agent);
  installed = agent;
  if (previous) {
    void previous.close();
  }
  return agent;
}
