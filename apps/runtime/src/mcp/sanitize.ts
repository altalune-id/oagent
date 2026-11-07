const MAX_DESCRIPTION_RUNES = 1024;
export const MAX_SCHEMA_BYTES = 16 * 1024;
export const TOOL_NAME = /^[\w-]{1,64}$/;

export function sanitizeDescription(text: string): string {
  const cleaned = text.replace(/[\p{Cc}\p{Cf}\p{Co}]/gu, c => (c === '\n' ? c : ''));
  return [...cleaned].slice(0, MAX_DESCRIPTION_RUNES).join('');
}

export function sanitizeSchemaValue(value: unknown): unknown {
  if (Array.isArray(value)) {
    return value.map(sanitizeSchemaValue);
  }
  if (value && typeof value === 'object') {
    return Object.fromEntries(Object.entries(value).map(([k, v]) => [
      k,
      typeof v === 'string' && (k === 'description' || k === 'title') ? sanitizeDescription(v) : sanitizeSchemaValue(v),
    ]));
  }
  return value;
}
