import type { ShownError } from '#base/lib/apperror/message';
import type { PersonalKeyView } from './PersonalKeys.vue';
import { useDates } from '#base/composables/useDates';
import { useErrorMessage } from '#base/composables/useErrorMessage';
import { useScopeLabel } from '#base/composables/useScopeLabel';
import { expiryState } from '#base/lib/tokens/keyStatus';
import { isPerson } from '~/lib/auth/person';
import { useAPIKeysStore } from '~/stores/apiKeys';
import { useAuthStore } from '~/stores/auth';
import { useProjectsStore } from '~/stores/projects';

export interface CreatedKey {
  plaintext: string;
  snippet: string;
}

// The server already hides other people's personal keys, so every personal key in the store is the viewer's.
export function usePersonalKeys(orgId: string) {
  const keys = useAPIKeysStore();
  const auth = useAuthStore();
  const projects = useProjectsStore();
  const scopeLabel = useScopeLabel();
  const errorMessage = useErrorMessage();
  const { date, when } = useDates();
  const { $apiKeyClient: client } = useNuxtApp();

  const error = ref<ShownError | null>(null);
  const created = ref<CreatedKey | null>(null);

  const signedIn = computed(() => isPerson(auth.me));
  const rows = computed<PersonalKeyView[]>(() => {
    const now = new Date();
    return keys.items.filter(k => k.kind === 'personal').map(k => ({
      rowKey: k.rowKey,
      name: k.name,
      secretHint: k.secretHint,
      scopes: k.scopes.map(scopeLabel),
      lastUsed: when(k.lastUsedAt),
      expires: date(k.expiresAt),
      expiry: expiryState(k.expiresAt, now),
      revoked: !!k.revokedAt,
    }));
  });

  async function revoke(hint: string) {
    error.value = null;
    try {
      await client.revokeAPIKey({ orgId, hint });
      keys.markRevoked(hint);
    } catch (e) {
      error.value = errorMessage(e, 'error.revoke_apikey');
    }
  }

  function dismiss() {
    created.value = null;
  }

  async function load() {
    await Promise.all([auth.load(), keys.ensureLoaded(orgId), projects.ensureLoaded(orgId)]);
  }

  return { rows, signedIn, error, created, revoke, dismiss, load };
}
