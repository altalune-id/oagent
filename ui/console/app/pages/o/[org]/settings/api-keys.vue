<script setup lang="ts">
import PageHeader from '#base/components/ui/PageHeader.vue';
import APIKeyCreateModal from '~/components/apikey/create/APIKeyCreateModal.vue';
import APIKeyList from '~/components/apikey/list/APIKeyList.vue';
import { useCan } from '~/composables/useCan';
import { useAuthStore } from '~/stores/auth';
import { useOrgsStore } from '~/stores/orgs';

const route = useRoute();
const { t } = useI18n();
const orgs = useOrgsStore();
const org = computed(() => orgs.bySlug(route.params.org as string));
const canManage = useCan('apikey:write');
await useAuthStore().load();

useHead({ title: () => `${org.value?.name ?? t('org.title')} · ${t('apikeys.title')} · OAgent` });
</script>

<template>
  <div v-if="org" class="flex flex-col gap-4">
    <PageHeader :title="t('apikeys.title')" :description="t('apikeys.subtitle')">
      <template v-if="canManage" #actions>
        <APIKeyCreateModal :org-id="org.id" />
      </template>
    </PageHeader>
    <APIKeyList :org-id="org.id" :can-manage="canManage" />
  </div>
</template>
