<script setup lang="ts">
import type { ShownError } from '#base/lib/apperror/message';
import type { Invite } from '../../../stores/invites';
import AppModal from '#base/components/ui/AppModal.vue';
import ErrorText from '#base/components/ui/ErrorText.vue';
import SecretField from '#base/components/ui/SecretField.vue';

export interface PendingInvitesLabels {
  title: string;
  draftsTitle: string;
  draftsHint: string;
  email: string;
  role: string;
  status: string;
  invitedBy: string;
  expires: string;
  pending: string;
  expired: string;
  owner: string;
  admin: string;
  member: string;
  resend: string;
  revoke: string;
  revokeTitle: string;
  confirmDraft: string;
  discard: string;
  cancel: string;
  newLink: string;
  copy: string;
}

const props = withDefaults(defineProps<{
  pending: Invite[],
  drafts: Invite[],
  canManage?: boolean,
  busy?: boolean,
  error?: ShownError | null,
  rowLink?: { id: string, link: string } | null,
  confirming?: { id: string } | null,
  confirmMessage?: string,
  labels?: PendingInvitesLabels,
}>(), {
  canManage: false,
  busy: false,
  error: null,
  rowLink: null,
  confirming: null,
  confirmMessage: '',
  labels: () => ({
    title: 'Pending invites',
    draftsTitle: 'Suggested by an assistant',
    draftsHint: 'An assistant drafted these invites. Nothing is sent until you confirm.',
    email: 'Email',
    role: 'Role',
    status: 'Status',
    invitedBy: 'Invited by',
    expires: 'Expires',
    pending: 'Pending',
    expired: 'Expired',
    owner: 'Owner',
    admin: 'Admin',
    member: 'Member',
    resend: 'Resend',
    revoke: 'Revoke',
    revokeTitle: 'Revoke invite',
    confirmDraft: 'Confirm and send',
    discard: 'Discard',
    cancel: 'Cancel',
    newLink: 'New link. Share it if the email doesn\'t arrive:',
    copy: 'Copy link',
  }),
});

const emit = defineEmits<{
  resend: [id: string],
  revoke: [id: string],
  confirmDraft: [id: string],
  discard: [id: string],
  confirm: [],
  cancel: [],
}>();

function roleLabel(r: string): string {
  return ({ owner: props.labels.owner, admin: props.labels.admin, member: props.labels.member } as Record<string, string>)[r] ?? r;
}

function statusLabel(s: string): string {
  return s === 'expired' ? props.labels.expired : props.labels.pending;
}

function when(d?: Date): string {
  return d ? d.toLocaleString() : '';
}
</script>

<template>
  <div class="space-y-6">
    <ErrorText :error="error" />

    <section v-if="drafts.length > 0" class="space-y-2">
      <h2 class="text-lg font-semibold">
        {{ labels.draftsTitle }}
      </h2>
      <p class="text-sm text-base-content/60">
        {{ labels.draftsHint }}
      </p>
      <ul class="space-y-2">
        <li v-for="d in drafts" :key="d.id" class="flex flex-wrap items-center gap-3 rounded-box border border-base-content/5 bg-base-100 p-3">
          <div class="min-w-0 flex-1">
            <div class="truncate font-medium">
              {{ d.email }}
            </div>
            <div class="text-xs text-base-content/60">
              {{ roleLabel(d.role) }} · {{ d.invitedByName }}
            </div>
          </div>
          <template v-if="canManage">
            <button type="button" class="btn btn-primary btn-xs" :disabled="busy" @click="emit('confirmDraft', d.id)">
              {{ labels.confirmDraft }}
            </button>
            <button type="button" class="btn btn-ghost btn-xs" :disabled="busy" @click="emit('discard', d.id)">
              {{ labels.discard }}
            </button>
          </template>
        </li>
      </ul>
    </section>

    <section v-if="pending.length > 0" class="space-y-2">
      <h2 class="text-lg font-semibold">
        {{ labels.title }}
      </h2>
      <div class="overflow-x-auto rounded-box border border-base-content/5 bg-base-100">
        <table class="table table-sm">
          <thead>
            <tr>
              <th>{{ labels.email }}</th>
              <th>{{ labels.role }}</th>
              <th>{{ labels.status }}</th>
              <th>{{ labels.invitedBy }}</th>
              <th>{{ labels.expires }}</th>
              <th />
            </tr>
          </thead>
          <tbody>
            <template v-for="i in pending" :key="i.id">
              <tr class="hover:bg-base-200">
                <td class="font-medium">
                  {{ i.email }}
                </td>
                <td>{{ roleLabel(i.role) }}</td>
                <td>
                  <span class="badge badge-sm" :class="i.status === 'expired' ? 'badge-warning' : 'badge-ghost'">{{ statusLabel(i.status) }}</span>
                </td>
                <td>{{ i.invitedByName }}</td>
                <td class="whitespace-nowrap text-xs">
                  {{ when(i.expiresAt) }}
                </td>
                <td class="whitespace-nowrap text-right">
                  <template v-if="canManage">
                    <button type="button" class="btn btn-ghost btn-xs" :disabled="busy" @click="emit('resend', i.id)">
                      <Icon name="lucide:send" size="14" />
                      {{ labels.resend }}
                    </button>
                    <button type="button" class="btn btn-ghost btn-xs" :disabled="busy" @click="emit('revoke', i.id)">
                      <Icon name="lucide:x" size="14" class="text-error" />
                      {{ labels.revoke }}
                    </button>
                  </template>
                </td>
              </tr>
              <tr v-if="rowLink?.id === i.id">
                <td colspan="6" class="space-y-2">
                  <p class="text-xs text-base-content/60">
                    {{ labels.newLink }}
                  </p>
                  <SecretField :value="rowLink.link" :label="labels.copy" />
                </td>
              </tr>
            </template>
          </tbody>
        </table>
      </div>
    </section>

    <AppModal :open="confirming !== null" :title="labels.revokeTitle" @close="emit('cancel')">
      <p class="text-sm">
        {{ confirmMessage }}
      </p>
      <template #footer>
        <button type="button" class="btn btn-ghost btn-sm" @click="emit('cancel')">
          {{ labels.cancel }}
        </button>
        <button type="button" class="btn btn-error btn-sm" :disabled="busy" @click="emit('confirm')">
          {{ labels.revoke }}
        </button>
      </template>
    </AppModal>
  </div>
</template>
