<script setup lang="ts">
import type { Column, StatusTone } from '#base/components/ui/types';
import type { ShownError } from '#base/lib/apperror/message';
import type { ExpiryState } from '#base/lib/tokens/keyStatus';
import { computed, ref } from 'vue';
import AppButton from '#base/components/ui/AppButton.vue';
import AppDropdown from '#base/components/ui/AppDropdown.vue';
import AppModal from '#base/components/ui/AppModal.vue';
import CopyButton from '#base/components/ui/CopyButton.vue';
import DataTable from '#base/components/ui/DataTable.vue';
import EmptyState from '#base/components/ui/EmptyState.vue';
import ErrorText from '#base/components/ui/ErrorText.vue';
import SecretField from '#base/components/ui/SecretField.vue';
import StatusLabel from '#base/components/ui/StatusLabel.vue';
import { keyStatus } from '#base/lib/tokens/keyStatus';

export interface PersonalKeyView {
  rowKey: string;
  name: string;
  secretHint: string;
  scopes: string[];
  lastUsed: string;
  expires: string;
  expiry: ExpiryState;
  revoked: boolean;
}

export interface PersonalKeysLabels {
  name: string;
  scopes: string;
  lastUsed: string;
  neverUsed: string;
  expires: string;
  expiresSoon: (days: number) => string;
  expired: string;
  status: string;
  active: string;
  revoked: string;
  revoke: string;
  revokeConfirm: string;
  actionsFor: (name: string) => string;
  actionsColumn: string;
  cancel: string;
  empty: string;
  secretWarning: string;
  copy: string;
  snippet: string;
  copySnippet: string;
  done: string;
  signIn: string;
  signInLink: string;
}

const props = withDefaults(defineProps<{
  keys: PersonalKeyView[],
  created?: { plaintext: string, snippet: string } | null,
  error?: ShownError | null,
  signedIn?: boolean,
  loginUrl?: string,
  labels?: PersonalKeysLabels,
}>(), {
  created: null,
  error: null,
  signedIn: true,
  loginUrl: '/login',
  labels: () => ({
    name: 'Name',
    scopes: 'Scopes',
    lastUsed: 'Last used',
    neverUsed: 'Never used',
    expires: 'Expires',
    expiresSoon: (days: number) => `Expires in ${days} days`,
    expired: 'Expired',
    status: 'Status',
    active: 'Active',
    revoked: 'Revoked',
    revoke: 'Revoke',
    revokeConfirm: 'Revoke this key?',
    actionsFor: (name: string) => `Actions for ${name}`,
    actionsColumn: 'Actions',
    cancel: 'Cancel',
    empty: 'You have no personal API keys in this organization yet.',
    secretWarning: 'Copy this secret now — it will not be shown again.',
    copy: 'Copy',
    snippet: 'Add this to .mcp.json',
    copySnippet: 'Copy snippet',
    done: 'Done',
    signIn: 'Sign in to manage your personal API keys.',
    signInLink: 'Sign in',
  }),
});

const emit = defineEmits<{
  revoke: [hint: string],
  dismiss: [],
}>();

const confirming = ref<PersonalKeyView | null>(null);
const statusTone: Record<string, StatusTone> = { active: 'success', expired: 'error', revoked: 'neutral' };

const columns = computed<Column[]>(() => [
  { key: 'name', label: props.labels.name },
  { key: 'scopes', label: props.labels.scopes },
  { key: 'lastUsed', label: props.labels.lastUsed, muted: true },
  { key: 'expires', label: props.labels.expires },
  { key: 'status', label: props.labels.status },
  { key: 'menu', label: props.labels.actionsColumn, hideLabel: true, align: 'end' },
]);

function revoke() {
  const k = confirming.value;
  confirming.value = null;
  if (k) {
    emit('revoke', k.secretHint);
  }
}
</script>

<template>
  <div v-if="!signedIn" class="rounded-box border border-base-300 p-4 text-ui">
    {{ labels.signIn }}
    <a :href="loginUrl" class="link">{{ labels.signInLink }}</a>
  </div>

  <div v-else class="flex flex-col gap-4">
    <div v-if="created" role="alert" class="alert alert-success alert-soft flex flex-col items-stretch gap-3 text-ui">
      <div class="flex w-full items-center justify-between gap-3">
        <p class="font-medium">
          {{ labels.secretWarning }}
        </p>
        <AppButton variant="plain" size="sm" class="shrink-0" @click="emit('dismiss')">
          {{ labels.done }}
        </AppButton>
      </div>
      <div class="w-full min-w-0">
        <SecretField :value="created.plaintext" :label="labels.copy" />
      </div>
      <div class="flex w-full min-w-0 flex-col gap-1">
        <div class="flex items-center justify-between gap-2">
          <span class="text-xs">{{ labels.snippet }}</span>
          <CopyButton :value="created.snippet" :label="labels.copySnippet" />
        </div>
        <pre class="overflow-x-auto rounded-box bg-base-100 p-3 font-mono text-xs text-base-content">{{ created.snippet }}</pre>
      </div>
    </div>

    <ErrorText :error="error" />

    <EmptyState v-if="keys.length === 0" icon="lucide:key-round" :title="labels.empty" />
    <DataTable v-else :caption="labels.name" row-key="rowKey" :columns="columns" :rows="keys">
      <template #cell-name="{ row }">
        <span class="whitespace-nowrap">
          <span class="font-medium">{{ row.name }}</span>
          <span class="ms-2 font-mono text-xs text-base-content/70">…{{ row.secretHint }}</span>
        </span>
      </template>
      <template #cell-scopes="{ row }">
        <span class="text-base-content/70">{{ row.scopes.join(', ') }}</span>
      </template>
      <template #cell-lastUsed="{ row }">
        <span class="whitespace-nowrap">{{ row.lastUsed || labels.neverUsed }}</span>
      </template>
      <template #cell-expires="{ row }">
        <span v-if="row.revoked || row.expiry.kind === 'none'" class="text-base-content/70">—</span>
        <StatusLabel v-else-if="row.expiry.kind === 'soon'" tone="warning" :label="labels.expiresSoon(row.expiry.days)" />
        <span v-else class="whitespace-nowrap text-base-content/70">{{ row.expires }}</span>
      </template>
      <template #cell-status="{ row }">
        <StatusLabel :tone="statusTone[keyStatus(row.revoked, row.expiry)] ?? 'neutral'" :label="labels[keyStatus(row.revoked, row.expiry)]" />
      </template>
      <template #cell-menu="{ row }">
        <AppDropdown v-if="keyStatus(row.revoked, row.expiry) === 'active'" placement="end" size="xs" trigger-class="btn-square" content-class="w-44" :label="labels.actionsFor(row.name)">
          <template #trigger>
            <Icon name="lucide:ellipsis" size="14" class="shrink-0" aria-hidden="true" />
          </template>
          <ul class="menu menu-sm w-full p-0 text-ui">
            <li>
              <button type="button" @click="confirming = row as PersonalKeyView">
                <Icon name="lucide:ban" size="16" class="shrink-0 text-error" aria-hidden="true" />
                {{ labels.revoke }}
              </button>
            </li>
          </ul>
        </AppDropdown>
      </template>
    </DataTable>

    <AppModal :open="!!confirming" :title="labels.revokeConfirm" @close="confirming = null">
      <p class="text-ui">
        <span class="font-medium">{{ confirming?.name }}</span>
        <span class="ms-2 font-mono text-xs text-base-content/70">…{{ confirming?.secretHint }}</span>
      </p>
      <template #footer>
        <AppButton variant="ghost" size="sm" @click="confirming = null">
          {{ labels.cancel }}
        </AppButton>
        <AppButton variant="error" size="sm" @click="revoke()">
          {{ labels.revoke }}
        </AppButton>
      </template>
    </AppModal>
  </div>
</template>
