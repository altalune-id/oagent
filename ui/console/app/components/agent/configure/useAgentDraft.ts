import type { ShownError } from '#base/lib/apperror/message';
import type { AgentConfig, AgentState } from './agentConfig';
import { useToast } from '#base/components/ui/toast';
import { useErrorMessage } from '#base/composables/useErrorMessage';
import { useAgentsStore } from '~/stores/agents';
import { changes, fromWire, stateOf } from './agentConfig';

export const autosaveDelayMs = 1000;

// useAgentDraft loads an agent's draft and live version, autosaves edits through UpdateAgentDraft and publishes them.
export function useAgentDraft(agentId: string) {
  const { $agentClient: client } = useNuxtApp();
  const errorMessage = useErrorMessage();
  const agents = useAgentsStore();
  const toast = useToast();
  const { t } = useI18n();

  const config = ref<AgentConfig>(fromWire());
  const state = ref<AgentState | null>(null);
  const live = ref<AgentConfig | undefined>();
  const loadError = ref<ShownError | null>(null);
  const saveError = ref<ShownError | null>(null);
  const publishError = ref<ShownError | null>(null);
  const saving = ref(false);
  const publishing = ref(false);
  const dirty = ref(false);
  let timer: ReturnType<typeof setTimeout> | undefined;
  let saved = '';
  let leaveBlocked = false;
  let inflight: Promise<boolean> | null = null;

  const diff = computed(() => changes(config.value, live.value));

  function track(next: AgentState) {
    state.value = next;
    agents.setPublishState(agentId, { publishedVersion: next.publishedVersion, unpublishedChanges: next.unpublishedChanges });
  }

  function apply(next: AgentState) {
    track(next);
    config.value = next.draft;
    saved = JSON.stringify(next.draft);
  }

  async function load() {
    loadError.value = null;
    try {
      const [got, versions] = await Promise.all([
        client.getAgent({ id: agentId }),
        client.listAgentVersions({ agentId, limit: 1 }),
      ]);
      if (got.agent) {
        apply(stateOf(got.agent));
      }
      live.value = versions.versions?.[0]?.config ? fromWire(versions.versions[0].config) : undefined;
    } catch (e) {
      loadError.value = errorMessage(e, 'error.load_agent');
    }
  }

  function schedule() {
    clearTimeout(timer);
    timer = setTimeout(save, autosaveDelayMs);
  }

  async function send(body: string) {
    saving.value = true;
    saveError.value = null;
    try {
      const res = await client.updateAgentDraft({ id: agentId, config: JSON.parse(body) });
      if (res.agent) {
        track(stateOf(res.agent));
      }
      saved = body;
      leaveBlocked = false;
      return true;
    } catch (e) {
      saveError.value = errorMessage(e, 'error.save_draft');
      return false;
    } finally {
      saving.value = false;
      dirty.value = JSON.stringify(config.value) !== saved;
      if (dirty.value && !saveError.value && !timer) {
        schedule();
      }
    }
  }

  async function save(): Promise<boolean> {
    if (!state.value) {
      return false;
    }
    clearTimeout(timer);
    timer = undefined;
    let busy = inflight;
    while (busy) {
      await busy;
      busy = inflight;
    }
    const body = JSON.stringify(config.value);
    if (body === saved) {
      dirty.value = false;
      return true;
    }
    const run = send(body).finally(() => {
      inflight = null;
    });
    inflight = run;
    return run;
  }

  watch(config, () => {
    if (JSON.stringify(config.value) === saved) {
      return;
    }
    dirty.value = true;
    schedule();
  }, { deep: true });

  async function publish() {
    if (!state.value || publishing.value) {
      return null;
    }
    publishing.value = true;
    publishError.value = null;
    if (!(await save())) {
      publishError.value = saveError.value;
      publishing.value = false;
      return null;
    }
    try {
      const res = await client.publishAgent({ id: agentId });
      if (res.agent) {
        const next = stateOf(res.agent);
        track(next);
        saved = JSON.stringify(next.draft);
        dirty.value = JSON.stringify(config.value) !== saved;
        if (dirty.value) {
          schedule();
        }
      }
      if (res.version?.config) {
        live.value = fromWire(res.version.config);
      }
      return { created: res.created, version: res.version?.version ?? 0 };
    } catch (e) {
      publishError.value = errorMessage(e, 'error.publish_agent');
      return null;
    } finally {
      publishing.value = false;
    }
  }

  function warnUnload(event: BeforeUnloadEvent) {
    if (dirty.value || saving.value) {
      event.preventDefault();
    }
  }

  async function leaveGuard() {
    if (!dirty.value && !saving.value) {
      return true;
    }
    if (leaveBlocked || await save()) {
      return true;
    }
    leaveBlocked = true;
    toast.push('info', t('configure.leave_first'));
    return false;
  }

  onBeforeUnmount(() => {
    if (!dirty.value) {
      return;
    }
    void save().then((ok) => {
      if (!ok && saveError.value) {
        toast.push('error', saveError.value.message);
      }
    });
  });

  return { config, state, live, diff, dirty, saving, publishing, loadError, saveError, publishError, load, save, publish, warnUnload, leaveGuard };
}
