<script setup lang="ts">
import PersonalKeyCreateForm from '~/components/apikey/personal/PersonalKeyCreateForm.vue';
import PersonalKeys from '~/components/apikey/personal/PersonalKeys.vue';
import { usePersonalKeyCreate } from '~/components/apikey/personal/usePersonalKeyCreate';
import { usePersonalKeys } from '~/components/apikey/personal/usePersonalKeys';
import { loginURL } from '~/lib/auth/urls';
import { useOrgsStore } from '~/stores/orgs';

const route = useRoute();
const { t } = useI18n();
const orgs = useOrgsStore();
await orgs.ensureLoaded();
const org = computed(() => orgs.bySlug(route.params.org as string));
const personal = usePersonalKeys(org.value?.id ?? '');
const { rows, signedIn, error, created } = personal;
const form = usePersonalKeyCreate(org.value?.id ?? '', (key) => {
  created.value = key;
});
const { name, expiresInDays, allProjects, projectIds, scopes, projects, mintable, busy, error: createError, canSubmit } = form;
const expiryOptions = computed(() => form.expiryOptions.map(o => ({ days: o.days, label: t(o.labelKey) })));
const loginUrl = computed(() => (import.meta.client ? loginURL(useRuntimeConfig().public.apiUrl, window.location) : ''));

const formLabels = computed(() => ({
  name: t('common.name'),
  expires: t('apikeys.expires'),
  expiresHelp: t('apikeys.expires_help'),
  projects: t('apikeys.projects'),
  allProjects: t('apikeys.all_projects'),
  selectProjects: t('apikeys.select_projects'),
  scopes: t('apikeys.scopes'),
  scopesHelp: t('apikeys.scopes_help'),
  create: t('common.create'),
  creating: t('common.creating'),
}));

const listLabels = computed(() => ({
  name: t('common.name'),
  scopes: t('apikeys.scopes'),
  lastUsed: t('apikeys.col_last_used'),
  neverUsed: t('tokens.never_used'),
  expires: t('apikeys.expires'),
  expiresSoon: (days: number) => t('tokens.expires_soon', { days }),
  expired: t('tokens.status_expired'),
  status: t('apikeys.col_status'),
  active: t('tokens.status_active'),
  revoked: t('tokens.status_revoked'),
  revoke: t('common.revoke'),
  revokeConfirm: t('apikeys.revoke_confirm'),
  cancel: t('common.cancel'),
  empty: t('apikeys.personal_empty'),
  secretWarning: t('apikeys.secret_warning'),
  copy: t('apikeys.copy'),
  snippet: t('apikeys.mcp_snippet'),
  copySnippet: t('apikeys.copy_snippet'),
  done: t('apikeys.done'),
  signIn: t('apikeys.personal_sign_in'),
  signInLink: t('auth.sign_in'),
}));

await personal.load();

useHead({ title: () => `${org.value?.name ?? t('org.title')} · ${t('apikeys.personal_title')} · OAgent` });
</script>

<template>
  <div v-if="org" class="space-y-6">
    <div>
      <h1 class="text-2xl font-bold">
        {{ t('apikeys.personal_title') }}
      </h1>
      <p class="text-sm text-base-content/60">
        {{ t('apikeys.personal_subtitle') }}
      </p>
    </div>
    <PersonalKeyCreateForm
      v-if="signedIn"
      v-model:name="name"
      v-model:expires-in-days="expiresInDays"
      v-model:all-projects="allProjects"
      v-model:project-ids="projectIds"
      v-model:scopes="scopes"
      :expiry-options="expiryOptions"
      :projects="projects"
      :scope-options="mintable"
      :busy="busy"
      :can-submit="canSubmit"
      :error="createError"
      :labels="formLabels"
      @submit="form.submit"
    />
    <PersonalKeys
      :keys="rows"
      :created="created"
      :error="error"
      :signed-in="signedIn"
      :login-url="loginUrl"
      :labels="listLabels"
      @revoke="personal.revoke"
      @dismiss="personal.dismiss"
    />
  </div>
</template>
