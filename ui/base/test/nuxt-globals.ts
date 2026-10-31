import type { Ref } from 'vue';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { defineStore } from 'pinia';
import { afterEach, beforeEach, vi } from 'vitest';
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, useId, useSlots, watch } from 'vue';

import { ErrorDetailSchema } from '#gen/apperror/v1/error_pb';

// Stand-ins for the Nuxt auto-imports the app uses (nuxt.config.ts has imports.scan: false, so only built-ins are globals).

export type ClientName = `$${string}Client`;

let clients: Partial<Record<ClientName, Record<string, unknown>>> = {};
let known = new Set<string>();
let states = new Map<string, Ref<unknown>>();

const locale = ref('en-US');
const locales = ref([{ code: 'en-US', name: 'English (US)' }, { code: 'id-ID', name: 'Bahasa Indonesia' }]);

export const setLocale = vi.fn(async (code: string) => {
  locale.value = code;
});

export const runtimeConfig: { public: Record<string, string> } = { public: { apiUrl: '', appVersion: '' } };

export const route = reactive({ path: '/', fullPath: '/', params: {} as Record<string, string>, query: {} as Record<string, string> });

export const router = {
  replace: vi.fn(async (_to: unknown) => undefined),
  push: vi.fn(async (_to: unknown) => undefined),
  resolve: vi.fn((to: string) => ({ href: `/base${to}` })),
};

export const useHead = vi.fn((_input: unknown) => undefined);

export function setRoute(next: { path?: string, params?: Record<string, string>, query?: Record<string, string> }) {
  route.path = next.path ?? '/';
  route.fullPath = route.path;
  route.params = next.params ?? {};
  route.query = next.query ?? {};
}

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
  watch,
  nextTick,
  onMounted,
  onBeforeUnmount,
  useId,
  useSlots,
  useHead,
  useRoute: () => route,
  useRouter: () => router,
  createError: (input: { statusCode: number, statusMessage?: string }) => Object.assign(new Error(input.statusMessage), input),
  defineStore,
  navigateTo,
  useRuntimeConfig,
  useNuxtApp: () => ({ ...clients, $i18n: { t, te: (key: string) => known.has(key) } }),
  useI18n: () => ({ t, te: (key: string) => known.has(key), locale, locales, setLocale }),
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
  runtimeConfig.public = { apiUrl: '', appVersion: '' };
  setRoute({});
  useHead.mockClear();
  locale.value = 'en-US';
  setLocale.mockClear();
  router.replace.mockClear();
  router.push.mockClear();
  navigateTo.mockClear();
  useRuntimeConfig.mockClear();
});

afterEach(() => {
  vi.unstubAllGlobals();
});
