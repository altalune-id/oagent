import { Buffer } from 'node:buffer';
import { createHash } from 'node:crypto';

const LINE_SEPARATOR = String.fromCharCode(0x2028);
const PARAGRAPH_SEPARATOR = String.fromCharCode(0x2029);

function text(s: string): string {
  return JSON.stringify(s.toWellFormed()).replaceAll(LINE_SEPARATOR, '\\u2028').replaceAll(PARAGRAPH_SEPARATOR, '\\u2029');
}

function number(n: number): string {
  if (!Number.isFinite(n)) {
    throw new TypeError('canonical: a number outside the JSON range');
  }
  return Object.is(n, -0) ? '0' : JSON.stringify(n);
}

function byCodePoint(a: string, b: string): number {
  return Buffer.compare(Buffer.from(a, 'utf8'), Buffer.from(b, 'utf8'));
}

// canonicalJson writes value the way the Go canonjson package does, so both sides hash the same tool arguments alike.
export function canonicalJson(value: unknown): string {
  if (value === null) {
    return 'null';
  }
  switch (typeof value) {
    case 'boolean':
      return value ? 'true' : 'false';
    case 'number':
      return number(value);
    case 'string':
      return text(value);
    case 'object':
      break;
    default:
      throw new TypeError(`canonical: a ${typeof value} is not JSON`);
  }
  if (Array.isArray(value)) {
    return `[${value.map(canonicalJson).join(',')}]`;
  }
  const fields = new Map<string, unknown>();
  for (const [k, v] of Object.entries(value)) {
    fields.set(k.toWellFormed(), v);
  }
  const keys = [...fields.keys()].sort(byCodePoint);
  return `{${keys.map(k => `${text(k)}:${canonicalJson(fields.get(k))}`).join(',')}}`;
}

// argsHash is the lowercase hex SHA-256 of a value's canonical JSON.
export function argsHash(value: unknown): string {
  return createHash('sha256').update(canonicalJson(value), 'utf8').digest('hex');
}
