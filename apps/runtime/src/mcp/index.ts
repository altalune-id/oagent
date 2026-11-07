import type { McpClient } from '@strands-agents/sdk';
import type { Logger } from '../log.ts';
import type { McpServerSpec, McpTool } from './client.ts';

import { authProblem } from './auth.ts';
import { TurnMcpClient } from './client.ts';
import { MAX_SCHEMA_BYTES, sanitizeDescription, sanitizeSchemaValue } from './sanitize.ts';

export type { McpAuth } from './auth.ts';
export type { McpServerSpec, McpTool } from './client.ts';

export type Note = {
  kind: 'mcp_server_unavailable',
  server: string,
};

export type OpenServer = {
  name: string,
  client: McpClient,
};

export interface McpTurn {
  servers: OpenServer[];
  tools: McpTool[];
  notes: Note[];
  promptLines: string[];
  close: () => Promise<void>;
}

function withTimeout<T>(work: Promise<T>, ms: number): Promise<T> {
  return Promise.race([
    work,
    new Promise<never>((_, reject) => {
      setTimeout(() => reject(new Error('mcp: list timed out')), ms).unref();
    }),
  ]);
}

async function openOne(spec: McpServerSpec, log: Logger): Promise<{ client: McpClient, tools: McpTool[] } | string> {
  const problem = authProblem(spec.auth);
  if (problem) {
    return problem;
  }
  const client = new TurnMcpClient(spec, log);
  try {
    await client.connect(false, { signal: AbortSignal.timeout(spec.connectTimeoutMs) });
    if (client.connectionState !== 'connected') {
      await client.disconnect();
      return 'connect_failed';
    }
    const listed = await withTimeout(client.listTools(), spec.connectTimeoutMs);
    const tools: McpTool[] = [];
    for (const tool of listed) {
      const description = sanitizeDescription(tool.description);
      const inputSchema = sanitizeSchemaValue(tool.toolSpec.inputSchema ?? { type: 'object' });
      if (JSON.stringify(inputSchema).length > MAX_SCHEMA_BYTES) {
        continue;
      }
      Object.assign(tool, { description });
      Object.assign(tool.toolSpec, { description, inputSchema });
      tools.push(tool);
    }
    return { client, tools };
  } catch (error) {
    log.warn({ server: spec.name, error }, 'mcp list failed');
    await client.disconnect().catch(() => {});
    return 'list_failed';
  }
}

export async function openMcpServers(specs: McpServerSpec[], log: Logger): Promise<McpTurn> {
  const results = await Promise.all(specs.map(spec => openOne(spec, log)));
  const turn: McpTurn = { servers: [], tools: [], notes: [], promptLines: [], close: async () => {} };
  results.forEach((result, i) => {
    const name = specs[i]!.name;
    if (typeof result === 'string') {
      log.warn({ server: name, code: result }, 'mcp server unavailable');
      turn.notes.push({ kind: 'mcp_server_unavailable', server: name });
      turn.promptLines.push(`Tools from ${name} are unavailable in this turn.`);
      return;
    }
    turn.servers.push({ name, client: result.client });
    turn.tools.push(...result.tools);
  });
  turn.close = async () => {
    await Promise.all(turn.servers.map(s => s.client.disconnect().catch(() => {})));
  };
  return turn;
}
