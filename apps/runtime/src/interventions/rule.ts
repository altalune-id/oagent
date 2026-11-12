import type { ApprovalSpec } from '../contract/index.ts';

import { canonicalJson } from '../contract/index.ts';

export type Decision = 'free' | 'ask';

type JsonType = 'null' | 'boolean' | 'number' | 'integer' | 'string' | 'array' | 'object';

function propertiesOf(schema: unknown): Record<string, unknown> | undefined {
  const props = (schema as { properties?: unknown } | undefined)?.properties;
  return props !== null && typeof props === 'object' && !Array.isArray(props) ? props as Record<string, unknown> : undefined;
}

export function orderedKeys(input: Record<string, unknown>, schema: unknown): string[] {
  const order = Object.keys(propertiesOf(schema) ?? {});
  const keys = Object.keys(input);
  return [...order.filter(k => keys.includes(k)), ...keys.filter(k => !order.includes(k)).sort()];
}

function jsonType(value: unknown): JsonType {
  if (value === null) {
    return 'null';
  }
  if (Array.isArray(value)) {
    return 'array';
  }
  return typeof value as JsonType;
}

function fitsType(value: unknown, declared: unknown): boolean {
  if (declared === undefined) {
    return true;
  }
  const allowed = (Array.isArray(declared) ? declared : [declared]) as JsonType[];
  const actual = jsonType(value);
  if (actual === 'number' && Number.isInteger(value) && allowed.includes('integer')) {
    return true;
  }
  return allowed.includes(actual);
}

export function evaluate(rule: ApprovalSpec, schema: unknown, args: Record<string, unknown>): Decision {
  if (rule.mode === 'never') {
    return 'free';
  }
  if (rule.mode === 'always') {
    return 'ask';
  }
  const props = propertiesOf(schema);
  if (props === undefined) {
    return 'ask';
  }
  for (const cond of rule.freeWhen) {
    if (cond.arg === '' || !Object.hasOwn(props, cond.arg)) {
      return 'ask';
    }
    const declared = (props[cond.arg] as { type?: unknown } | undefined)?.type;
    if (!cond.values.every(v => fitsType(v, declared))) {
      return 'ask';
    }
    if (!Object.hasOwn(args, cond.arg)) {
      if (cond.absent !== true) {
        return 'ask';
      }
      continue;
    }
    const got = canonicalJson(args[cond.arg]);
    if (!cond.values.some(v => canonicalJson(v) === got)) {
      return 'ask';
    }
  }
  return 'free';
}
