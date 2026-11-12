import type { McpCallToolOptions, McpToolFilterCallback } from '@strands-agents/sdk';
import type { Logger } from '../log.ts';
import type { McpAuth } from './auth.ts';
import { StreamableHTTPClientTransport } from '@modelcontextprotocol/client';

import { McpClient } from '@strands-agents/sdk';
import { guardedFetch } from '../egress/index.ts';
import { credential } from './auth.ts';
import { TOOL_NAME } from './sanitize.ts';

export type McpTool = Parameters<McpToolFilterCallback>[0];

export type McpServerSpec = {
  name: string,
  url: string,
  auth: McpAuth,
  allowedTools?: string[],
  tools?: { name: string }[],
  connectTimeoutMs: number,
  callTimeoutMs: number,
};

export function originFetch(serverUrl: string, auth: McpAuth, fetchImpl: typeof fetch = globalThis.fetch): typeof fetch {
  return guardedFetch(serverUrl, credential(auth), fetchImpl);
}

export class TurnMcpClient extends McpClient {
  private readonly callTimeoutMs: number;

  constructor(spec: McpServerSpec, log: Logger) {
    super({
      transport: new StreamableHTTPClientTransport(new URL(spec.url), { fetch: originFetch(spec.url, spec.auth) }),
      applicationName: spec.name,
      logHandler: () => log.info({ server: spec.name }, 'mcp server log'),
      prefix: `${spec.name}_`,
      continueOnError: true,
      toolFilters: {
        allowed: spec.tools?.map(t => t.name) ?? spec.allowedTools ?? [],
        rejected: [(tool: McpTool) => !TOOL_NAME.test(tool.name)],
      },
    });
    this.callTimeoutMs = spec.callTimeoutMs;
  }

  override async connect(reconnect?: boolean, options?: { signal?: AbortSignal }): Promise<void> {
    if (reconnect) {
      throw new Error('mcp: a turn client never reconnects; open a new one');
    }
    if (!this.client.transport) {
      this.client.setVersionNegotiation({ mode: 'legacy' });
    }
    await super.connect(false, options);
  }

  override callTool(tool: McpTool, args: Parameters<McpClient['callTool']>[1], options?: McpCallToolOptions): ReturnType<McpClient['callTool']> {
    return super.callTool(tool, args, { ...options, timeoutMs: this.callTimeoutMs });
  }
}
