import commonText from '../../../../password/common.txt?raw';
import policy from '../../../../password/policy.json';

export type Reason = 'too_short' | 'too_long' | 'common' | 'repeated' | 'sequential' | 'contains_email' | 'contains_name';
export interface Hints { email?: string; name?: string; }

export { policy };

const maxUnit = 4;
const common = new Set(commonText.split('\n').map(l => l.trim()).filter(l => l !== '' && !l.startsWith('#')));
const length = (s: string) => [...s].length;

export function isCommon(lower: string): boolean {
  return common.has(lower);
}

export function violations(plain: string, hints: Hints = {}): Reason[] {
  const n = length(plain);
  if (n > policy.maxLength) {
    return ['too_long'];
  }
  const out: Reason[] = [];
  if (n < policy.minLength) {
    out.push('too_short');
  }
  const lower = toLower(plain);
  if (common.has(lower)) {
    out.push('common');
  }
  const runes = [...lower];
  if (repeated(runes)) {
    out.push('repeated');
  }
  if (sequential(runes)) {
    out.push('sequential');
  }
  if (containsEmail(lower, hints.email ?? '')) {
    out.push('contains_email');
  }
  if (containsName(lower, hints.name ?? '')) {
    out.push('contains_name');
  }
  return out;
}

function repeated(r: string[]): boolean {
  for (let unit = 1; unit <= maxUnit && unit < r.length; unit++) {
    if (r.length % unit !== 0) {
      continue;
    }
    if (r.every((c, i) => i < unit || c === r[i - unit])) {
      return true;
    }
  }
  return false;
}

function sequential(r: string[]): boolean {
  if (r.length < 3) {
    return false;
  }
  const code = (c: string) => c.codePointAt(0) ?? 0;
  const step = code(r[1]!) - code(r[0]!);
  if (step !== 1 && step !== -1) {
    return false;
  }
  return r.every((c, i) => i === 0 || code(c) - code(r[i - 1]!) === step);
}

// Code point by code point, with U+0130 as plain i, so it matches Go's strings.ToLower.
function toLower(s: string): string {
  return [...s.replace(/\u0130/g, 'i')].map(c => c.toLowerCase()).join('');
}

function containsEmail(lower: string, email: string): boolean {
  const e = toLower(email.trim());
  if (e === '') {
    return false;
  }
  const local = e.split('@')[0] ?? '';
  return lower.includes(e) || containsPart(lower, local);
}

function containsName(lower: string, name: string): boolean {
  return containsPart(lower, toLower(name));
}

function containsPart(lower: string, hint: string): boolean {
  return hint.split(/[^\p{L}\p{N}]+/u).some(part => length(part) >= policy.hintMinLength && lower.includes(part));
}
