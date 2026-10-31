import type { Ref } from 'vue';
import type { ShownError } from '#base/lib/apperror/message';

import type { Role } from '../roles';
import { create } from '@bufbuild/protobuf';
import { useErrorMessage } from '#base/composables/useErrorMessage';
import { useValidation } from '#base/composables/useValidation';
import { errorCode } from '#base/lib/apperror/message';
import { CreateInviteRequestSchema } from '#gen/invite/v1/invite_pb';
import { toInvite, useInvitesStore } from '~/stores/invites';
import { inviteRoles } from '../roles';

export interface InviteLink {
  id: string;
  email: string;
  link: string;
}

// orgId is the org's public id; viewerRole is '' when the viewer is not a member.
export function useInvite(orgId: string, viewerRole: Ref<string>) {
  const invites = useInvitesStore();
  const errorMessage = useErrorMessage();
  const validation = useValidation(CreateInviteRequestSchema, {
    fields: ['email', 'role'],
    codes: { USR003: 'email', MBR003: 'role', GEN006: 'email' },
  });
  const { t } = useI18n();
  const { $inviteClient: client } = useNuxtApp();

  const modalOpen = ref(false);
  const email = ref('');
  const role = ref<Role>('member');
  const busy = ref(false);
  const error = ref<ShownError | null>(null);
  const sent = ref<InviteLink | null>(null);
  const rowLink = ref<InviteLink | null>(null);
  const confirming = ref<{ id: string, email: string } | null>(null);
  const notice = ref('');

  const roles = computed(() => inviteRoles(viewerRole.value));
  const canInvite = computed(() => roles.value.length > 0);
  const sentMessage = computed(() => (sent.value ? t('invites.emailed', { email: sent.value.email }) : ''));
  const confirmMessage = computed(() => (confirming.value ? t('invites.revoke_confirm', { email: confirming.value.email }) : ''));

  async function load() {
    await invites.load(orgId);
  }

  function open() {
    email.value = '';
    role.value = 'member';
    error.value = null;
    sent.value = null;
    validation.clear();
    modalOpen.value = true;
  }

  function close() {
    modalOpen.value = false;
    error.value = null;
    validation.clear();
  }

  async function run(fallbackKey: string, action: () => Promise<void>, shown: (e: unknown) => ShownError | null = e => errorMessage(e, fallbackKey)) {
    if (busy.value) {
      return;
    }
    busy.value = true;
    error.value = null;
    notice.value = '';
    try {
      await action();
    } catch (e) {
      error.value = shown(e);
    } finally {
      busy.value = false;
    }
  }

  async function submit() {
    if (busy.value) {
      return;
    }
    const req = { orgId, email: email.value.trim(), role: role.value };
    if (!validation.check(create(CreateInviteRequestSchema, req))) {
      error.value = validation.formError.value;
      return;
    }
    await run('error.create_invite', async () => {
      const res = await client.createInvite(req);
      if (res.invite) {
        invites.upsert(orgId, toInvite(res.invite));
        sent.value = { id: res.invite.id, email: res.invite.email, link: res.link };
      }
    }, e => validation.fromServer(e, 'error.create_invite'));
  }

  async function resend(id: string) {
    await run('error.resend_invite', async () => {
      const res = await client.resendInvite({ orgId, inviteId: id });
      if (res.invite) {
        invites.upsert(orgId, toInvite(res.invite));
        rowLink.value = { id, email: res.invite.email, link: res.link };
      }
    });
  }

  async function confirmDraft(id: string) {
    await run('error.confirm_invite', async () => {
      try {
        const res = await client.confirmDraftInvite({ orgId, inviteId: id });
        if (res.invite) {
          invites.upsert(orgId, toInvite(res.invite));
          notice.value = t('invites.sent_to', { email: res.invite.email });
        }
      } catch (e) {
        if (errorCode(e) === 'INV003') {
          invites.drop(orgId, id);
        }
        throw e;
      }
    });
  }

  async function discardDraft(id: string) {
    await run('error.discard_invite', async () => {
      await client.discardDraftInvite({ orgId, inviteId: id });
      invites.drop(orgId, id);
    });
  }

  function askRevoke(id: string) {
    const invite = invites.pending.find(i => i.id === id);
    confirming.value = invite ? { id, email: invite.email } : null;
  }

  function cancel() {
    confirming.value = null;
  }

  async function confirm() {
    const c = confirming.value;
    if (!c) {
      return;
    }
    confirming.value = null;
    await run('error.revoke_invite', async () => {
      await client.revokeInvite({ orgId, inviteId: c.id });
      invites.drop(orgId, c.id);
    });
  }

  return {
    pending: computed(() => invites.pending),
    drafts: computed(() => invites.drafts),
    loading: computed(() => invites.loading),
    loadError: computed(() => invites.error),
    roles,
    canInvite,
    modalOpen,
    email,
    role,
    busy,
    error,
    fieldErrors: validation.errors,
    sent,
    sentMessage,
    rowLink,
    notice,
    confirming,
    confirmMessage,
    load,
    open,
    close,
    submit,
    resend,
    confirmDraft,
    discardDraft,
    askRevoke,
    cancel,
    confirm,
  };
}
