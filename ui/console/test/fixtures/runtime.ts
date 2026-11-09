import type { McpServer } from '~/stores/mcpServers';
import type { ModelProvider } from '~/stores/modelProviders';
import { create } from '@bufbuild/protobuf';
import { vi } from 'vitest';
import { McpServerAuthKind, McpServerSchema, McpServerToolSchema } from '#gen/mcpserver/v1/mcpserver_pb';
import { ModelProviderKind, ModelProviderSchema } from '#gen/modelprovider/v1/modelprovider_pb';
import { setClient } from '#test/nuxt-globals';
import { toMcpServer, toMcpTool } from '~/stores/mcpServers';
import { toModelProvider } from '~/stores/modelProviders';

const orgId = 'V3StGXR8Z5jdHj6BmyT2a';
const projectId = 'x6MH9WPWjktMwCBKHy4pF';
const usd = (dollars: number) => BigInt(Math.round(dollars * 1_000_000));
const prices = (input: number, output: number, cacheRead = 0) => ({ inputMicros: usd(input), outputMicros: usd(output), cacheReadMicros: usd(cacheRead), cacheWriteMicros: 0n });

export function providerPBs() {
  return [
    create(ModelProviderSchema, {
      id: 'Pv6tQm2ZxR8wKp3VbNc4K',
      orgId,
      projectId,
      name: 'OpenRouter',
      kind: ModelProviderKind.OPENAI_COMPATIBLE,
      baseUrl: 'https://openrouter.ai/api/v1',
      apiKeySet: true,
      models: [
        { id: 'openai/gpt-4.1-mini', vision: true, prices: prices(0.4, 1.6, 0.1) },
        { id: 'google/gemini-2.5-flash', vision: true, prices: prices(0.3, 2.5) },
      ],
    }),
    create(ModelProviderSchema, { id: 'Pv9pHs4YvT7kMq7WcXe2B', orgId, projectId, name: 'Groq', kind: ModelProviderKind.OPENAI_COMPATIBLE, baseUrl: 'https://api.groq.com/openai/v1', apiKeySet: true, models: [{ id: 'llama-3.3-70b-versatile', vision: false }] }),
    create(ModelProviderSchema, { id: 'Pv4cLd8NwQ2xTr6YbMa9F', orgId, projectId, name: 'Anthropic', kind: ModelProviderKind.ANTHROPIC, baseUrl: 'https://api.anthropic.com', apiKeySet: true, models: [{ id: 'claude-sonnet-5-5', vision: true, prices: prices(3, 15, 0.3) }] }),
  ];
}

export function serverPBs() {
  return [
    create(McpServerSchema, { id: 'Ms2bVn7QxK4pRt9KwZc3H', orgId, projectId, name: 'yasaku', url: 'https://yasaku.altalune.id/mcp', authKind: McpServerAuthKind.BEARER, authSet: true, trusted: true, timeoutMs: 20_000 }),
    create(McpServerSchema, { id: 'Ms8kWq3NvT6yHp7ZxR5cJ', orgId, projectId, name: 'tagihan', url: 'https://tagihan.example.id/mcp', authKind: McpServerAuthKind.HEADER, authHeader: 'X-Api-Key', authSet: true, trusted: false, timeoutMs: 10_000 }),
  ];
}

const trusted = new Set(['yasaku']);

function tool(server: string, name: string, description: string, readOnly: boolean, args: Record<string, string> = {}) {
  const properties = Object.fromEntries(Object.entries(args).map(([arg, type]) => [arg, { type }]));
  return create(McpServerToolSchema, { name, description, readOnly, requireApproval: !(readOnly && trusted.has(server)), grantable: /^[\w-]{1,64}$/.test(`${server}__${name}`), inputSchema: { type: 'object', properties } });
}

const money = { confirm: 'boolean', amount: 'number' };

export function toolPBs(): Record<string, ReturnType<typeof tool>[]> {
  return {
    Ms2bVn7QxK4pRt9KwZc3H: [
      tool('yasaku', 'record_expense', 'Record an expense in a wallet', false, money),
      tool('yasaku', 'record_income', 'Record income in a wallet', false, money),
      tool('yasaku', 'record_transfer', 'Move money between wallets', false, money),
      tool('yasaku', 'record_batch', 'Record several transactions at once', false, { confirm: 'boolean' }),
      tool('yasaku', 'delete_tx', 'Delete a transaction', false, { id: 'string' }),
      tool('yasaku', 'list_wallets', 'List wallets and balances', true),
      tool('yasaku', 'list_categories', 'List categories', true),
      tool('yasaku', 'search_tx', 'Search transactions', true, { query: 'string' }),
      tool('yasaku', 'list_recent_tx', 'List the latest transactions', true),
      tool('yasaku', 'period_report', 'Totals per category for a period', true),
      tool('yasaku', 'cashflow_report', 'Money in and out for a period', true),
      tool('yasaku', 'close_period', 'Close a month so it can no longer change', false),
      tool('yasaku', 'export_all_transactions_as_spreadsheet_for_accountant_review', 'Export every transaction as a spreadsheet', true),
    ],
    Ms8kWq3NvT6yHp7ZxR5cJ: [
      tool('tagihan', 'list_bills', 'List unpaid bills', true),
      tool('tagihan', 'mark_paid', 'Mark a bill as paid', false, { id: 'string' }),
    ],
  };
}

// providers returns the fixture providers as the store holds them.
export function providers(): ModelProvider[] {
  return providerPBs().map(toModelProvider);
}

// servers returns the fixture servers as the store holds them once their tools are listed.
export function servers(): McpServer[] {
  const tools = toolPBs();
  return serverPBs().map(s => ({ ...toMcpServer(s), tools: (tools[s.id] ?? []).map(toMcpTool) }));
}

// fakeRuntimeClients answers the model provider and MCP server RPCs with the fixtures.
export function fakeRuntimeClients() {
  const provider = {
    listModelProviders: vi.fn().mockResolvedValue({ modelProviders: providerPBs() }),
    createModelProvider: vi.fn(),
    updateModelProvider: vi.fn(),
    deleteModelProvider: vi.fn().mockResolvedValue({}),
  };
  const server = {
    listMcpServers: vi.fn().mockResolvedValue({ mcpServers: serverPBs() }),
    listMcpServerTools: vi.fn(async ({ id }: { id: string }) => ({ tools: toolPBs()[id] ?? [] })),
    createMcpServer: vi.fn(),
    updateMcpServer: vi.fn(),
    deleteMcpServer: vi.fn().mockResolvedValue({}),
  };
  setClient('$modelProviderClient', provider);
  setClient('$mcpServerClient', server);
  return { provider, server };
}
