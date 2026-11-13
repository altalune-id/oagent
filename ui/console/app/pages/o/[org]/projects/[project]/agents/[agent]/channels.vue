<script setup lang="ts">
import type { ReplyKey, ReplyTexts } from '~/components/channels/replies';
import type { ReplyEditorLabels } from '~/components/channels/ReplyTextsEditor.vue';
import type { WhatsAppStatus } from '~/stores/channels';
import AppButton from '#base/components/ui/AppButton.vue';
import AppModal from '#base/components/ui/AppModal.vue';
import ConfirmModal from '#base/components/ui/ConfirmModal.vue';
import CopyButton from '#base/components/ui/CopyButton.vue';
import FieldError from '#base/components/ui/FieldError.vue';
import KeyValue from '#base/components/ui/KeyValue.vue';
import LoadError from '#base/components/ui/LoadError.vue';
import PageHeader from '#base/components/ui/PageHeader.vue';
import Section from '#base/components/ui/Section.vue';
import StatusLabel from '#base/components/ui/StatusLabel.vue';
import { useToast } from '#base/components/ui/toast';
import { useConfirm } from '#base/composables/useConfirm';
import { useDates } from '#base/composables/useDates';
import { useErrorMessage } from '#base/composables/useErrorMessage';
import { chosenModel } from '~/components/agent/configure/runtime';
import { useAgentPage } from '~/components/agent/useAgentPage';
import OpenwaConnectModal from '~/components/channels/OpenwaConnectModal.vue';
import { replyErrors, replyKeys, seconds, timingErrors, toMs } from '~/components/channels/replies';
import ReplyTextsEditor from '~/components/channels/ReplyTextsEditor.vue';
import { useOpenwaConnect } from '~/components/channels/useOpenwaConnect';
import { useReplaceSecret } from '~/components/channels/useReplaceSecret';
import CodeText from '~/components/layout/CodeText.vue';
import ProjectReadOnlyNote from '~/components/project/ProjectReadOnlyNote.vue';
import { useCan } from '~/composables/useCan';
import { describedBy } from '~/lib/names/names';
import { useChannelsStore } from '~/stores/channels';

const { t } = useI18n();
const uid = useId();
const route = useRoute();
const { when } = useDates();
const toast = useToast();
const errorMessage = useErrorMessage();
const canEdit = useCan('channel:write');
const { project, agent, providers, base } = await useAgentPage();
const channels = useChannelsStore();
await channels.ensureLoaded(project.value!.id, agent.value.id);
const form = useOpenwaConnect(project.value!.id, agent.value.id);

const connection = computed(() => channels.connection(agent.value.id));
const replace = useReplaceSecret(() => connection.value?.id);
const published = computed(() => (agent.value.publishedVersion ?? 0) > 0);
const unpublished = computed(() => !published.value || !!agent.value.unpublishedChanges);
const vision = computed(() => !!chosenModel({ providerId: agent.value.providerId ?? '', model: agent.value.model ?? '' }, providers.items).model?.vision);
const { apiUrl } = useRuntimeConfig().public;
const endpoint = computed(() => new URL(`${(apiUrl || '/api').replace(/\/$/, '')}/v1/agents/${agent.value.id}/runs`, window.location.origin).href);
const curl = computed(() => [
  `curl -N ${endpoint.value} \\`,
  '  -H "Authorization: Bearer $OAGENT_KEY" \\',
  '  -H "Content-Type: application/json" \\',
  '  -d \'{"threadId":"rumah-001","user":"cust_0001","messages":[{"role":"user","content":"beli kopi 40rb pakai cash"}]}\'',
]);

const statusInfo: Record<WhatsAppStatus, { tone: 'success' | 'warning' | 'error', key: string }> = {
  connected: { tone: 'success', key: 'openwa.status_connected' },
  waiting: { tone: 'warning', key: 'openwa.status_waiting' },
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
    { label: t('openwa.workspace'), value: `${c.openwaOrg} / ${c.openwaProject}`, mono: true },
    { label: t('openwa.webhook_url'), value: c.webhookUrl, mono: true, copy: true },
    { label: t('openwa.last_result'), value: c.lastResultAt ? when(c.lastResultAt) : t('openwa.no_message_yet') },
    { label: t('openwa.secret'), value: '' },
    { label: t('openwa.device_key'), value: '' },
  ];
});

const replies = ref<ReplyTexts>({ ...channels.replies(agent.value.id) });
const replyErrs = ref<Partial<Record<ReplyKey, string>>>({});
const savingReplies = ref(false);

const quietText = ref('');
const quietMaxText = ref('');
const quietLimit = computed(() => seconds(channels.settings.timing.maxQuietMs));
const timingErrs = ref<Partial<Record<'quiet' | 'quietMax', string>>>({});
const savingTiming = ref(false);

watch(() => connection.value?.id, () => {
  replies.value = { ...channels.replies(agent.value.id) };
  replyErrs.value = {};
  quietText.value = String(seconds(connection.value?.quietMs ?? channels.settings.timing.quietMs));
  quietMaxText.value = String(seconds(connection.value?.quietMaxMs ?? channels.settings.timing.quietMaxMs));
  timingErrs.value = {};
}, { immediate: true });

const replyLabels = computed<ReplyEditorLabels>(() => ({
  legend: t('channels.replies'),
  hint: t('channels.replies_hint'),
  fields: Object.fromEntries(replyKeys.map(k => [k, { label: t(`channels.reply_${k}`), hint: t(`channels.reply_${k}_hint`) }])) as ReplyEditorLabels['fields'],
  reset: t('channels.reset_reply'),
  save: t('channels.save_replies'),
}));

async function saveReplies() {
  replyErrs.value = Object.fromEntries(Object.entries(replyErrors(replies.value)).map(([k, key]) => [k, t(key)]));
  if (Object.keys(replyErrs.value).length || !connection.value || savingReplies.value) {
    return;
  }
  savingReplies.value = true;
  try {
    await channels.update(connection.value.id, { replies: replies.value });
    replies.value = { ...channels.replies(agent.value.id) };
    toast.push('success', t('channels.replies_saved'));
  } catch (e) {
    toast.push('error', errorMessage(e, 'error.save_channel').message);
  } finally {
    savingReplies.value = false;
  }
}

async function saveTiming() {
  const quietMs = toMs(quietText.value);
  const quietMaxMs = toMs(quietMaxText.value);
  timingErrs.value = Object.fromEntries(Object.entries(timingErrors(quietMs, quietMaxMs, channels.settings.timing.maxQuietMs)).map(([k, key]) => [k, t(key, { max: quietLimit.value })]));
  if (Object.keys(timingErrs.value).length || !connection.value || savingTiming.value) {
    return;
  }
  savingTiming.value = true;
  try {
    await channels.update(connection.value.id, { quietMs: quietMs!, quietMaxMs: quietMaxMs! });
    toast.push('success', t('channels.timing_saved'));
  } catch (e) {
    toast.push('error', errorMessage(e, 'error.save_channel').message);
  } finally {
    savingTiming.value = false;
  }
}

const disconnecting = useConfirm<string>(async (id) => {
  await channels.remove(id);
  toast.push('success', t('openwa.disconnected'));
}, 'error.disconnect_whatsapp');

function changeDevice() {
  form.show();
  void form.next();
}

const switching = ref(false);

async function setEnabled(id: string, enabled: boolean) {
  switching.value = true;
  try {
    await channels.update(id, { enabled });
    toast.push('success', enabled ? t('openwa.turned_on') : t('openwa.turned_off'));
  } catch (e) {
    toast.push('error', errorMessage(e, 'error.save_channel').message);
  } finally {
    switching.value = false;
  }
}

const turningOff = useConfirm<string>(id => setEnabled(id, false), 'error.save_channel');

function onSwitch(e: Event) {
  const box = e.target as HTMLInputElement;
  const c = connection.value;
  if (!c) {
    return;
  }
  if (box.checked) {
    void setEnabled(c.id, true);
    return;
  }
  box.checked = true;
  turningOff.ask(c.id);
}

useHead({ title: () => `${agent.value?.name ?? ''} · ${t('channels.title')} · OAgent` });
</script>

<template>
  <div class="flex max-w-3xl flex-col gap-4">
    <PageHeader :title="t('channels.title')" :description="t('channels.subtitle')" />
    <ProjectReadOnlyNote v-if="!canEdit" :text="t('channels.read_only')" />

    <LoadError v-if="channels.error" :error="channels.error" :busy="channels.loading" :retry-label="t('common.retry')" @retry="channels.load(project!.id, agent.id)" />
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

        <template v-else-if="!connection.secretSet || connection.status === 'waiting'">
          <StatusLabel v-if="!connection.secretSet" tone="warning" :label="t('openwa.status_needs_secret')" :detail="t('openwa.needs_secret_detail')" />
          <StatusLabel v-else tone="warning" :label="t('openwa.status_waiting')" :detail="t('openwa.waiting_detail')" />
          <div v-if="canEdit" class="flex flex-wrap gap-2">
            <AppButton size="sm" @click="form.show(true)">
              {{ t('openwa.continue_setup') }}
            </AppButton>
            <AppButton variant="plain" size="sm" @click="disconnecting.ask(connection.id)">
              {{ t('openwa.disconnect') }}
            </AppButton>
          </div>
        </template>

        <template v-else>
          <div class="-mx-4 border-y border-base-300">
            <KeyValue :items="facts" :copy-label="t('common.copy_label', { label: '{label}' })">
              <template #value-0>
                <StatusLabel :tone="statusInfo[connection.status].tone" :label="t(statusInfo[connection.status].key)" />
                <span v-if="connection.enabled === false" class="font-medium" data-testid="status-off">· {{ t('openwa.switch_off') }}</span>
              </template>
              <template #value-5>
                <span>{{ t('openwa.set') }}</span>
                <span v-if="connection.secondarySet" class="text-xs text-base-content/70">{{ t('openwa.secondary_set') }}</span>
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
          <div v-if="connection.status === 'key_refused' || connection.status === 'device_mismatch'" role="alert" class="alert alert-error alert-soft alert-vertical text-base-content text-ui sm:alert-horizontal">
            <Icon name="lucide:circle-alert" size="16" class="shrink-0" aria-hidden="true" />
            <span><CodeText :text="t(`openwa.problem_${connection.status}`, { device: connection.deviceId })" /></span>
            <AppButton v-if="canEdit && connection.status === 'key_refused'" variant="plain" size="xs" @click="replace.show('key')">
              {{ t('openwa.replace_key') }}
            </AppButton>
            <AppButton v-if="canEdit && connection.status === 'device_mismatch'" variant="plain" size="xs" @click="changeDevice()">
              {{ t('openwa.change_device') }}
            </AppButton>
          </div>
          <div v-if="connection.lastBadSignatureAt" role="note" class="alert alert-warning alert-soft alert-vertical text-base-content text-ui sm:alert-horizontal">
            <Icon name="lucide:shield-alert" size="16" class="shrink-0" aria-hidden="true" />
            <span>{{ t('openwa.bad_signature_seen', { when: when(connection.lastBadSignatureAt) }) }}</span>
            <AppButton v-if="canEdit" variant="plain" size="xs" @click="replace.show('secret')">
              {{ t('openwa.replace_secret') }}
            </AppButton>
          </div>
        </template>

        <div v-if="connection" class="flex flex-col gap-1 rounded-box border border-base-300 px-3 py-2" data-testid="channel-switch">
          <label v-if="canEdit" class="flex items-center gap-2 text-ui font-medium">
            <input type="checkbox" role="switch" class="toggle toggle-sm" :checked="connection.enabled" :disabled="switching" :aria-describedby="`${uid}-switch-hint`" @change="onSwitch">
            {{ t('openwa.switch_label') }}
          </label>
          <p v-else class="text-ui font-medium">
            {{ t('openwa.switch_label') }}: {{ connection.enabled ? t('openwa.switch_on') : t('openwa.switch_off') }}
          </p>
          <p :id="`${uid}-switch-hint`" class="text-xs text-base-content/70">
            {{ connection.enabled ? t('openwa.on_hint') : t('openwa.off_hint') }}
          </p>
        </div>
        <div v-if="canEdit && connection && connection.secretSet && connection.status !== 'waiting'">
          <AppButton variant="error" size="sm" soft @click="disconnecting.ask(connection.id)">
            {{ t('openwa.disconnect') }}
          </AppButton>
        </div>

        <p v-if="agent.model && !vision" role="note" class="text-xs text-base-content/70">
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
          <ReplyTextsEditor v-model="replies" :defaults="channels.settings.defaultReplies" :labels="replyLabels" :errors="replyErrs" :read-only="!canEdit || savingReplies" @save="saveReplies()" />
        </Section>

        <Section :title="t('channels.windows')" :description="t('channels.windows_hint')">
          <form novalidate class="flex flex-col gap-3" @submit.prevent="saveTiming()">
            <fieldset class="grid min-w-0 grid-cols-[minmax(0,1fr)] gap-3 sm:grid-cols-2" :disabled="!canEdit || savingTiming">
              <div class="flex flex-col gap-1">
                <label class="flex flex-col gap-1">
                  <span class="label text-ui font-semibold text-base-content">{{ t('channels.quiet_window') }}</span>
                  <span class="input input-sm w-full text-ui tabular-nums" :class="{ 'input-error': timingErrs.quiet }">
                    <input v-model="quietText" type="text" inputmode="decimal" class="grow" :aria-invalid="!!timingErrs.quiet" :aria-describedby="describedBy(`${uid}-quiet-hint`, !!timingErrs.quiet && `${uid}-quiet-err`)">
                    <span class="text-base-content/70" aria-hidden="true">s</span>
                  </span>
                </label>
                <p :id="`${uid}-quiet-hint`" class="text-xs text-base-content/70">
                  {{ quietLimit ? t('channels.quiet_window_hint', { max: quietLimit }) : t('channels.quiet_window_hint_open') }}
                </p>
                <FieldError :id="`${uid}-quiet-err`" :message="timingErrs.quiet" />
              </div>
              <div class="flex flex-col gap-1">
                <label class="flex flex-col gap-1">
                  <span class="label text-ui font-semibold text-base-content">{{ t('channels.quiet_max') }}</span>
                  <span class="input input-sm w-full text-ui tabular-nums" :class="{ 'input-error': timingErrs.quietMax }">
                    <input v-model="quietMaxText" type="text" inputmode="decimal" class="grow" :aria-invalid="!!timingErrs.quietMax" :aria-describedby="describedBy(`${uid}-max-hint`, !!timingErrs.quietMax && `${uid}-max-err`)">
                    <span class="text-base-content/70" aria-hidden="true">s</span>
                  </span>
                </label>
                <p :id="`${uid}-max-hint`" class="text-xs text-base-content/70">
                  {{ quietLimit ? t('channels.quiet_max_hint', { max: quietLimit }) : t('channels.quiet_max_hint_open') }}
                </p>
                <FieldError :id="`${uid}-max-err`" :message="timingErrs.quietMax" />
              </div>
            </fieldset>
            <p class="text-xs text-base-content/70">
              {{ t('channels.late_cutoff_value', { n: channels.settings.timing.maxMessageAgeMinutes }) }}
            </p>
            <div v-if="canEdit">
              <AppButton type="submit" size="sm" :busy="savingTiming">
                {{ t('channels.save_timing') }}
              </AppButton>
            </div>
          </form>
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
          <div class="mockup-code w-full text-xs" tabindex="0" role="region" :aria-label="t('apiaccess.example')">
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
          <div v-if="replace.kind.value === 'secret' && connection?.secretSet" class="flex flex-col gap-1">
            <label class="flex items-center gap-2 text-ui">
              <input v-model="replace.dropPrevious.value" type="checkbox" class="checkbox checkbox-sm rounded-sm" :aria-describedby="`${uid}-drop-hint`">
              <span>{{ t('openwa.drop_previous') }}</span>
            </label>
            <p :id="`${uid}-drop-hint`" class="text-xs text-base-content/70">
              {{ t('openwa.rotation_note') }}
            </p>
          </div>
        </form>
        <template #footer>
          <AppButton variant="plain" size="sm" @click="replace.hide()">
            {{ t('common.cancel') }}
          </AppButton>
          <AppButton size="sm" :busy="replace.saving.value" @click="replace.save()">
            {{ t('common.save') }}
          </AppButton>
        </template>
      </AppModal>

      <ConfirmModal
        :handle="turningOff"
        :title="t('openwa.turn_off_title')"
        :body="t('openwa.turn_off_body')"
        :confirm-label="t('openwa.turn_off')"
        :cancel-label="t('common.cancel')"
      />
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
