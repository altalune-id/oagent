<script setup lang="ts">
import type { Column, StatusTone } from '#base/components/ui/types';
import type { ShownError } from '#base/lib/apperror/message';
import type { Invite } from '../../../stores/invites';
import { computed } from 'vue';
import AppBadge from '#base/components/ui/AppBadge.vue';
import AppButton from '#base/components/ui/AppButton.vue';
import AppDropdown from '#base/components/ui/AppDropdown.vue';
import AppModal from '#base/components/ui/AppModal.vue';
import DataTable from '#base/components/ui/DataTable.vue';
import ErrorText from '#base/components/ui/ErrorText.vue';
import SecretField from '#base/components/ui/SecretField.vue';
import Section from '#base/components/ui/Section.vue';
import StatusLabel from '#base/components/ui/StatusLabel.vue';
import { formatDate } from '#base/lib/format/date';

export interface PendingInvitesLabels {
  title: string;
  actionsColumn: string;
  actionsFor: string;
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
  moment?: (d?: Date) => string,
}>(), {
  canManage: false,
  busy: false,
  error: null,
  rowLink: null,
  confirming: null,
  confirmMessage: '',
  moment: (d?: Date) => formatDate(d, 'en-US', { style: 'moment' }),
  labels: () => ({
    title: 'Pending invites',
    actionsColumn: 'Actions',
    actionsFor: 'Actions for {name}',
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

function statusTone(s: string): StatusTone {
  return s === 'expired' ? 'warning' : 'neutral';
}

const columns = computed<Column[]>(() => [
  { key: 'email', label: props.labels.email },
  { key: 'role', label: props.labels.role },
  { key: 'status', label: props.labels.status },
  { key: 'invitedByName', label: props.labels.invitedBy },
  { key: 'expires', label: props.labels.expires },
  ...(props.canManage ? [{ key: 'menu', label: props.labels.actionsColumn, hideLabel: true, align: 'end' as const }] : []),
]);

const linkFor = computed(() => props.pending.find(i => i.id === props.rowLink?.id));
</script>

<template>
  <div class="flex flex-col gap-5">
    <ErrorText :error="error" />

    <Section v-if="drafts.length > 0" :title="labels.draftsTitle" :description="labels.draftsHint" flush>
      <ul class="divide-y divide-base-300">
        <li v-for="d in drafts" :key="d.id" class="flex flex-wrap items-center gap-3 px-4 py-2 text-ui">
          <div class="flex min-w-0 flex-1 flex-col">
            <span class="truncate font-medium">{{ d.email }}</span>
            <span class="text-xs text-base-content/70">{{ roleLabel(d.role) }} · {{ d.invitedByName }}</span>
          </div>
          <template v-if="canManage">
            <AppButton size="xs" :disabled="busy" @click="emit('confirmDraft', d.id)">
              {{ labels.confirmDraft }}
            </AppButton>
            <AppButton variant="ghost" size="xs" :disabled="busy" @click="emit('discard', d.id)">
              {{ labels.discard }}
            </AppButton>
          </template>
        </li>
      </ul>
    </Section>

    <section v-if="pending.length > 0" class="flex flex-col gap-2">
      <h2 class="text-ui font-semibold">
        {{ labels.title }}
      </h2>
      <DataTable :caption="labels.title" row-key="id" :columns="columns" :rows="pending">
        <template #cell-email="{ row }">
          <span class="font-medium">{{ row.email }}</span>
        </template>
        <template #cell-role="{ row }">
          <AppBadge :label="roleLabel(row.role)" />
        </template>
        <template #cell-status="{ row }">
          <StatusLabel :tone="statusTone(row.status)" :label="statusLabel(row.status)" />
        </template>
        <template #cell-expires="{ row }">
          <span class="whitespace-nowrap tabular-nums text-base-content/70">{{ moment(row.expiresAt) }}</span>
        </template>
        <template #cell-menu="{ row }">
          <AppDropdown placement="end" size="xs" trigger-class="btn-square" content-class="w-44" :label="labels.actionsFor.replace('{name}', row.email)" :disabled="busy">
            <template #trigger>
              <Icon name="lucide:ellipsis" size="14" class="shrink-0" aria-hidden="true" />
            </template>
            <ul class="menu menu-sm w-full p-0 text-ui">
              <li>
                <button type="button" @click="emit('resend', row.id)">
                  <Icon name="lucide:send" size="16" class="shrink-0" aria-hidden="true" />
                  {{ labels.resend }}
                </button>
              </li>
              <li>
                <button type="button" @click="emit('revoke', row.id)">
                  <Icon name="lucide:x" size="16" class="shrink-0 text-error" aria-hidden="true" />
                  {{ labels.revoke }}
                </button>
              </li>
            </ul>
          </AppDropdown>
        </template>
      </DataTable>
      <div v-if="rowLink && linkFor" class="flex flex-col gap-2 rounded-box border border-base-300 bg-base-100 p-4">
        <p class="text-xs text-base-content/70">
          {{ labels.newLink }}
        </p>
        <SecretField :value="rowLink.link" :label="labels.copy" />
      </div>
    </section>

    <AppModal :open="confirming !== null" :title="labels.revokeTitle" @close="emit('cancel')">
      <p class="text-ui">
        {{ confirmMessage }}
      </p>
      <template #footer>
        <AppButton variant="ghost" size="sm" @click="emit('cancel')">
          {{ labels.cancel }}
        </AppButton>
        <AppButton variant="error" size="sm" :disabled="busy" @click="emit('confirm')">
          {{ labels.revoke }}
        </AppButton>
      </template>
    </AppModal>
  </div>
</template>
