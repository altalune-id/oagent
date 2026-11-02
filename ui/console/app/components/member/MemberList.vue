<script setup lang="ts">
import type { Column } from '#base/components/ui/types';
import type { ShownError } from '#base/lib/apperror/message';
import { computed } from 'vue';
import AppBadge from '#base/components/ui/AppBadge.vue';
import AppButton from '#base/components/ui/AppButton.vue';
import AppDropdown from '#base/components/ui/AppDropdown.vue';
import AppModal from '#base/components/ui/AppModal.vue';
import Avatar from '#base/components/ui/Avatar.vue';
import DataTable from '#base/components/ui/DataTable.vue';
import ErrorText from '#base/components/ui/ErrorText.vue';
import { displayName } from './names';

export interface MemberListRow {
  userId: string;
  email: string;
  name: string;
  role: string;
  initials: string;
  self: boolean;
  roles: string[];
  removable: boolean;
}

export interface MemberListLabels {
  caption: string;
  member: string;
  role: string;
  you: string;
  owner: string;
  admin: string;
  memberRole: string;
  remove: string;
  leave: string;
  cancel: string;
  removeTitle: string;
  leaveTitle: string;
  roleTitle: string;
  changeRole: string;
  empty: string;
  actions: string;
  actionsColumn: string;
}

const props = withDefaults(defineProps<{
  rows: MemberListRow[],
  canLeave?: boolean,
  busy?: boolean,
  error?: ShownError | null,
  confirming?: { kind: 'remove' | 'leave' | 'role', name: string } | null,
  confirmMessage?: string,
  labels?: MemberListLabels,
}>(), {
  canLeave: false,
  busy: false,
  error: null,
  confirming: null,
  confirmMessage: '',
  labels: () => ({
    caption: 'Members',
    member: 'Member',
    role: 'Role',
    you: 'you',
    owner: 'Owner',
    admin: 'Admin',
    memberRole: 'Member',
    remove: 'Remove',
    leave: 'Leave organization',
    cancel: 'Cancel',
    removeTitle: 'Remove member',
    leaveTitle: 'Leave organization',
    roleTitle: 'Change role?',
    changeRole: 'Change role',
    empty: 'No members yet.',
    actions: 'Actions for {name}',
    actionsColumn: 'Actions',
  }),
});

const emit = defineEmits<{
  role: [userId: string, role: string],
  remove: [userId: string],
  leave: [],
  confirm: [],
  cancel: [],
}>();

function roleLabel(role: string): string {
  switch (role) {
    case 'owner':
      return props.labels.owner;
    case 'admin':
      return props.labels.admin;
    case 'member':
      return props.labels.memberRole;
    default:
      return role;
  }
}

const confirmTitle = computed(() => {
  switch (props.confirming?.kind) {
    case 'leave':
      return props.labels.leaveTitle;
    case 'role':
      return props.labels.roleTitle;
    default:
      return props.labels.removeTitle;
  }
});

const confirmAction = computed(() => {
  switch (props.confirming?.kind) {
    case 'leave':
      return props.labels.leave;
    case 'role':
      return props.labels.changeRole;
    default:
      return props.labels.remove;
  }
});

const columns = computed<Column[]>(() => [
  { key: 'member', label: props.labels.member },
  { key: 'role', label: props.labels.role },
  { key: 'menu', label: props.labels.actionsColumn, hideLabel: true, align: 'end' },
]);

function hasActions(m: MemberListRow): boolean {
  return m.removable || (m.self && props.canLeave);
}

function onRole(m: MemberListRow, event: Event) {
  const select = event.target as HTMLSelectElement;
  const role = select.value;
  select.value = m.role;
  emit('role', m.userId, role);
}
</script>

<template>
  <div class="flex flex-col gap-4">
    <ErrorText :error="error" />

    <p v-if="rows.length === 0" class="text-ui text-base-content/70">
      {{ labels.empty }}
    </p>
    <DataTable v-else :caption="labels.caption" row-key="userId" :columns="columns" :rows="rows">
      <template #cell-member="{ row }">
        <span class="flex min-w-0 items-center gap-3">
          <Avatar :id="row.userId" :name="row.name" :email="row.email" size="sm" />
          <span class="flex min-w-0 flex-col">
            <span class="flex items-center gap-2">
              <span class="truncate font-medium">{{ displayName(row) }}</span>
              <AppBadge v-if="row.self" :label="labels.you" size="xs" outline />
            </span>
            <span v-if="row.name.trim()" class="truncate text-xs text-base-content/70">{{ row.email }}</span>
          </span>
        </span>
      </template>
      <template #cell-role="{ row }">
        <label v-if="row.roles.length > 0" class="select select-xs w-32 text-xs">
          <span class="sr-only">{{ labels.role }}: {{ displayName(row) }}</span>
          <select :value="row.role" :disabled="busy" @change="onRole(row, $event)">
            <option v-for="r in row.roles" :key="r" :value="r">
              {{ roleLabel(r) }}
            </option>
          </select>
        </label>
        <AppBadge v-else :label="roleLabel(row.role)" />
      </template>
      <template #cell-menu="{ row }">
        <AppDropdown v-if="hasActions(row)" placement="end" size="xs" trigger-class="btn-square" content-class="w-48" :label="labels.actions.replace('{name}', displayName(row))" :disabled="busy">
          <template #trigger>
            <Icon name="lucide:ellipsis" size="14" class="shrink-0" aria-hidden="true" />
          </template>
          <ul class="menu menu-sm w-full p-0 text-ui">
            <li v-if="row.removable">
              <button type="button" @click="emit('remove', row.userId)">
                <Icon name="lucide:user-minus" size="16" class="shrink-0 text-error" aria-hidden="true" />
                {{ labels.remove }}
              </button>
            </li>
            <li v-else>
              <button type="button" @click="emit('leave')">
                <Icon name="lucide:log-out" size="16" class="shrink-0" aria-hidden="true" />
                {{ labels.leave }}
              </button>
            </li>
          </ul>
        </AppDropdown>
      </template>
    </DataTable>

    <AppModal :open="confirming !== null" :title="confirmTitle" @close="emit('cancel')">
      <p class="text-ui">
        {{ confirmMessage }}
      </p>
      <template #footer>
        <AppButton variant="ghost" size="sm" @click="emit('cancel')">
          {{ labels.cancel }}
        </AppButton>
        <AppButton :variant="confirming?.kind === 'role' ? 'primary' : 'error'" size="sm" :disabled="busy" @click="emit('confirm')">
          {{ confirmAction }}
        </AppButton>
      </template>
    </AppModal>
  </div>
</template>
