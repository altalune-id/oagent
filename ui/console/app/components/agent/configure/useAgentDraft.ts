import type { ShownError } from '#base/lib/apperror/message';
import type { AgentConfig, AgentState } from './agentConfig';
import { useErrorMessage } from '#base/composables/useErrorMessage';
import { changes, fromWire, stateOf } from './agentConfig';

export const autosaveDelayMs = 1000;

// useAgentDraft loads an agent's draft and live version, autosaves edits through UpdateAgentDraft and publishes them.
export function useAgentDraft(agentId: string) {
  const { $agentClient: client } = useNuxtApp();
  const errorMessage = useErrorMessage();

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

  const diff = computed(() => changes(config.value, live.value));

  function apply(next: AgentState) {
    state.value = next;
    config.value = next.draft;
    saved = JSON.stringify(next.draft);
  }

  async function load() {
    loadError.value = null;
    try {
      const [got, versions] = await Promise.all([
        client.getAgent({ id: agentId }),
        client.listAgentVersions({ agentId }),
      ]);
      if (got.agent) {
        apply(stateOf(got.agent));
      }
      live.value = versions.versions?.[0]?.config ? fromWire(versions.versions[0].config) : undefined;
    } catch (e) {
      loadError.value = errorMessage(e, 'error.load_agent');
    }
  }

  async function save() {
    clearTimeout(timer);
    timer = undefined;
    const body = JSON.stringify(config.value);
    if (body === saved) {
      dirty.value = false;
      return true;
    }
    saving.value = true;
    saveError.value = null;
    try {
      const res = await client.updateAgentDraft({ id: agentId, config: config.value });
      if (res.agent) {
        state.value = stateOf(res.agent);
        saved = body;
      }
      dirty.value = JSON.stringify(config.value) !== saved;
      return true;
    } catch (e) {
      saveError.value = errorMessage(e, 'error.save_draft');
      return false;
    } finally {
      saving.value = false;
    }
  }

  watch(config, () => {
    if (JSON.stringify(config.value) === saved) {
      return;
    }
    dirty.value = true;
    clearTimeout(timer);
    timer = setTimeout(save, autosaveDelayMs);
  }, { deep: true });

  async function publish() {
    if (!(await save())) {
      return null;
    }
    publishing.value = true;
    publishError.value = null;
    try {
      const res = await client.publishAgent({ id: agentId });
      if (res.agent) {
        apply(stateOf(res.agent));
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

  onBeforeUnmount(() => {
    if (timer) {
      save();
    }
  });

  return { config, state, live, diff, dirty, saving, publishing, loadError, saveError, publishError, load, save, publish };
}
