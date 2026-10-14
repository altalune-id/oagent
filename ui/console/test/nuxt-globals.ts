import type { Ref } from 'vue';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { defineStore } from 'pinia';
import { afterEach, beforeEach, vi } from 'vitest';
import { computed, ref } from 'vue';

import { ErrorDetailSchema } from '../gen/apperror/v1/error_pb';

// Stand-ins for the Nuxt auto-imports the app uses (nuxt.config.ts has imports.scan: false, so only built-ins are globals).

export type ClientName = '$orgClient' | '$projectClient' | '$agentClient' | '$apiKeyClient' | '$webhookClient' | '$authClient' | '$memberClient' | '$inviteClient';

let clients: Partial<Record<ClientName, Record<string, unknown>>> = {};
let known = new Set<string>();
let states = new Map<string, Ref<unknown>>();

export const runtimeConfig = { public: { apiUrl: '', appVersion: '' } };

export function setClient(name: ClientName, client: Record<string, unknown>) {
  clients[name] = client;
}

export function setKnownKeys(...keys: string[]) {
  known = new Set(keys);
}

export function t(key: string, params?: Record<string, unknown>): string {
  return params && Object.keys(params).length > 0 ? `${key}|${JSON.stringify(params)}` : key;
}

export function connectError(code: string, meta: Record<string, string> = {}, requestId = 'req-1'): ConnectError {
  return new ConnectError('server text', Code.InvalidArgument, undefined, [
    { desc: ErrorDetailSchema, value: create(ErrorDetailSchema, { code, meta, requestId }) },
  ]);
}

export function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

export function fakeStorage() {
  const data = new Map<string, string>();
  return {
    data,
    getItem: vi.fn((key: string) => data.get(key) ?? null),
    setItem: vi.fn((key: string, value: string) => {
      data.set(key, value);
    }),
  };
}

export const navigateTo = vi.fn(async (_to: string) => undefined);
export const useRuntimeConfig = vi.fn(() => runtimeConfig);

Object.assign(globalThis, {
  ref,
  computed,
  defineStore,
  navigateTo,
  useRuntimeConfig,
  useNuxtApp: () => ({ ...clients }),
  useI18n: () => ({ t, te: (key: string) => known.has(key) }),
  useState: <T>(key: string, init: () => T): Ref<T> => {
    if (!states.has(key)) {
      states.set(key, ref(init()) as Ref<unknown>);
    }
    return states.get(key) as Ref<T>;
  },
});

beforeEach(() => {
  clients = {};
  known = new Set();
  states = new Map();
  runtimeConfig.public.apiUrl = '';
  navigateTo.mockClear();
  useRuntimeConfig.mockClear();
});

afterEach(() => {
  vi.unstubAllGlobals();
});
