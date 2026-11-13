export interface Copy {
  key: string;
  params: Record<string, string | number>;
  count?: number;
}

// clearCopy is the confirm title and body for clearing what agents remember about a person in a store, naming one agent when the list is filtered.
export function clearCopy(name: string, n: number, store: string, agent?: string): { title: Copy, body: Copy } {
  return agent
    ? { title: { key: 'endusers.memories_clear_agent_title', params: { n, agent, name, store }, count: n }, body: { key: 'endusers.memories_clear_agent_body', params: { agent } } }
    : { title: { key: 'endusers.memories_clear_title', params: { n, name, store }, count: n }, body: { key: 'endusers.memories_clear_body', params: {} } };
}

// clip shortens text to at most max characters, cutting at the last word boundary and ending with an ellipsis.
export function clip(text: string, max: number): string {
  if (text.length <= max) {
    return text;
  }
  const cut = text.slice(0, max - 1);
  const space = cut.lastIndexOf(' ');
  return `${space > 0 ? cut.slice(0, space) : cut}…`;
}
