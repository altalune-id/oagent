export type SeenRequest = {
  url: string,
  headers: Headers,
  redirect: RequestRedirect | undefined,
  body: Record<string, unknown>,
};

function sse(events: Record<string, unknown>[]): string {
  return events.map(e => `event: ${String(e.type)}\ndata: ${JSON.stringify(e)}\n\n`).join('');
}

export function messagesStream(text: string): Response {
  const body = sse([
    { type: 'message_start', message: { id: 'msg_1', type: 'message', role: 'assistant', model: 'claude-sonnet-5-5', content: [], stop_reason: null, stop_sequence: null, usage: { input_tokens: 120, output_tokens: 1, cache_read_input_tokens: 80, cache_creation_input_tokens: 40 } } },
    { type: 'content_block_start', index: 0, content_block: { type: 'text', text: '' } },
    { type: 'content_block_delta', index: 0, delta: { type: 'text_delta', text } },
    { type: 'content_block_stop', index: 0 },
    { type: 'message_delta', delta: { stop_reason: 'end_turn', stop_sequence: null }, usage: { output_tokens: 9 } },
    { type: 'message_stop' },
  ]);
  return new Response(body, { status: 200, headers: { 'content-type': 'text/event-stream' } });
}

export function apiError(status: number, message: string): Response {
  const type = status === 429 ? 'rate_limit_error' : 'invalid_request_error';
  return new Response(JSON.stringify({ type: 'error', error: { type, message } }), { status, headers: { 'content-type': 'application/json' } });
}

export function fakeAnthropic(responses: (() => Response)[]): { fetch: typeof fetch, seen: SeenRequest[] } {
  const seen: SeenRequest[] = [];
  const fetchImpl = (async (input: string | URL | Request, init: RequestInit = {}) => {
    seen.push({
      url: String(input instanceof Request ? input.url : input),
      headers: new Headers(init.headers),
      redirect: init.redirect,
      body: JSON.parse(String(init.body)) as Record<string, unknown>,
    });
    const next = responses[seen.length - 1];
    if (!next) {
      throw new Error(`fakeAnthropic: no response for request ${seen.length}`);
    }
    return next();
  }) as typeof fetch;
  return { fetch: fetchImpl, seen };
}
