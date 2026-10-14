<script setup lang="ts">
import APIKeyCreateModal from '~/components/apikey/create/APIKeyCreateModal.vue';
import APIKeyList from '~/components/apikey/list/APIKeyList.vue';
import { grants } from '~/components/apikey/personal/grants';
import { useAuthStore } from '~/stores/auth';
import { useOrgsStore } from '~/stores/orgs';

const route = useRoute();
const { t } = useI18n();
const orgs = useOrgsStore();
const org = computed(() => orgs.bySlug(route.params.org as string));
const auth = useAuthStore();
await auth.load();
// NOTE: with nobody signed in (before credentials are required) the button stays; the server decides.
const canManage = computed(() => !auth.me || auth.me.operator || grants(auth.me.permissions, 'apikey:write'));

useHead({ title: () => `${org.value?.name ?? t('org.title')} · ${t('apikeys.title')} · oagent` });
</script>

<template>
  <div v-if="org" class="space-y-6">
    <div class="flex items-start justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold">
          {{ t('apikeys.title') }}
        </h1>
        <p class="text-sm text-base-content/60">
          {{ t('apikeys.subtitle') }}
        </p>
      </div>
      <APIKeyCreateModal v-if="canManage" :org-id="org.id" />
    </div>
    <APIKeyList :org-id="org.id" />
  </div>
</template>
