import type { ServerResponse } from 'node:http';

export function reply(res: ServerResponse, status: number, body?: Record<string, unknown>): void {
  if (body === undefined) {
    res.writeHead(status).end();
    return;
  }
  res.writeHead(status, { 'content-type': 'application/json' }).end(JSON.stringify(body));
}
