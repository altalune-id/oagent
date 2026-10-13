<script setup lang="ts">
import MemberList from '~/components/member/MemberList.vue'
import { useMembers } from '~/components/member/useMembers'
import ErrorText from '~/components/ui/ErrorText.vue'
import { useOrgsStore } from '~/stores/orgs'

const route = useRoute()
const { t } = useI18n()
const orgs = useOrgsStore()
await orgs.ensureLoaded()
const org = computed(() => orgs.bySlug(route.params.org as string))
const members = useMembers(org.value?.id ?? '')
const { rows, canLeave, loading, loadError, busy, error, confirming, confirmMessage } = members

const labels = computed(() => ({
  member: t('members.col_member'),
  role: t('members.col_role'),
  you: t('members.you'),
  owner: t('members.role_owner'),
  admin: t('members.role_admin'),
  memberRole: t('members.role_member'),
  remove: t('members.remove'),
  leave: t('members.leave'),
  cancel: t('common.cancel'),
  removeTitle: t('members.remove_title'),
  leaveTitle: t('members.leave_title'),
  roleTitle: t('members.role_title'),
  changeRole: t('members.change_role'),
  empty: t('members.empty'),
}))

await members.load()

useHead({ title: () => `${org.value?.name ?? t('org.title')} · ${t('members.title')} · oagent` })
</script>

<template>
  <div v-if="org" class="space-y-6">
    <div>
      <h1 class="text-2xl font-bold">
        {{ t('members.title') }}
      </h1>
      <p class="text-sm text-base-content/60">
        {{ t('members.subtitle') }}
      </p>
    </div>
    <ErrorText v-if="loadError" :error="loadError" />
    <span v-else-if="loading" class="loading loading-spinner loading-sm" />
    <MemberList
      v-else
      :rows="rows"
      :can-leave="canLeave"
      :busy="busy"
      :error="error"
      :confirming="confirming"
      :confirm-message="confirmMessage"
      :labels="labels"
      @role="members.askRole"
      @remove="members.askRemove"
      @leave="members.askLeave"
      @confirm="members.confirm"
      @cancel="members.cancel"
    />
  </div>
</template>
