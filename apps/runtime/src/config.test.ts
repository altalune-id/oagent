import { describe, expect, it } from 'vitest';

import { ConfigError, loadConfig } from './config.ts';

const TOKEN = 'x'.repeat(32);

function fields(env: Record<string, string | undefined>): string[] {
  try {
    loadConfig(env);
  } catch (error) {
    if (error instanceof ConfigError) {
      return error.fields;
    }
    throw error;
  }
  return [];
}

describe('loadConfig', () => {
  it('fills every default from a token alone', () => {
    expect(loadConfig({ RUNTIME_TOKEN: TOKEN })).toEqual({
      token: TOKEN,
      port: 5160,
      maxConcurrent: 8,
      env: 'production',
      allowPrivate: new Set(),
      ignoredAllowPrivate: [],
      ignoredProxyVars: [],
      shutdownGraceMs: 25_000,
      logLevel: 'info',
      logFormat: 'text',
    });
  });

  it('reads every variable', () => {
    const config = loadConfig({
      RUNTIME_TOKEN: TOKEN,
      PORT: '6000',
      RUNTIME_MAX_CONCURRENT: '2',
      RUNTIME_ENV: 'development',
      RUNTIME_ALLOW_PRIVATE: '[::1]:5190, Localhost:5190',
      RUNTIME_SHUTDOWN_GRACE_MS: '1000',
      RUNTIME_LOG_LEVEL: 'debug',
      RUNTIME_LOG_FORMAT: 'json',
    });
    expect(config).toMatchObject({ port: 6000, maxConcurrent: 2, env: 'development', shutdownGraceMs: 1000, logLevel: 'debug', logFormat: 'json' });
    expect(config.allowPrivate).toEqual(new Set(['::1:5190', 'localhost:5190']));
  });

  it('treats empty strings as unset', () => {
    expect(loadConfig({ RUNTIME_TOKEN: TOKEN, PORT: '', RUNTIME_MAX_CONCURRENT: '', RUNTIME_ENV: '', RUNTIME_LOG_LEVEL: '', RUNTIME_LOG_FORMAT: '' }))
      .toMatchObject({ port: 5160, maxConcurrent: 8, env: 'production', logLevel: 'info', logFormat: 'text' });
  });

  it('refuses a missing or short token without echoing it', () => {
    expect(fields({})).toEqual(['RUNTIME_TOKEN']);
    expect(fields({ RUNTIME_TOKEN: 'short-secret' })).toEqual(['RUNTIME_TOKEN']);
    expect(() => loadConfig({ RUNTIME_TOKEN: 'short-secret' })).toThrow(/^invalid config: RUNTIME_TOKEN$/);
  });

  it('names every bad field at once', () => {
    expect(fields({
      RUNTIME_TOKEN: TOKEN,
      PORT: 'x',
      RUNTIME_MAX_CONCURRENT: '0',
      RUNTIME_ENV: 'staging',
      RUNTIME_SHUTDOWN_GRACE_MS: '-1',
      RUNTIME_LOG_LEVEL: 'trace',
      RUNTIME_LOG_FORMAT: 'yaml',
    })).toEqual(['PORT', 'RUNTIME_MAX_CONCURRENT', 'RUNTIME_ENV', 'RUNTIME_SHUTDOWN_GRACE_MS', 'RUNTIME_LOG_LEVEL', 'RUNTIME_LOG_FORMAT']);
  });

  it('refuses a port out of range and a fraction', () => {
    expect(fields({ RUNTIME_TOKEN: TOKEN, PORT: '65536' })).toEqual(['PORT']);
    expect(fields({ RUNTIME_TOKEN: TOKEN, PORT: '80.5' })).toEqual(['PORT']);
  });

  it('refuses an allow-list entry that is not host:port', () => {
    expect(fields({ RUNTIME_TOKEN: TOKEN, RUNTIME_ENV: 'development', RUNTIME_ALLOW_PRIVATE: 'localhost' })).toEqual(['RUNTIME_ALLOW_PRIVATE']);
  });

  it('drops the allow-list in production and reports it as ignored', () => {
    const config = loadConfig({ RUNTIME_TOKEN: TOKEN, RUNTIME_ALLOW_PRIVATE: '127.0.0.1:5190' });
    expect(config.allowPrivate.size).toBe(0);
    expect(config.ignoredAllowPrivate).toEqual(['127.0.0.1:5190']);
  });

  it('reports the proxy variables it ignores', () => {
    expect(loadConfig({ RUNTIME_TOKEN: TOKEN, HTTPS_PROXY: 'http://p:1', NODE_USE_ENV_PROXY: '1' }).ignoredProxyVars).toEqual(['HTTPS_PROXY', 'NODE_USE_ENV_PROXY']);
  });

  it('returns a frozen object', () => {
    expect(Object.isFrozen(loadConfig({ RUNTIME_TOKEN: TOKEN }))).toBe(true);
  });
});
