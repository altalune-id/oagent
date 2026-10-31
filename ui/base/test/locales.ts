import { createParser } from '@intlify/message-compiler';
import { expect } from 'vitest';

// NOTE: @intlify/message-compiler's NodeTypes is a const enum, so its values are not exported at runtime.
const NAMED = 4;

export function paramsOf(message: string): string[] {
  const errors: string[] = [];
  const ast = createParser({ onError: err => errors.push(err.message) }).parse(message);
  expect(errors, `"${message}" does not compile`).toEqual([]);
  const names: string[] = [];
  const walk = (node: unknown): void => {
    if (Array.isArray(node)) {
      node.forEach(walk);
      return;
    }
    if (node && typeof node === 'object') {
      const n = node as { type?: number, key?: string };
      if (n.type === NAMED && typeof n.key === 'string') {
        names.push(n.key);
      }
      Object.values(node).forEach(walk);
    }
  };
  walk(ast);
  return names;
}

export function expectSameKeysAndParams(locales: Record<string, Record<string, string>>) {
  const [first, ...rest] = Object.values(locales);
  for (const other of rest) {
    expect(Object.keys(other!).sort()).toEqual(Object.keys(first!).sort());
    for (const [key, message] of Object.entries(first!)) {
      expect(paramsOf(other![key]!).sort(), key).toEqual(paramsOf(message).sort());
    }
  }
}
