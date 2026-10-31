import type { ShownError } from '#base/lib/apperror/message';
import type { CreatedKey } from './usePersonalKeys';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import scopes from '@shared/scope/scopes.json';
import { useErrorMessage } from '#base/composables/useErrorMessage';
import { useScopeLabel } from '#base/composables/useScopeLabel';
import { mcpSnippet } from '#base/lib/tokens/mcpSnippet';
import { mcpURL } from '~/lib/auth/urls';
import { toAPIKey, useAPIKeysStore } from '~/stores/apiKeys';
import { useAuthStore } from '~/stores/auth';
import { useProjectsStore } from '~/stores/projects';
import { mintableForMe } from './grants';

export const expiryOptions = [
  { days: 7, labelKey: 'apikeys.days_7' },
  { days: 30, labelKey: 'apikeys.days_30' },
  { days: 90, labelKey: 'apikeys.days_90' },
  { days: 365, labelKey: 'apikeys.days_365' },
];

// usePersonalKeyCreate holds the new personal key form for one org and hands the secret to onCreated once.
export function usePersonalKeyCreate(orgId: string, onCreated: (created: CreatedKey) => void) {
  const keys = useAPIKeysStore();
  const auth = useAuthStore();
  const projectsStore = useProjectsStore();
  const scopeLabel = useScopeLabel();
  const errorMessage = useErrorMessage();
  const { $apiKeyClient: client } = useNuxtApp();
  const apiUrl = useRuntimeConfig().public.apiUrl;

  const name = ref('');
  const expiresInDays = ref(30);
  const allProjects = ref(true);
  const projectIds = ref<string[]>([]);
  const scopesPicked = ref<string[]>([]);
  const busy = ref(false);
  const error = ref<ShownError | null>(null);

  const canSubmit = computed(() => !busy.value && !!name.value.trim());
  const mintable = computed(() => mintableForMe(scopes, auth.me?.permissions ?? []).map(s => ({ name: s.name, label: scopeLabel(s.name) })));
  const projects = computed(() => projectsStore.forOrg(orgId).map(p => ({ id: p.id, name: p.name })));

  function reset() {
    name.value = '';
    expiresInDays.value = 30;
    allProjects.value = true;
    projectIds.value = [];
    scopesPicked.value = [];
  }

  async function submit() {
    if (!canSubmit.value) {
      return;
    }
    busy.value = true;
    error.value = null;
    try {
      const res = await client.createAPIKey({
        orgId,
        kind: 'personal',
        name: name.value.trim(),
        scopes: scopesPicked.value,
        allProjects: allProjects.value,
        projectIds: allProjects.value ? [] : projectIds.value,
        expiresAt: timestampFromDate(new Date(Date.now() + expiresInDays.value * 86_400_000)),
      });
      if (res.key) {
        keys.add(toAPIKey(res.key));
      }
      onCreated({ plaintext: res.plaintext, snippet: mcpSnippet('oagent', mcpURL(apiUrl, window.location.origin), res.plaintext) });
      reset();
    } catch (e) {
      error.value = errorMessage(e, 'error.create_apikey');
    } finally {
      busy.value = false;
    }
  }

  return { name, expiresInDays, expiryOptions, allProjects, projectIds, scopes: scopesPicked, projects, mintable, busy, error, canSubmit, submit };
}
