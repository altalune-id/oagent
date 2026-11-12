const ENTITY: Record<string, string> = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' };

export function escapeToolOutput(text: string): string {
  return text.replace(/[&<>]/g, c => ENTITY[c]!);
}

export function attr(value: string): string {
  return value.replace(/[&<>"]/g, c => ENTITY[c]!);
}
