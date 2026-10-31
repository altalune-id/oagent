<script setup lang="ts">
import AppModal from '#base/components/ui/AppModal.vue';
import ErrorText from '#base/components/ui/ErrorText.vue';
import OrgFields from './OrgFields.vue';
import { useOrgCreate } from './useOrgCreate';

const { t } = useI18n();
const { open, name, slug, busy, error, fieldErrors, hide, submit } = useOrgCreate();
</script>

<template>
  <AppModal :open="open" :title="t('org.new')" @close="hide()">
    <form novalidate class="space-y-1" @submit.prevent="submit()">
      <OrgFields v-model:name="name" v-model:slug="slug" :field-errors="fieldErrors" :busy="busy" />
      <ErrorText :error="error" />
    </form>
    <template #footer>
      <button class="btn btn-ghost btn-sm" type="button" :disabled="busy" @click="hide()">
        {{ t('common.cancel') }}
      </button>
      <button class="btn btn-primary btn-sm" type="button" :disabled="busy || !name.trim()" @click="submit()">
        {{ busy ? t('common.creating') : t('common.create') }}
      </button>
    </template>
  </AppModal>
</template>
