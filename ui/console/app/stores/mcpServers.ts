import type { ShownError } from '#base/lib/apperror/message';
import { sampleMcpServers, sampleProbe } from '~/preview/samples';

export type McpAuth = 'none' | 'bearer' | 'header';

export type ToolHint = 'read_only' | 'changes' | 'destructive';

export interface McpToolArg {
  name: string;
  type: 'boolean' | 'string' | 'number';
}

export interface McpTool {
  name: string;
  description: string;
  hint: ToolHint;
  args: McpToolArg[];
}

export interface McpServer {
  id: string;
  name: string;
  url: string;
  auth: McpAuth;
  headerName: string;
  secretSet: boolean;
  trusted: boolean;
  timeoutSeconds: number;
  tools: McpTool[];
  usedBy: string[];
}

export type ProbeResult
  = | { kind: 'ok', tools: McpTool[] }
    | { kind: 'unreachable' | 'auth' | 'private' | 'empty' };

export const useMcpServersStore = defineStore('mcpServers', () => {
  const items = ref<McpServer[]>([]);
  const loading = ref(false);
  const error = ref<ShownError | null>(null);
  let loadedFor = '';

  async function load(projectId: string) {
    loading.value = true;
    error.value = null;
    items.value = await Promise.resolve(sampleMcpServers());
    loadedFor = projectId;
    loading.value = false;
  }

  function ensureLoaded(projectId: string) {
    return loadedFor === projectId ? Promise.resolve() : load(projectId);
  }

  function save(server: McpServer) {
    const known = items.value.some(s => s.id === server.id);
    items.value = known ? items.value.map(s => (s.id === server.id ? server : s)) : [...items.value, server];
  }

  function remove(id: string) {
    items.value = items.value.filter(s => s.id !== id);
  }

  function probe(url: string): Promise<ProbeResult> {
    return sampleProbe(url);
  }

  return { items, probe, loading, error, load, ensureLoaded, save, remove };
});
