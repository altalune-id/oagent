export interface TextPart {
  text: string;
  code: boolean;
}

// codeTokens are the scopes, events and headers that body copy names; they render in mono.
export const codeTokens = ['run:create', 'messages:write', 'messages:read', 'message.matched', 'X-Api-Key'];

// splitCode cuts text into plain parts and the code tokens it names, in order.
export function splitCode(text: string, tokens: string[] = codeTokens): TextPart[] {
  const escaped = tokens.map(t => t.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'));
  if (!escaped.length) {
    return [{ text, code: false }];
  }
  return text.split(new RegExp(`(${escaped.join('|')})`)).filter(Boolean).map(part => ({ text: part, code: tokens.includes(part) }));
}
