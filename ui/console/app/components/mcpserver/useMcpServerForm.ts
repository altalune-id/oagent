import type { ShownError } from '#base/lib/apperror/message';
import type { McpAuth, McpServer, ServerInput } from '~/stores/mcpServers';
import { useToast } from '#base/components/ui/toast';
import { useValidation } from '#base/composables/useValidation';
import { CreateMcpServerRequestSchema } from '#gen/mcpserver/v1/mcpserver_pb';
import { createRequest, useMcpServersStore } from '~/stores/mcpServers';
import { headerNameError } from './tools';

export interface ServerErrors {
  name?: string;
  url?: string;
  headerName?: string;
  secret?: string;
  timeout?: string;
}

// useMcpServerForm holds the add and edit form of an MCP server; the name is fixed once saved and the secret is write-only, never kept after a save.
export function useMcpServerForm(orgId: string, projectId: string) {
  const { t } = useI18n();
  const servers = useMcpServersStore();
  const toast = useToast();
  const validation = useValidation(CreateMcpServerRequestSchema, {
    fields: ['name', 'url', 'auth_header', 'auth_value', 'timeout_ms'],
    codes: { MCS002: 'name', MCS003: 'url', MCS006: 'auth_value', MCS006_header_name: 'auth_header' },
  });

  const open = ref(false);
  const editing = ref<McpServer | null>(null);
  const name = ref('');
  const url = ref('');
  const auth = ref<McpAuth>('bearer');
  const headerName = ref('');
  const secret = ref('');
  const replacingSecret = ref(false);
  const trusted = ref(false);
  const timeout = ref<number | ''>('');
  const defaults = computed(() => servers.defaults);
  const local = ref<ServerErrors>({});
  const formError = ref<ShownError | null>(null);
  const busy = ref(false);

  const urlChanged = computed(() => !!editing.value && url.value.trim() !== editing.value.url);
  const needsSecret = computed(() => auth.value !== 'none');
  const secretRequired = computed(() => {
    const was = editing.value;
    return needsSecret.value && (!was || !was.secretSet || was.auth !== auth.value || urlChanged.value);
  });
  const secretShown = computed(() => needsSecret.value && (secretRequired.value || replacingSecret.value));
  const errors = computed<ServerErrors>(() => {
    const v = validation.errors.value;
    const out: ServerErrors = { name: v.name, url: v.url, headerName: v.auth_header, secret: v.auth_value, timeout: v.timeout_ms, ...local.value };
    return Object.fromEntries(Object.entries(out).filter(([, msg]) => msg)) as ServerErrors;
  });

  function show(server?: McpServer) {
    editing.value = server ?? null;
    name.value = server?.name ?? '';
    url.value = server?.url ?? '';
    auth.value = server?.auth ?? 'bearer';
    headerName.value = server?.headerName ?? '';
    secret.value = '';
    replacingSecret.value = false;
    trusted.value = server?.trusted ?? false;
    timeout.value = server ? server.timeoutMs / 1000 : '';
    local.value = {};
    formError.value = null;
    validation.clear();
    open.value = true;
  }

  function hide() {
    if (busy.value) {
      return;
    }
    secret.value = '';
    open.value = false;
  }

  function input(): ServerInput | null {
    const next: ServerErrors = {};
    const headerKey = auth.value === 'header' ? headerNameError(headerName.value.trim()) : '';
    if (headerKey) {
      next.headerName = t(headerKey, { name: headerName.value.trim() });
    }
    if (secretRequired.value && !secret.value.trim()) {
      next.secret = t('mcpservers.secret_required');
    }
    const ms = Math.round(Number(timeout.value) * 1000);
    local.value = next;
    const out: ServerInput = { name: editing.value?.name ?? name.value, url: url.value, auth: auth.value, headerName: headerName.value, secret: secret.value, trusted: trusted.value, timeoutMs: ms };
    const storedSecret = 'kept';
    const shaped = validation.check(createRequest(orgId, projectId, { ...out, secret: out.secret.trim() || storedSecret }));
    formError.value = validation.formError.value;
    return shaped && Object.keys(next).length === 0 ? out : null;
  }

  async function submit(): Promise<boolean> {
    if (busy.value) {
      return false;
    }
    const req = input();
    if (!req) {
      return false;
    }
    busy.value = true;
    try {
      const was = editing.value;
      const saved = was ? await servers.update(was.id, req) : await servers.add(orgId, projectId, req);
      toast.push('success', t('mcpservers.saved', { name: saved.name }));
      secret.value = '';
      open.value = false;
      void servers.loadTools(saved.id);
      return true;
    } catch (e) {
      local.value = {};
      formError.value = validation.fromServer(e, 'error.save_mcp_server');
      return false;
    } finally {
      busy.value = false;
    }
  }

  return { open, editing, name, url, auth, headerName, secret, replacingSecret, trusted, timeout, defaults, errors, formError, busy, urlChanged, needsSecret, secretRequired, secretShown, show, hide, submit };
}
