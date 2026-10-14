<script setup lang="ts">
import type { ShownError } from '../../../lib/apperror/message';
import { reactive, ref, watch } from 'vue';
import CopyButton from '../../ui/CopyButton.vue';
import ErrorText from '../../ui/ErrorText.vue';
import SecretField from '../../ui/SecretField.vue';

export interface PersonalKey {
  rowKey: string;
  name: string;
  scopes: string[];
  secretHint: string;
  expiresAt?: Date;
  revokedAt?: Date;
}

export interface PersonalKeyForm {
  name: string;
  scopes: string[];
  expiresInHours: number;
  allProjects: boolean;
  projectIds: string[];
}

export interface PersonalKeysLabels {
  name: string;
  scopes: string;
  projects: string;
  allProjects: string;
  expires: string;
  days7: string;
  days30: string;
  days90: string;
  days365: string;
  create: string;
  creating: string;
  empty: string;
  hint: string;
  status: string;
  active: string;
  revoked: string;
  revoke: string;
  revokeConfirm: string;
  cancel: string;
  secretWarning: string;
  copy: string;
  snippet: string;
  copySnippet: string;
  done: string;
  signIn: string;
  signInLink: string;
}

const props = withDefaults(defineProps<{
  keys: PersonalKey[],
  scopes: { name: string, label: string }[],
  projects: { id: string, name: string }[],
  created?: { plaintext: string, snippet: string } | null,
  busy?: boolean,
  error?: ShownError | null,
  signedIn?: boolean,
  loginUrl?: string,
  labels?: PersonalKeysLabels,
}>(), {
  created: null,
  busy: false,
  error: null,
  signedIn: true,
  loginUrl: '/login',
  labels: () => ({
    name: 'Name',
    scopes: 'Scopes',
    projects: 'Projects',
    allProjects: 'All projects',
    expires: 'Expires in',
    days7: '7 days',
    days30: '30 days',
    days90: '90 days',
    days365: '1 year',
    create: 'Create',
    creating: 'Creating…',
    empty: 'You have no personal keys yet.',
    hint: 'Secret',
    status: 'Status',
    active: 'Active',
    revoked: 'Revoked',
    revoke: 'Revoke',
    revokeConfirm: 'Revoke?',
    cancel: 'Cancel',
    secretWarning: 'Copy the secret now. It is shown only once.',
    copy: 'Copy',
    snippet: 'Add this to .mcp.json',
    copySnippet: 'Copy snippet',
    done: 'Done',
    signIn: 'Sign in to manage your personal keys.',
    signInLink: 'Sign in',
  }),
});

const emit = defineEmits<{
  create: [form: PersonalKeyForm],
  revoke: [hint: string],
  dismiss: [],
}>();

const form = reactive<PersonalKeyForm>({ name: '', scopes: [], expiresInHours: 720, allProjects: true, projectIds: [] });
const confirming = ref('');

watch(() => props.created, (created) => {
  if (created) {
    Object.assign(form, { name: '', scopes: [], expiresInHours: 720, allProjects: true, projectIds: [] });
  }
});

function submit() {
  if (!props.busy && form.name.trim()) {
    emit('create', { ...form, name: form.name.trim(), scopes: [...form.scopes], projectIds: [...form.projectIds] });
  }
}

function revoke(hint: string) {
  confirming.value = '';
  emit('revoke', hint);
}
</script>

<template>
  <div v-if="!signedIn" class="rounded-box border border-base-300 p-4 text-sm">
    {{ labels.signIn }}
    <a :href="loginUrl" class="link link-primary">{{ labels.signInLink }}</a>
  </div>

  <div v-else class="space-y-6">
    <div v-if="created" class="space-y-3 rounded-box border border-warning/40 bg-base-100 p-4">
      <p class="text-sm text-warning">
        {{ labels.secretWarning }}
      </p>
      <SecretField :value="created.plaintext" :label="labels.copy" />
      <div class="space-y-1">
        <div class="flex items-center justify-between gap-2">
          <span class="text-xs text-base-content/60">{{ labels.snippet }}</span>
          <CopyButton :value="created.snippet" :label="labels.copySnippet" />
        </div>
        <pre class="overflow-x-auto rounded-box bg-base-200 p-3 text-xs">{{ created.snippet }}</pre>
      </div>
      <button type="button" class="btn btn-sm" @click="emit('dismiss')">
        {{ labels.done }}
      </button>
    </div>

    <form v-else class="space-y-3 rounded-box border border-base-300 bg-base-100 p-4" @submit.prevent="submit">
      <label class="block">
        <span class="mb-1 block text-xs text-base-content/60">{{ labels.name }}</span>
        <input v-model="form.name" type="text" class="input input-sm input-bordered w-full" placeholder="laptop" :disabled="busy">
      </label>

      <div>
        <span class="mb-1 block text-xs text-base-content/60">{{ labels.scopes }}</span>
        <div class="grid grid-cols-2 gap-1">
          <label v-for="s in scopes" :key="s.name" class="flex items-center gap-2 text-sm">
            <input v-model="form.scopes" type="checkbox" :value="s.name" class="checkbox checkbox-xs" :disabled="busy">{{ s.label }}
          </label>
        </div>
      </div>

      <div class="space-y-2">
        <span class="block text-xs text-base-content/60">{{ labels.projects }}</span>
        <label class="flex items-center gap-2 text-sm"><input v-model="form.allProjects" type="checkbox" class="checkbox checkbox-sm" :disabled="busy">{{ labels.allProjects }}</label>
        <div v-if="!form.allProjects" class="max-h-32 space-y-1 overflow-auto rounded-box border border-base-300 p-2">
          <label v-for="p in projects" :key="p.id" class="flex items-center gap-2 text-sm">
            <input v-model="form.projectIds" type="checkbox" :value="p.id" class="checkbox checkbox-xs" :disabled="busy">{{ p.name }}
          </label>
        </div>
      </div>

      <label class="block">
        <span class="mb-1 block text-xs text-base-content/60">{{ labels.expires }}</span>
        <select v-model.number="form.expiresInHours" class="select select-sm select-bordered w-full" :disabled="busy">
          <option :value="168">{{ labels.days7 }}</option>
          <option :value="720">{{ labels.days30 }}</option>
          <option :value="2160">{{ labels.days90 }}</option>
          <option :value="8760">{{ labels.days365 }}</option>
        </select>
      </label>

      <ErrorText :error="error" />
      <button type="submit" class="btn btn-primary btn-sm" :disabled="busy || !form.name.trim()">
        <Icon name="lucide:plus" size="16" />
        {{ busy ? labels.creating : labels.create }}
      </button>
    </form>

    <p v-if="keys.length === 0" class="text-sm text-base-content/60">
      {{ labels.empty }}
    </p>
    <div v-else class="overflow-x-auto rounded-box border border-base-content/5 bg-base-100">
      <table class="table table-sm">
        <thead>
          <tr>
            <th>{{ labels.name }}</th>
            <th>{{ labels.scopes }}</th>
            <th>{{ labels.hint }}</th>
            <th>{{ labels.status }}</th>
            <th />
          </tr>
        </thead>
        <tbody>
          <tr v-for="k in keys" :key="k.rowKey" class="hover:bg-base-200">
            <td>{{ k.name }}</td>
            <td class="text-base-content/60">
              {{ k.scopes.join(', ') }}
            </td>
            <td class="text-base-content/60">
              …{{ k.secretHint }}
            </td>
            <td>
              <span class="badge badge-sm" :class="k.revokedAt ? 'badge-error' : 'badge-success'">
                {{ k.revokedAt ? labels.revoked : labels.active }}
              </span>
            </td>
            <td class="whitespace-nowrap text-right">
              <template v-if="!k.revokedAt">
                <template v-if="confirming === k.secretHint">
                  <span class="mr-1 text-xs">{{ labels.revokeConfirm }}</span>
                  <button type="button" class="btn btn-error btn-xs" @click="revoke(k.secretHint)">
                    {{ labels.revoke }}
                  </button>
                  <button type="button" class="btn btn-ghost btn-xs" @click="confirming = ''">
                    {{ labels.cancel }}
                  </button>
                </template>
                <button v-else type="button" class="btn btn-ghost btn-xs text-error" @click="confirming = k.secretHint">
                  <Icon name="lucide:ban" size="14" />
                  {{ labels.revoke }}
                </button>
              </template>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
