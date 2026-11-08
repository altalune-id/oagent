export const eventTypes = new Set([
  'RUN_STARTED',
  'RUN_FINISHED',
  'RUN_ERROR',
  'TEXT_MESSAGE_START',
  'TEXT_MESSAGE_CONTENT',
  'TEXT_MESSAGE_END',
  'TOOL_CALL_START',
  'TOOL_CALL_ARGS',
  'TOOL_CALL_END',
  'TOOL_CALL_RESULT',
]);

export interface AgUiEvent {
  type: string;
  [field: string]: unknown;
}

function parse(data: string[]): AgUiEvent | undefined {
  if (data.length === 0) {
    return undefined;
  }
  try {
    const event: unknown = JSON.parse(data.join('\n'));
    if (event && typeof event === 'object' && eventTypes.has((event as AgUiEvent).type)) {
      return event as AgUiEvent;
    }
  } catch {
    return undefined;
  }
  return undefined;
}

// readEvents yields the allow-listed AG-UI events of an SSE body, skipping comments, unknown types and broken frames.
export async function* readEvents(body: ReadableStream<Uint8Array>): AsyncGenerator<AgUiEvent> {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  let data: string[] = [];
  try {
    for (;;) {
      const { value, done } = await reader.read();
      buffer += decoder.decode(value, { stream: !done });
      const lines = buffer.split(/\r?\n/);
      buffer = done ? '' : lines.pop()!;
      for (const line of lines) {
        if (line === '') {
          const event = parse(data);
          data = [];
          if (event) {
            yield event;
          }
          continue;
        }
        const colon = line.indexOf(':');
        if (colon === 0) {
          continue;
        }
        const field = colon < 0 ? line : line.slice(0, colon);
        const value = colon < 0 ? '' : line.slice(colon + 1).replace(/^ /, '');
        if (field === 'data') {
          data.push(value);
        }
      }
      if (done) {
        return;
      }
    }
  } finally {
    reader.releaseLock();
  }
}
