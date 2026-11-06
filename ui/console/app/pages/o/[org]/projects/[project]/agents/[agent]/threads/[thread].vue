<script setup lang="ts">
import AppButton from '#base/components/ui/AppButton.vue';
import ConfirmModal from '#base/components/ui/ConfirmModal.vue';
import EmptyState from '#base/components/ui/EmptyState.vue';
import KeyValue from '#base/components/ui/KeyValue.vue';
import LoadError from '#base/components/ui/LoadError.vue';
import StatusLabel from '#base/components/ui/StatusLabel.vue';
import { useConfirm } from '#base/composables/useConfirm';
import { useDates } from '#base/composables/useDates';
import { useAgentPage } from '~/components/agent/useAgentPage';
import { shownName } from '~/components/enduser/forget';
import { runStatusTone } from '~/components/thread/conversation';
import ChatTranscript from '~/components/try/ChatTranscript.vue';
import { useCan } from '~/composables/useCan';
import { useUnits } from '~/composables/useUnits';
import { useAuthStore } from '~/stores/auth';
import { useThreadsStore } from '~/stores/threads';

definePageMeta({ bleed: true });

const { t } = useI18n();
const route = useRoute();
const { moment } = useDates();
const { cost, costTitle, count } = useUnits();
const canEdit = useCan('agent:write');
const { agent, base, usesSample } = await useAgentPage();
const store = useThreadsStore();
const auth = useAuthStore();
await store.ensureLoaded(agent.value.id, usesSample.value);
await store.open(route.params.thread as string);
onBeforeUnmount(() => store.stopPolling());

const thread = computed(() => store.byId(route.params.thread as string));
if (!thread.value && !store.error) {
  throw createError({ statusCode: 404, statusMessage: t('error.thread_not_found'), fatal: true });
}

const project = `/o/${route.params.org}/projects/${route.params.project}`;
const runningRun = computed(() => {
  const last = [...(thread.value?.items ?? [])].reverse().find(i => i.kind === 'run');
  return last?.kind === 'run' && last.run.status === 'running' ? last.run : null;
});
const openApproval = computed(() => thread.value?.items.find(i => i.kind === 'approval' && i.approval.status === 'pending'));
const title = computed(() => {
  const th = thread.value;
  if (!th) {
    return '';
  }
  return th.kind === 'api' ? t('threads.api_user', { id: th.title }) : th.kind === 'try' ? t('try.title') : th.title;
});
const facts = computed(() => !thread.value
  ? []
  : [
      { label: t('threads.col_channel'), value: t(`threads.kind_${thread.value!.kind}`) },
      { label: t('thread.id'), value: thread.value!.id, mono: true, copy: true },
      { label: t('thread.people'), value: '' },
      { label: t('thread.created'), value: moment(thread.value!.createdAt) },
      { label: t('thread.usage'), value: t('thread.usage_value', { tokens: count(thread.value!.tokens), cost: cost(thread.value!.costCents) }) },
      { label: t('thread.open_approval'), value: openApproval.value?.kind === 'approval' ? openApproval.value.approval.title : t('thread.none') },
    ]);

const cancelling = useConfirm<string>(runId => store.cancelRun(thread.value!.id, runId, auth.me?.name ?? ''), 'error.cancel_run');
watch(runningRun, (run) => {
  if (!run) {
    cancelling.cancel();
  }
});

useHead({ title: () => `${title.value} · ${t('threads.title')} · OAgent` });
</script>

<template>
  <LoadError v-if="store.error" class="m-4 lg:m-6" :error="store.error" :busy="store.loading" :retry-label="t('common.retry')" @retry="store.load(agent.id, usesSample)" />
  <EmptyState v-else-if="!thread" icon="lucide:messages-square" :title="t('error.thread_not_found')" class="px-4 lg:px-6">
    <template #actions>
      <AppButton variant="outline" size="sm" :to="`${base}/threads`">
        {{ t('threads.title') }}
      </AppButton>
    </template>
  </EmptyState>
  <div v-else class="flex min-w-0 flex-col">
    <div class="flex flex-wrap items-center justify-between gap-3 border-b border-base-300 px-4 py-3 lg:px-6">
      <div class="min-w-0">
        <div class="flex flex-wrap items-center gap-2">
          <h1 class="text-xl font-semibold break-words">
            {{ title }}
          </h1>
          <span class="badge badge-sm badge-ghost">{{ t(`threads.kind_${thread.kind}`) }}</span>
          <span v-if="usesSample" class="badge badge-info badge-soft text-base-content badge-sm">{{ t('preview.badge') }}</span>
        </div>
        <p class="flex flex-wrap items-center gap-x-2 text-ui text-base-content/70">
          <span :title="costTitle(thread.costCents)">{{ t('thread.summary', { runs: thread.runs, cost: cost(thread.costCents) }, thread.runs) }}</span>
          <StatusLabel v-if="thread.status" :tone="runStatusTone[thread.status]" :label="runningRun && store.cancelling(runningRun.id) ? t('thread.cancelling') : t(`runstatus.${thread.status}`)" />
        </p>
      </div>
      <AppButton v-if="runningRun && canEdit && !store.cancelling(runningRun.id)" variant="error" size="sm" soft @click="cancelling.ask(runningRun.id)">
        <Icon name="lucide:octagon-x" size="16" aria-hidden="true" />
        {{ t('thread.cancel_run') }}
      </AppButton>
    </div>

    <div class="grid min-w-0 grid-cols-[minmax(0,1fr)] lg:grid-cols-[minmax(0,1fr)_minmax(18rem,22rem)]">
      <section class="min-w-0 p-4 lg:p-6" :aria-label="t('thread.transcript')">
        <ChatTranscript viewer="agent" :items="thread.items" :agent-name="agent.name" :you-name="t('try.you')" :run-base="`${base}/runs`" read-only />
      </section>
      <aside class="min-w-0 border-t border-base-300 bg-base-200 lg:border-s lg:border-t-0" :aria-label="t('thread.facts')">
        <KeyValue :items="facts" :copy-label="t('common.copy_label', { label: '{label}' })">
          <template #value-4>
            <span :title="costTitle(thread.costCents)">{{ facts[4]!.value }}</span>
          </template>
          <template #value-2>
            <span class="flex flex-wrap gap-x-2">
              <template v-for="(p, i) in thread.people" :key="`${i}:${p.name}`">
                <NuxtLink v-if="p.endUserId && thread.kind !== 'try'" :to="`${project}/end-users/${p.endUserId}`" class="link link-hover">{{ p.name }}</NuxtLink>
                <span v-else>{{ shownName(p.name, p.deleted, t) }}</span>
              </template>
            </span>
          </template>
        </KeyValue>
      </aside>
    </div>

    <ConfirmModal
      :handle="cancelling"
      :title="t('thread.cancel_title')"
      :body="t('thread.cancel_body')"
      :confirm-label="t('thread.cancel_run')"
      :cancel-label="t('common.cancel')"
    />
  </div>
</template>
