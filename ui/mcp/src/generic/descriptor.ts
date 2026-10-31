export type FieldKind = 'text' | 'id' | 'mono' | 'url' | 'badge' | 'date' | 'bool' | 'list' | 'secret';
export type BadgeTone = 'success' | 'warning' | 'error' | 'info' | 'neutral' | 'ghost';

export interface Field {
  key: string;
  label: string;
  kind?: FieldKind;
  badges?: Record<string, { label: string, tone: BadgeTone }>;
  hideEmpty?: boolean;
  format?: (value: unknown) => string;
}

export interface ListDescriptor {
  kind: 'list';
  title: string;
  rows: string;
  key: string | string[];
  columns: Field[];
  empty: string;
  refresh?: { tool: string, args: string[] };
}

export interface DetailDescriptor {
  kind: 'detail' | 'result';
  title: string;
  object?: string;
  fields: Field[];
  note?: (object: Record<string, unknown>) => string;
}

export type Descriptor = ListDescriptor | DetailDescriptor;

// valueAt reads a dotted path from a tool's structuredContent; an empty path is the content itself.
export function valueAt(data: unknown, path = ''): unknown {
  return path.split('.').filter(Boolean).reduce<unknown>(
    (at, key) => (at && typeof at === 'object' ? (at as Record<string, unknown>)[key] : undefined),
    data,
  );
}

// rowsAt reads the list at path, keeping only object rows.
export function rowsAt(data: unknown, path: string): Record<string, unknown>[] {
  const list = valueAt(data, path);
  return Array.isArray(list) ? list.filter((r): r is Record<string, unknown> => typeof r === 'object' && r !== null) : [];
}

// objectAt reads the object at path, or null when there is none.
export function objectAt(data: unknown, path?: string): Record<string, unknown> | null {
  const v = valueAt(data, path);
  return v && typeof v === 'object' && !Array.isArray(v) ? (v as Record<string, unknown>) : null;
}

// isEmpty reports a value protojson would have left out: missing, empty, false or an empty list.
export function isEmpty(value: unknown): boolean {
  return value === undefined || value === null || value === '' || value === false || (Array.isArray(value) && value.length === 0);
}

// formatValue renders a plain value as text: dates as UTC minutes, booleans as Yes or No, lists comma-joined.
export function formatValue(field: Field, value: unknown): string {
  if (field.format) {
    return field.format(value);
  }
  if (isEmpty(value) && field.kind !== 'bool') {
    return '—';
  }
  switch (field.kind) {
    case 'bool':
      return value === true ? 'Yes' : 'No';
    case 'date': {
      const d = new Date(String(value));
      return Number.isNaN(d.getTime()) ? String(value) : `${d.toISOString().slice(0, 16).replace('T', ' ')} UTC`;
    }
    case 'list':
      return Array.isArray(value) ? value.map(String).join(', ') : String(value);
    default:
      return String(value);
  }
}

// badgeFor maps a value onto its badge, or a neutral badge with the raw value.
export function badgeFor(field: Field, value: unknown): { label: string, tone: BadgeTone } {
  const key = String(value ?? '');
  return field.badges?.[key] ?? { label: key || '—', tone: 'ghost' };
}

const snake = (key: string) => key.replace(/[A-Z]/g, c => `_${c.toLowerCase()}`);

// pick copies the named arguments of input that are set, in either JSON or proto spelling, for a refresh that repeats the tool's call.
export function pick(input: Record<string, unknown>, keys: string[]): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const key of keys) {
    const value = input[key] ?? input[snake(key)];
    if (value !== undefined && value !== '') {
      out[key] = value;
    }
  }
  return out;
}

// rowKey is a row's identity from one field or several.
export function rowKey(row: Record<string, unknown>, key: string | string[]): string {
  return (Array.isArray(key) ? key : [key]).map(k => String(valueAt(row, k) ?? '')).join('/');
}
