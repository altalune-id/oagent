<script setup lang="ts">
import type { ReplyKey, ReplyTexts } from '~/components/channels/replies';
import type { ReplyEditorLabels } from '~/components/channels/ReplyTextsEditor.vue';
import type { WhatsAppStatus } from '~/stores/channels';
import AppButton from '#base/components/ui/AppButton.vue';
import AppModal from '#base/components/ui/AppModal.vue';
import ConfirmModal from '#base/components/ui/ConfirmModal.vue';
import CopyButton from '#base/components/ui/CopyButton.vue';
import KeyValue from '#base/components/ui/KeyValue.vue';
import LoadError from '#base/components/ui/LoadError.vue';
import PageHeader from '#base/components/ui/PageHeader.vue';
import Section from '#base/components/ui/Section.vue';
import StatusLabel from '#base/components/ui/StatusLabel.vue';
import { useToast } from '#base/components/ui/toast';
import { useConfirm } from '#base/composables/useConfirm';
import { useDates } from '#base/composables/useDates';
import { chosenModel } from '~/components/agent/configure/runtime';
import { useAgentPage } from '~/components/agent/useAgentPage';
import OpenwaConnectModal from '~/components/channels/OpenwaConnectModal.vue';
import { replyErrors, replyKeys, seconds } from '~/components/channels/replies';
import ReplyTextsEditor from '~/components/channels/ReplyTextsEditor.vue';
import { useOpenwaConnect } from '~/components/channels/useOpenwaConnect';
import { useReplaceSecret } from '~/components/channels/useReplaceSecret';
import CodeText from '~/components/layout/CodeText.vue';
import PreviewNote from '~/components/layout/PreviewNote.vue';
import ProjectReadOnlyNote from '~/components/project/ProjectReadOnlyNote.vue';
import { useCan } from '~/composables/useCan';
import { describedBy } from '~/lib/names/names';
import { useChannelsStore } from '~/stores/channels';

const { t } = useI18n();
const uid = useId();
const route = useRoute();
const { when } = useDates();
const toast = useToast();
// TODO: gate on the ChannelService scope once it exists; agent:write stands in until then.
const canEdit = useCan('agent:write');
const { agent, providers, base } = await useAgentPage();
const channels = useChannelsStore();
await channels.ensureLoaded(agent.value.id);
const form = useOpenwaConnect(agent.value.id);

const connection = computed(() => channels.connection(agent.value.id, 'openwa'));
const replace = useReplaceSecret(agent.value.id, 'openwa');
const published = computed(() => (agent.value.publishedVersion ?? 0) > 0);
const unpublished = computed(() => !published.value || !!agent.value.unpublishedChanges);
const vision = computed(() => !!chosenModel({ providerId: agent.value.providerId ?? '', model: agent.value.model ?? '' }, providers.items).model?.vision);
const endpoint = computed(() => `https://oagent.altalune.id/api/v1/agents/${agent.value.id}/runs`);
const curl = computed(() => [
  `curl -N ${endpoint.value} \\`,
  '  -H "Authorization: Bearer $OAGENT_KEY" \\',
  '  -H "Content-Type: application/json" \\',
  '  -d \'{"threadId":"rumah-001","user":"cust_0001","messages":[{"role":"user","content":"beli kopi 40rb pakai cash"}]}\'',
]);

const statusInfo: Record<WhatsAppStatus, { tone: 'success' | 'warning' | 'error', key: string }> = {
  connected: { tone: 'success', key: 'openwa.status_connected' },
  waiting: { tone: 'warning', key: 'openwa.status_waiting' },
  bad_signature: { tone: 'error', key: 'openwa.status_bad_signature' },
  key_refused: { tone: 'error', key: 'openwa.status_key_refused' },
  device_mismatch: { tone: 'error', key: 'openwa.status_device_mismatch' },
};

const facts = computed(() => {
  const c = connection.value;
  if (!c) {
    return [];
  }
  return [
    { label: t('openwa.status'), value: '' },
    { label: t('openwa.device_id'), value: c.deviceId, mono: true },
    { label: t('openwa.webhook_url'), value: c.webhookUrl, mono: true, copy: true },
    { label: t('openwa.last_message'), value: c.lastMessageAt ? t('openwa.last_message_value', { when: when(c.lastMessageAt) }) : t('openwa.no_message_yet') },
    { label: t('openwa.rejected_24h'), value: String(c.rejected24h) },
    { label: t('openwa.secret'), value: '' },
    { label: t('openwa.device_key'), value: '' },
  ];
});

const replies = ref<ReplyTexts>({ ...channels.replies(agent.value.id) });
const replyErrs = ref<Partial<Record<ReplyKey, string>>>({});
watch(() => connection.value?.id, () => {
  replies.value = { ...channels.replies(agent.value.id) };
  replyErrs.value = {};
});

const replyLabels = computed<ReplyEditorLabels>(() => ({
  legend: t('channels.replies'),
  hint: t('channels.replies_hint'),
  fields: Object.fromEntries(replyKeys.map(k => [k, { label: t(`channels.reply_${k}`), hint: t(`channels.reply_${k}_hint`) }])) as ReplyEditorLabels['fields'],
  reset: t('channels.reset_reply'),
  save: t('channels.save_replies'),
}));

const timing = computed(() => {
  const w = channels.settings.timing;
  return [
    { label: t('channels.quiet_window'), value: t('channels.seconds', { n: seconds(w.quietMs) }) },
    { label: t('channels.quiet_max'), value: t('channels.seconds', { n: seconds(w.quietMaxMs) }) },
    { label: t('channels.late_cutoff'), value: t('channels.minutes', { n: w.maxMessageAgeMinutes }) },
  ];
});

function saveReplies() {
  replyErrs.value = Object.fromEntries(Object.entries(replyErrors(replies.value)).map(([k, key]) => [k, t(key)]));
  if (Object.keys(replyErrs.value).length) {
    return;
  }
  channels.saveReplies(agent.value.id, replies.value);
  replies.value = { ...channels.replies(agent.value.id) };
  toast.push('success', t('channels.replies_saved'));
}

const disconnecting = useConfirm<boolean>(async () => {
  channels.remove(agent.value.id, 'openwa');
  toast.push('success', t('openwa.disconnected'));
}, 'error.disconnect_whatsapp');

useHead({ title: () => `${agent.value?.name ?? ''} · ${t('channels.title')} · OAgent` });
</script>

<template>
  <div class="flex max-w-3xl flex-col gap-4">
    <PageHeader :title="t('channels.title')" :description="t('channels.subtitle')" :preview="t('preview.badge')">
      <template #status>
        <PreviewNote :text="t('preview.local_note')" />
      </template>
    </PageHeader>
    <ProjectReadOnlyNote v-if="!canEdit" :text="t('channels.read_only')" />

    <LoadError v-if="channels.error" :error="channels.error" :busy="channels.loading" :retry-label="t('common.retry')" @retry="channels.load(agent.id)" />
    <template v-else>
      <Section :title="t('openwa.title')" :description="t('openwa.subtitle')">
        <div v-if="connection && unpublished" role="alert" class="alert alert-warning alert-soft text-base-content text-ui">
          <Icon name="lucide:triangle-alert" size="16" class="shrink-0" aria-hidden="true" />
          <span>{{ t('openwa.publish_warning') }}</span>
        </div>
        <template v-if="!connection">
          <StatusLabel tone="neutral" :label="t('openwa.status_none')" />
          <p class="text-ui">
            {{ t('openwa.not_connected') }}
          </p>
          <p v-if="!published" class="text-xs text-base-content/70">
            {{ t('openwa.needs_publish') }}
          </p>
          <div v-if="canEdit">
            <AppButton size="sm" @click="form.show()">
              <Icon name="lucide:message-circle" size="16" aria-hidden="true" />
              {{ t('openwa.connect') }}
            </AppButton>
          </div>
        </template>

        <template v-else-if="connection.status === 'waiting'">
          <StatusLabel tone="warning" :label="t('openwa.status_waiting')" :detail="t('openwa.waiting_detail')" />
          <div v-if="canEdit" class="flex flex-wrap gap-2">
            <AppButton size="sm" @click="form.show(true)">
              {{ t('openwa.continue_setup') }}
            </AppButton>
            <AppButton variant="plain" size="sm" @click="disconnecting.ask(true)">
              {{ t('openwa.disconnect') }}
            </AppButton>
          </div>
        </template>

        <template v-else>
          <div class="-mx-4 border-y border-base-300">
            <KeyValue :items="facts" :copy-label="t('common.copy_label', { label: '{label}' })">
              <template #value-0>
                <StatusLabel :tone="statusInfo[connection.status].tone" :label="t(statusInfo[connection.status].key)" />
              </template>
              <template #value-5>
                <span>{{ t('openwa.set') }}</span>
                <AppButton v-if="canEdit" variant="plain" size="xs" @click="replace.show('secret')">
                  {{ t('openwa.replace') }}
                </AppButton>
              </template>
              <template #value-6>
                <span>{{ t('openwa.set') }}</span>
                <AppButton v-if="canEdit" variant="plain" size="xs" @click="replace.show('key')">
                  {{ t('openwa.replace') }}
                </AppButton>
              </template>
            </KeyValue>
          </div>
          <div v-if="connection.status !== 'connected'" role="alert" class="alert alert-error alert-soft alert-vertical text-base-content text-ui sm:alert-horizontal">
            <Icon name="lucide:circle-alert" size="16" class="shrink-0" aria-hidden="true" />
            <span><CodeText :text="t(`openwa.problem_${connection.status}`, { n: connection.rejectedLastHour })" /></span>
            <AppButton v-if="canEdit && connection.status !== 'device_mismatch'" variant="plain" size="xs" @click="replace.show(connection.status === 'bad_signature' ? 'secret' : 'key')">
              {{ connection.status === 'bad_signature' ? t('openwa.replace_secret') : t('openwa.replace_key') }}
            </AppButton>
          </div>
          <div v-if="canEdit">
            <AppButton variant="error" size="sm" soft @click="disconnecting.ask(true)">
              {{ t('openwa.disconnect') }}
            </AppButton>
          </div>
        </template>

        <p v-if="!vision" role="note" class="text-xs text-base-content/70">
          {{ t('openwa.no_vision', { model: agent.model || '—' }) }}
          <NuxtLink :to="`${base}#configure-model`" class="link">
            {{ t('try.pick_vision') }}
          </NuxtLink>
        </p>
        <p class="text-xs text-base-content/70">
          {{ t('openwa.who_may_talk') }}
        </p>
      </Section>

      <template v-if="connection">
        <Section :title="t('channels.replies')">
          <ReplyTextsEditor v-model="replies" :defaults="channels.settings.defaultReplies" :labels="replyLabels" :errors="replyErrs" :read-only="!canEdit" @save="saveReplies()" />
        </Section>

        <Section :title="t('channels.windows')" :description="t('channels.windows_hint')">
          <div class="-mx-4 border-y border-base-300">
            <KeyValue :items="timing" :copy-label="t('common.copy_label', { label: '{label}' })" />
          </div>
        </Section>
      </template>

      <Section :title="t('channels.provider_whatsapp_business')" :description="t('channels.business_subtitle')">
        <template #actions>
          <span class="badge badge-sm badge-outline">{{ t('widget.coming_later') }}</span>
        </template>
        <p class="text-ui text-base-content/70">
          {{ t('channels.business_body') }}
        </p>
      </Section>

      <Section :title="t('apiaccess.title')" :description="t('apiaccess.subtitle')">
        <div class="-mx-4 border-y border-base-300">
          <KeyValue :items="[{ label: t('apiaccess.endpoint'), value: `POST ${endpoint}`, mono: true }, { label: t('apiaccess.agent_id'), value: agent.id, mono: true, copy: true }]" :copy-label="t('common.copy_label', { label: '{label}' })">
            <template #value-0>
              <span class="flex min-w-0 flex-col">
                <span class="flex items-center gap-2"><span class="font-mono text-xs break-all">POST {{ endpoint }}</span><CopyButton :value="endpoint" :label="t('common.copy_label', { label: t('apiaccess.endpoint') })" /></span>
              </span>
            </template>
          </KeyValue>
        </div>
        <p class="text-ui">
          <CodeText :text="t('apiaccess.keys')" /> <NuxtLink :to="`/o/${route.params.org}/settings/api-keys`" class="link">
            {{ t('apiaccess.create_key') }}
          </NuxtLink>
        </p>
        <div class="flex flex-col gap-1">
          <div class="flex items-center justify-between">
            <p class="text-xs text-base-content/70">
              {{ t('apiaccess.example') }}
            </p>
            <CopyButton :value="curl.join('\n')" :label="t('common.copy')" />
          </div>
          <div class="mockup-code w-full text-xs">
            <pre v-for="(line, i) in curl" :key="i" :data-prefix="i === 0 ? '$' : ' '"><code>{{ line }}</code></pre>
          </div>
        </div>
        <p class="text-xs text-base-content/70">
          {{ t('apiaccess.stream_note') }}
        </p>
      </Section>

      <Section :title="t('widget.title')">
        <template #actions>
          <span class="badge badge-sm badge-outline">{{ t('widget.coming_later') }}</span>
        </template>
        <p class="text-ui text-base-content/70">
          {{ t('widget.body') }}
        </p>
      </Section>

      <OpenwaConnectModal :form="form" :unpublished="unpublished" />

      <AppModal :open="!!replace.kind.value" :title="replace.kind.value === 'secret' ? t('openwa.replace_secret') : t('openwa.replace_key')" @close="replace.hide()">
        <form novalidate class="flex flex-col gap-3" @submit.prevent="replace.save()">
          <label class="flex flex-col gap-1">
            <span class="label text-ui font-semibold text-base-content">{{ replace.kind.value === 'secret' ? t('openwa.new_secret') : t('openwa.new_key') }}</span>
            <input v-model="replace.value.value" type="password" autocomplete="new-password" class="input input-sm w-full font-mono text-xs" :class="{ 'input-error': replace.error.value }" :aria-invalid="!!replace.error.value" :aria-describedby="describedBy(`${uid}-replace-hint`, !!replace.error.value && `${uid}-replace-err`)">
          </label>
          <p :id="`${uid}-replace-hint`" class="-mt-2 text-xs text-base-content/70">
            {{ t('secret.write_only') }}
          </p>
          <p v-if="replace.error.value" :id="`${uid}-replace-err`" role="alert" class="flex items-center gap-1.5 text-xs text-base-content">
            <Icon name="lucide:circle-alert" size="14" class="shrink-0 text-error" aria-hidden="true" />
            {{ replace.error.value }}
          </p>
          <template v-if="replace.kind.value === 'secret'">
            <label class="flex items-start gap-2 text-ui">
              <input v-model="replace.keepOld.value" type="checkbox" class="checkbox checkbox-sm mt-0.5 rounded-sm">
              {{ t('openwa.keep_old') }}
            </label>
            <p class="-mt-2 ps-7 text-xs text-base-content/70">
              {{ t('openwa.keep_old_why') }}
            </p>
          </template>
        </form>
        <template #footer>
          <AppButton variant="plain" size="sm" @click="replace.hide()">
            {{ t('common.cancel') }}
          </AppButton>
          <AppButton size="sm" @click="replace.save()">
            {{ t('common.save') }}
          </AppButton>
        </template>
      </AppModal>

      <ConfirmModal
        :handle="disconnecting"
        :title="t('openwa.disconnect_title')"
        :body="t('openwa.disconnect_body')"
        :confirm-label="t('openwa.disconnect')"
        :cancel-label="t('common.cancel')"
      />
    </template>
  </div>
</template>
