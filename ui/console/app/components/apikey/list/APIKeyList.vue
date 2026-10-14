<script setup lang="ts">
import ErrorText from '~/components/ui/ErrorText.vue';
import { useErrorMessage } from '~/composables/useErrorMessage';
import { useScopeLabel } from '~/composables/useScopeLabel';
import { useAPIKeysStore } from '~/stores/apiKeys';

const { orgId } = defineProps<{ orgId: string }>();

const { t } = useI18n();
const errorMessage = useErrorMessage();
const scopeLabel = useScopeLabel();
const keys = useAPIKeysStore();
const { $apiKeyClient: client } = useNuxtApp();

await keys.ensureLoaded(orgId);

function kindLabel(kind: string): string {
  if (kind === 'org') {
    return t('apikeys.kind_org');
  }
  if (kind === 'personal') {
    return t('apikeys.kind_personal');
  }
  return t('apikeys.kind_project');
}

async function revoke(hint: string) {
  try {
    await client.revokeAPIKey({ orgId, hint });
    keys.markRevoked(hint);
  } catch (e) {
    keys.error = errorMessage(e, 'error.revoke_apikey');
  }
}
</script>

<template>
  <ErrorText v-if="keys.error" :error="keys.error" />
  <p v-else-if="keys.items.length === 0" class="text-sm text-base-content/60">
    {{ t('apikeys.empty') }}
  </p>
  <div v-else class="overflow-x-auto rounded-box border border-base-content/5 bg-base-100">
    <table class="table table-sm">
      <thead>
        <tr>
          <th>{{ t('common.name') }}</th>
          <th>{{ t('apikeys.col_kind') }}</th>
          <th>{{ t('apikeys.col_scopes') }}</th>
          <th>{{ t('apikeys.col_hint') }}</th>
          <th>{{ t('apikeys.col_status') }}</th>
          <th />
        </tr>
      </thead>
      <tbody>
        <tr v-for="k in keys.items" :key="k.rowKey" class="hover:bg-base-200">
          <td>{{ k.name }}</td>
          <td>{{ kindLabel(k.kind) }}</td>
          <td class="text-base-content/60">
            {{ k.scopes.map(scopeLabel).join(', ') }}
          </td>
          <td class="text-base-content/60">
            …{{ k.secretHint }}
          </td>
          <td>
            <span class="badge badge-sm" :class="k.revokedAt ? 'badge-error' : 'badge-success'">
              {{ k.revokedAt ? t('apikeys.revoked') : t('apikeys.active') }}
            </span>
          </td>
          <td class="whitespace-nowrap text-right">
            <button v-if="!k.revokedAt" class="btn btn-ghost btn-xs text-error" @click="revoke(k.secretHint)">
              <Icon name="lucide:ban" size="14" />
              {{ t('common.revoke') }}
            </button>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
