<script setup lang="ts">
import InviteModal from '~/components/member/invite/InviteModal.vue';
import PendingInvites from '~/components/member/invite/PendingInvites.vue';
import { useInvite } from '~/components/member/invite/useInvite';
import MemberList from '~/components/member/MemberList.vue';
import { useMembers } from '~/components/member/useMembers';
import ErrorText from '~/components/ui/ErrorText.vue';
import { useOrgsStore } from '~/stores/orgs';

const route = useRoute();
const { t } = useI18n();
const orgs = useOrgsStore();
await orgs.ensureLoaded();
const org = computed(() => orgs.bySlug(route.params.org as string));
const members = useMembers(org.value?.id ?? '');
const { rows, canLeave, loading, loadError, busy, error, confirming, confirmMessage } = members;
const viewerRole = computed(() => members.viewer.value.role);
const invite = useInvite(org.value?.id ?? '', viewerRole);
const {
  pending,
  drafts,
  canInvite,
  roles: inviteRoles,
  modalOpen,
  email: inviteEmail,
  role: inviteRole,
  busy: inviteBusy,
  error: inviteError,
  fieldErrors: inviteFieldErrors,
  sent,
  sentMessage,
  rowLink,
  notice: inviteNotice,
  confirming: inviteConfirming,
  confirmMessage: inviteConfirmMessage,
  loadError: inviteLoadError,
} = invite;

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
}));

const roleLabels = computed(() => ({ owner: t('members.role_owner'), admin: t('members.role_admin'), member: t('members.role_member') }));
const modalLabels = computed(() => ({
  title: t('invites.invite'),
  email: t('invites.col_email'),
  role: t('members.col_role'),
  ...roleLabels.value,
  send: t('invites.send'),
  cancel: t('common.cancel'),
  done: t('invites.done'),
  linkHint: t('invites.link_hint'),
  copy: t('invites.copy'),
}));
const inviteLabels = computed(() => ({
  title: t('invites.title'),
  draftsTitle: t('invites.drafts_title'),
  draftsHint: t('invites.drafts_hint'),
  email: t('invites.col_email'),
  role: t('members.col_role'),
  status: t('invites.col_status'),
  invitedBy: t('invites.col_invited_by'),
  expires: t('invites.col_expires'),
  pending: t('invites.status_pending'),
  expired: t('invites.status_expired'),
  ...roleLabels.value,
  resend: t('invites.resend'),
  revoke: t('invites.revoke'),
  revokeTitle: t('invites.revoke_title'),
  confirmDraft: t('invites.confirm_draft'),
  discard: t('invites.discard'),
  cancel: t('common.cancel'),
  newLink: t('invites.new_link'),
  copy: t('invites.copy'),
}));

await members.load();
if (canInvite.value) {
  await invite.load();
}

useHead({ title: () => `${org.value?.name ?? t('org.title')} · ${t('members.title')} · oagent` });
</script>

<template>
  <div v-if="org" class="space-y-6">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <h1 class="text-2xl font-bold">
          {{ t('members.title') }}
        </h1>
        <p class="text-sm text-base-content/60">
          {{ t('members.subtitle') }}
        </p>
      </div>
      <button v-if="canInvite" type="button" class="btn btn-primary btn-sm" @click="invite.open">
        <Icon name="lucide:user-plus" size="16" />
        {{ t('invites.invite') }}
      </button>
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
    <p v-if="inviteNotice" class="text-sm text-success">
      {{ inviteNotice }}
    </p>
    <ErrorText v-if="canInvite && inviteLoadError" :error="inviteLoadError" />
    <PendingInvites
      v-else-if="canInvite"
      :pending="pending"
      :drafts="drafts"
      :can-manage="canInvite"
      :busy="inviteBusy"
      :error="modalOpen ? null : inviteError"
      :row-link="rowLink"
      :confirming="inviteConfirming"
      :confirm-message="inviteConfirmMessage"
      :labels="inviteLabels"
      @resend="invite.resend"
      @revoke="invite.askRevoke"
      @confirm-draft="invite.confirmDraft"
      @discard="invite.discardDraft"
      @confirm="invite.confirm"
      @cancel="invite.cancel"
    />
    <InviteModal
      v-model:email="inviteEmail"
      v-model:role="inviteRole"
      :open="modalOpen"
      :roles="inviteRoles"
      :busy="inviteBusy"
      :error="inviteError"
      :field-errors="inviteFieldErrors"
      :sent-link="sent?.link ?? ''"
      :sent-message="sentMessage"
      :labels="modalLabels"
      @submit="invite.submit"
      @close="invite.close"
    />
  </div>
</template>
