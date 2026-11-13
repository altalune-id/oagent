import type { JsonObject } from '@bufbuild/protobuf';
import type { ShownError } from '#base/lib/apperror/message';
import type { CreateMcpServerRequest, McpServer as McpServerPB, McpServerTool, UpdateMcpServerRequest } from '#gen/mcpserver/v1/mcpserver_pb';
import { create } from '@bufbuild/protobuf';
import { useErrorMessage } from '#base/composables/useErrorMessage';
import { errorCode } from '#base/lib/apperror/message';
import { CreateMcpServerRequestSchema, McpServerAuthKind, UpdateMcpServerRequestSchema } from '#gen/mcpserver/v1/mcpserver_pb';

export const secretRefusedCode = 'MCS008';

export type McpAuth = 'none' | 'bearer' | 'header';

export interface TimeoutDefaults {
  timeoutMs: number;
  minTimeoutMs: number;
  maxTimeoutMs: number;
}

export type ToolHint = 'read_only' | 'changes' | 'destructive';

export interface McpToolArg {
  name: string;
  type: 'boolean' | 'string' | 'number' | 'integer' | 'other';
}

export interface McpTool {
  name: string;
  description: string;
  hint: ToolHint;
  args: McpToolArg[];
  grantable: boolean;
  requireApproval: boolean;
}

export interface McpServer {
  id: string;
  orgId: string;
  projectId: string;
  name: string;
  url: string;
  auth: McpAuth;
  headerName: string;
  secretSet: boolean;
  trusted: boolean;
  timeoutMs: number;
  tools: McpTool[];
}

export interface ServerInput {
  name: string;
  url: string;
  auth: McpAuth;
  headerName: string;
  secret: string;
  trusted: boolean;
  timeoutMs: number;
}

const authKinds: Record<McpAuth, McpServerAuthKind> = {
  none: McpServerAuthKind.NONE,
  bearer: McpServerAuthKind.BEARER,
  header: McpServerAuthKind.HEADER,
};

function toAuth(kind: McpServerAuthKind): McpAuth {
  if (kind === McpServerAuthKind.HEADER) {
    return 'header';
  }
  return kind === McpServerAuthKind.BEARER ? 'bearer' : 'none';
}

function argType(schema: unknown): McpToolArg['type'] {
  const type = schema && typeof schema === 'object' ? (schema as JsonObject).type : undefined;
  return type === 'boolean' || type === 'string' || type === 'number' || type === 'integer' ? type : 'other';
}

export function toMcpTool(t: McpServerTool): McpTool {
  const props = t.inputSchema?.properties;
  const args = props && typeof props === 'object' && !Array.isArray(props) ? Object.entries(props).map(([name, s]) => ({ name, type: argType(s) })) : [];
  return { name: t.name, description: t.description, hint: t.readOnly ? 'read_only' : 'changes', args, grantable: t.grantable, requireApproval: t.requireApproval };
}

export function toMcpServer(s: McpServerPB): McpServer {
  return {
    id: s.id,
    orgId: s.orgId,
    projectId: s.projectId,
    name: s.name,
    url: s.url,
    auth: toAuth(s.authKind),
    headerName: s.authHeader,
    secretSet: s.authSet,
    trusted: s.trusted,
    timeoutMs: s.timeoutMs,
    tools: [],
  };
}

function authFields(input: ServerInput) {
  return { url: input.url.trim(), authKind: authKinds[input.auth], authHeader: input.auth === 'header' ? input.headerName.trim() : '', timeoutMs: input.timeoutMs, trusted: input.trusted };
}

// createRequest builds the create call for a server; the secret is trimmed and sent only for bearer or header auth.
export function createRequest(orgId: string, projectId: string, input: ServerInput): CreateMcpServerRequest {
  return create(CreateMcpServerRequestSchema, { orgId, projectId, name: input.name.trim(), ...authFields(input), authValue: input.auth === 'none' ? '' : input.secret.trim() });
}

// updateRequest builds the update call for a server; the name never changes and an empty secret keeps the stored one.
export function updateRequest(id: string, input: ServerInput): UpdateMcpServerRequest {
  const authValue = input.auth === 'none' ? '' : input.secret.trim();
  return create(UpdateMcpServerRequestSchema, { id, ...authFields(input), ...(authValue ? { authValue } : {}) });
}

export const useMcpServersStore = defineStore('mcpServers', () => {
  const items = ref<McpServer[]>([]);
  const loading = ref(false);
  const error = ref<ShownError | null>(null);
  const toolsLoading = ref<Record<string, boolean>>({});
  const toolsError = ref<Record<string, ShownError | null>>({});
  const toolsRefused = ref<Record<string, boolean>>({});
  const defaults = ref<TimeoutDefaults | null>(null);

  const { $mcpServerClient: client } = useNuxtApp();
  const errorMessage = useErrorMessage();
  const toolsLoaded = new Set<string>();
  let loadedFor = '';
  let inflight: Promise<void> | null = null;
  let inflightFor = '';
  let latest = 0;

  function put(server: McpServer) {
    const known = items.value.some(s => s.id === server.id);
    items.value = known ? items.value.map(s => (s.id === server.id ? server : s)) : [...items.value, server];
    return server;
  }

  async function load(projectId: string) {
    const mine = ++latest;
    loading.value = true;
    error.value = null;
    try {
      const res = await client.listMcpServers({ projectId });
      if (mine !== latest) {
        return;
      }
      toolsLoaded.clear();
      items.value = (res.mcpServers ?? []).map(s => toMcpServer(s));
      defaults.value = res.defaults ? { timeoutMs: res.defaults.timeoutMs, minTimeoutMs: res.defaults.minTimeoutMs, maxTimeoutMs: res.defaults.maxTimeoutMs } : null;
      loadedFor = projectId;
    } catch (e) {
      if (mine === latest) {
        error.value = errorMessage(e, 'error.load_mcp_servers');
      }
    } finally {
      if (mine === latest) {
        loading.value = false;
      }
    }
  }

  function ensureLoaded(projectId: string) {
    if (!projectId || loadedFor === projectId) {
      return Promise.resolve();
    }
    if (inflight && inflightFor === projectId) {
      return inflight;
    }
    inflightFor = projectId;
    const run = load(projectId).finally(() => {
      if (inflight === run) {
        inflight = null;
      }
    });
    inflight = run;
    return run;
  }

  async function loadTools(id: string) {
    toolsLoading.value = { ...toolsLoading.value, [id]: true };
    toolsError.value = { ...toolsError.value, [id]: null };
    toolsRefused.value = { ...toolsRefused.value, [id]: false };
    try {
      const res = await client.listMcpServerTools({ id });
      const server = items.value.find(s => s.id === id);
      if (server) {
        put({ ...server, tools: (res.tools ?? []).map(toMcpTool) });
      }
      toolsLoaded.add(id);
    } catch (e) {
      toolsError.value = { ...toolsError.value, [id]: errorMessage(e, 'error.load_mcp_tools') };
      toolsRefused.value = { ...toolsRefused.value, [id]: errorCode(e) === secretRefusedCode };
    } finally {
      toolsLoading.value = { ...toolsLoading.value, [id]: false };
    }
  }

  function ensureTools(id: string) {
    return toolsLoaded.has(id) || toolsLoading.value[id] ? Promise.resolve() : loadTools(id);
  }

  async function add(orgId: string, projectId: string, input: ServerInput) {
    const res = await client.createMcpServer(createRequest(orgId, projectId, input));
    return put(toMcpServer(res.mcpServer!));
  }

  async function update(id: string, input: ServerInput) {
    const res = await client.updateMcpServer(updateRequest(id, input));
    toolsLoaded.delete(id);
    return put(toMcpServer(res.mcpServer!));
  }

  async function remove(id: string) {
    await client.deleteMcpServer({ id });
    toolsLoaded.delete(id);
    items.value = items.value.filter(s => s.id !== id);
  }

  return { items, loading, error, defaults, toolsLoading, toolsError, toolsRefused, load, ensureLoaded, loadTools, ensureTools, add, update, remove };
});
