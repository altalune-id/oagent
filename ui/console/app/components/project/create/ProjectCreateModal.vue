<script setup lang="ts">
import AppModal from '~/components/ui/AppModal.vue'
import { useProjectCreate } from './useProjectCreate'

const { orgId } = defineProps<{ orgId: string }>()

const { t } = useI18n()
const { open, name, slug, busy, error, hide, submit } = useProjectCreate(orgId)
</script>

<template>
  <AppModal :open="open" :title="t('project.new')" @close="hide()">
    <form class="space-y-3" @submit.prevent="submit()">
      <label class="block">
        <span class="mb-1 block text-xs text-base-content/60">{{ t('common.name') }}</span>
        <input v-model="name" type="text" class="input input-sm input-bordered w-full" placeholder="Support Bot" :disabled="busy">
      </label>
      <label class="block">
        <span class="mb-1 block text-xs text-base-content/60">{{ t('common.slug') }}</span>
        <input v-model="slug" type="text" class="input input-sm input-bordered w-full" placeholder="support-bot (generated when empty)" :disabled="busy">
      </label>
      <p v-if="error" class="text-sm text-error">
        {{ error }}
      </p>
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
