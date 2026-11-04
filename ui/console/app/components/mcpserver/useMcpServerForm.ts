import type { McpAuth, McpServer, ProbeResult } from '~/stores/mcpServers';
import { useToast } from '#base/components/ui/toast';
import { nameTaken } from '~/lib/names/names';
import { isHttpsUrl } from '~/lib/url/url';
import { useMcpServersStore } from '~/stores/mcpServers';
import { serverNameError } from './tools';

export interface ServerErrors {
  name?: string;
  url?: string;
  headerName?: string;
  secret?: string;
  timeout?: string;
}

// useMcpServerForm holds the add and edit form of an MCP server and its tool probe; the name is fixed once saved.
export function useMcpServerForm(probe?: (url: string) => Promise<ProbeResult>) {
  const { t } = useI18n();
  const servers = useMcpServersStore();
  const probeUrl = probe ?? servers.probe;
  const toast = useToast();

  const open = ref(false);
  const editing = ref<McpServer | null>(null);
  const name = ref('');
  const url = ref('');
  const auth = ref<McpAuth>('bearer');
  const headerName = ref('');
  const secret = ref('');
  const replacingSecret = ref(false);
  const trusted = ref(false);
  const timeout = ref(20);
  const errors = ref<ServerErrors>({});
  const probing = ref(false);
  const result = ref<ProbeResult | null>(null);
  const resultUrl = ref('');
  let probeRun = 0;
  let probingUrl = '';

  const probed = computed(() => (resultUrl.value === url.value.trim() ? result.value : null));
  const urlChanged = computed(() => !!editing.value && url.value.trim() !== editing.value.url);
  const needsSecret = computed(() => auth.value !== 'none');
  const secretRequired = computed(() => {
    const was = editing.value;
    return needsSecret.value && (!was || !was.secretSet || was.auth !== auth.value || urlChanged.value);
  });
  const secretShown = computed(() => needsSecret.value && (secretRequired.value || replacingSecret.value));

  watch(url, (next) => {
    if (probing.value && next.trim() !== probingUrl) {
      probeRun++;
      probing.value = false;
    }
  });

  function show(server?: McpServer) {
    probeRun++;
    editing.value = server ?? null;
    name.value = server?.name ?? '';
    url.value = server?.url ?? '';
    auth.value = server?.auth ?? 'bearer';
    headerName.value = server?.headerName ?? '';
    secret.value = '';
    replacingSecret.value = false;
    trusted.value = server?.trusted ?? false;
    timeout.value = server?.timeoutSeconds ?? 20;
    errors.value = {};
    result.value = server ? { kind: 'ok', tools: server.tools } : null;
    resultUrl.value = server?.url ?? '';
    probing.value = false;
    open.value = true;
  }

  function hide() {
    probeRun++;
    probing.value = false;
    secret.value = '';
    open.value = false;
  }

  function validate(): boolean {
    const next: ServerErrors = {};
    const nameKey = editing.value ? '' : serverNameError(name.value.trim());
    if (nameKey) {
      next.name = t(nameKey);
    } else if (!editing.value && nameTaken(servers.items, name.value)) {
      next.name = t('mcpservers.name_taken');
    }
    if (!isHttpsUrl(url.value.trim())) {
      next.url = t('mcpservers.url_https');
    }
    if (auth.value === 'header' && !/^[\w-]+$/.test(headerName.value.trim())) {
      next.headerName = t('mcpservers.header_required');
    }
    if (secretRequired.value && !secret.value.trim()) {
      next.secret = t('mcpservers.secret_required');
    }
    if (!Number.isInteger(timeout.value) || timeout.value < 1 || timeout.value > 120) {
      next.timeout = t('mcpservers.timeout_range');
    }
    errors.value = next;
    return Object.keys(next).length === 0;
  }

  async function loadTools() {
    const target = url.value.trim();
    if (!isHttpsUrl(target)) {
      errors.value = { ...errors.value, url: t('mcpservers.url_https') };
      return;
    }
    const mine = ++probeRun;
    probingUrl = target;
    probing.value = true;
    result.value = null;
    const answer = await probeUrl(target);
    if (mine === probeRun) {
      result.value = answer;
      resultUrl.value = target;
      probing.value = false;
    }
  }

  function submit(): boolean {
    if (!validate()) {
      return false;
    }
    const was = editing.value;
    const found = probed.value;
    servers.save({
      id: was?.id ?? `mcs_${Date.now().toString(36)}`,
      name: was?.name ?? name.value.trim(),
      url: url.value.trim(),
      auth: auth.value,
      headerName: auth.value === 'header' ? headerName.value.trim() : '',
      secretSet: needsSecret.value,
      trusted: trusted.value,
      timeoutSeconds: timeout.value,
      tools: found?.kind === 'ok' ? found.tools : [],
      usedBy: was?.usedBy ?? [],
    });
    toast.push('success', t('mcpservers.saved', { name: was?.name ?? name.value.trim() }));
    secret.value = '';
    open.value = false;
    return true;
  }

  return { open, editing, name, url, auth, headerName, secret, replacingSecret, trusted, timeout, errors, probing, probed, urlChanged, needsSecret, secretRequired, secretShown, show, hide, loadTools, submit };
}
