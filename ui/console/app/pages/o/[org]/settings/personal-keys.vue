<script setup lang="ts">
import PersonalKeys from '~/components/apikey/personal/PersonalKeys.vue';
import { usePersonalKeys } from '~/components/apikey/personal/usePersonalKeys';
import { loginURL } from '~/lib/auth/urls';
import { useOrgsStore } from '~/stores/orgs';

const route = useRoute();
const { t } = useI18n();
const orgs = useOrgsStore();
await orgs.ensureLoaded();
const org = computed(() => orgs.bySlug(route.params.org as string));
const personal = usePersonalKeys(org.value?.id ?? '');
const { mine, mintable, orgProjects, signedIn, busy, error, created } = personal;
const loginUrl = computed(() => (import.meta.client ? loginURL(useRuntimeConfig().public.apiUrl, window.location) : ''));

const labels = computed(() => ({
  name: t('common.name'),
  scopes: t('apikeys.scopes'),
  projects: t('apikeys.projects'),
  allProjects: t('apikeys.all_projects'),
  expires: t('apikeys.expires'),
  days7: t('apikeys.days_7'),
  days30: t('apikeys.days_30'),
  days90: t('apikeys.days_90'),
  days365: t('apikeys.days_365'),
  create: t('common.create'),
  creating: t('common.creating'),
  empty: t('apikeys.personal_empty'),
  hint: t('apikeys.col_hint'),
  status: t('apikeys.col_status'),
  active: t('apikeys.active'),
  revoked: t('apikeys.revoked'),
  revoke: t('common.revoke'),
  revokeConfirm: t('apikeys.revoke_confirm'),
  cancel: t('common.cancel'),
  secretWarning: t('apikeys.secret_warning'),
  copy: t('apikeys.copy'),
  snippet: t('apikeys.mcp_snippet'),
  copySnippet: t('apikeys.copy_snippet'),
  done: t('apikeys.done'),
  signIn: t('apikeys.personal_sign_in'),
  signInLink: t('auth.sign_in'),
}));

await personal.load();

useHead({ title: () => `${org.value?.name ?? t('org.title')} · ${t('apikeys.personal_title')} · oagent` });
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
    <PersonalKeys
      :keys="mine"
      :scopes="mintable"
      :projects="orgProjects"
      :created="created"
      :busy="busy"
      :error="error"
      :signed-in="signedIn"
      :login-url="loginUrl"
      :labels="labels"
      @create="personal.create"
      @revoke="personal.revoke"
      @dismiss="personal.dismiss"
    />
  </div>
</template>
