<script setup lang="ts">
import EmptyState from '#base/components/ui/EmptyState.vue';
import LoadError from '#base/components/ui/LoadError.vue';
import LoadingStatus from '#base/components/ui/LoadingStatus.vue';
import StatusLabel from '#base/components/ui/StatusLabel.vue';
import { useDates } from '#base/composables/useDates';
import { useAgentPage } from '~/components/agent/useAgentPage';
import { runDetailText, runFix, runStatusTone } from '~/components/thread/conversation';
import ChatTranscript from '~/components/try/ChatTranscript.vue';
import { useUnits } from '~/composables/useUnits';
import { useThreadsStore } from '~/stores/threads';

definePageMeta({ bleed: true });

const { t, te } = useI18n();
const route = useRoute();
const { moment } = useDates();
const { cost, costTitle, seconds, count } = useUnits();
const { agent, base } = await useAgentPage();
const store = useThreadsStore();
const runId = computed(() => route.params.run as string);
const view = computed(() => (store.runView?.run.id === runId.value ? store.runView : null));
const run = computed(() => view.value?.run);
const fix = computed(() => runFix(run.value?.detail, base.value));
const threadLink = computed(() => (run.value ? `${base.value}/threads/${run.value.threadId}` : ''));

watch(runId, id => void store.loadRun(id), { immediate: true });

useHead({ title: () => `${t('run.title', { id: runId.value })} · OAgent` });
</script>

<template>
  <div v-if="store.runError" class="m-4 lg:m-6">
    <h1 class="sr-only">
      {{ t('run.title', { id: runId }) }}
    </h1>
    <LoadError :error="store.runError" :busy="store.runLoading" :retry-label="t('common.retry')" @retry="store.loadRun(runId)" />
  </div>
  <LoadingStatus v-else-if="store.runLoading && !run" class="m-4 lg:m-6" :label="t('common.loading')" />
  <EmptyState v-else-if="!run || !view" icon="lucide:activity" :title="t('error.run_not_found')" :heading-level="1" class="px-4 lg:px-6">
    <template #actions>
      <NuxtLink :to="`${base}/runs`" class="link text-ui">
        {{ t('run.all') }}
      </NuxtLink>
    </template>
  </EmptyState>
  <div v-else class="flex min-w-0 flex-col">
    <header class="flex flex-wrap items-center justify-between gap-x-6 gap-y-2 border-b border-base-300 px-4 py-3 lg:px-6">
      <div class="min-w-0">
        <h1 class="text-xl font-semibold">
          {{ t('run.heading') }} <span class="font-mono text-base font-medium break-all">{{ run.id }}</span>
        </h1>
        <p class="text-ui text-base-content/70">
          {{ run.draft ? t('run.started_draft', { at: moment(run.startedAt) }) : t('run.started_live', { version: view.agentVersion, at: moment(run.startedAt) }) }}<template v-if="view.attempt > 1">
            · {{ t('run.attempt', { n: view.attempt }) }}
          </template>
          ·
          <NuxtLink :to="threadLink" class="link">
            {{ t('run.open_thread') }}
          </NuxtLink>
        </p>
      </div>
      <dl class="flex flex-wrap items-baseline gap-x-5 gap-y-1 text-ui">
        <div class="flex items-baseline gap-1.5">
          <dt class="text-base-content/70">
            {{ t('run.status') }}
          </dt>
          <dd>
            <span role="status"><StatusLabel :tone="runStatusTone[run.status]" :label="t(`runstatus.${run.status}`)" :detail="runDetailText(run.detail, t, te)" /></span>
          </dd>
        </div>
        <div class="flex items-baseline gap-1.5">
          <dt class="text-base-content/70">
            {{ t('run.duration') }}
          </dt>
          <dd class="font-medium tabular-nums">
            {{ seconds(run.seconds) }}
          </dd>
        </div>
        <div class="flex items-baseline gap-1.5">
          <dt class="text-base-content/70">
            {{ t('run.tokens') }}
          </dt>
          <dd class="font-medium tabular-nums">
            {{ run.estimated ? t('thread.tokens_estimated', { n: count(run.tokens) }) : count(run.tokens) }}
          </dd>
        </div>
        <div v-if="run.helperTokens" class="flex items-baseline gap-1.5">
          <dt class="text-base-content/70">
            {{ t('run.helper_tokens') }}
          </dt>
          <dd class="font-medium tabular-nums">
            {{ count(run.helperTokens) }}
          </dd>
        </div>
        <div v-if="run.embeddingTokens" class="flex items-baseline gap-1.5">
          <dt class="text-base-content/70">
            {{ t('run.embedding_tokens') }}
          </dt>
          <dd class="font-medium tabular-nums">
            {{ count(run.embeddingTokens) }}
          </dd>
        </div>
        <div v-if="run.transcriptionSeconds" class="flex items-baseline gap-1.5">
          <dt class="text-base-content/70">
            {{ t('run.transcription') }}
          </dt>
          <dd class="font-medium tabular-nums">
            {{ seconds(run.transcriptionSeconds) }}
          </dd>
        </div>
        <div v-if="run.retrieveCalls" class="flex items-baseline gap-1.5">
          <dt class="text-base-content/70">
            {{ t('run.knowledge_search') }}
          </dt>
          <dd class="font-medium tabular-nums">
            {{ t('run.knowledge_calls', { n: run.retrieveCalls, ms: count(run.retrieveMs ?? 0) }, run.retrieveCalls) }}
            <span v-if="run.retrievePassages">{{ t('run.knowledge_passages', { n: count(run.retrievePassages) }, run.retrievePassages) }}</span>
          </dd>
        </div>
        <div class="flex items-baseline gap-1.5">
          <dt class="text-base-content/70">
            {{ t('run.cost') }}
          </dt>
          <dd class="font-medium tabular-nums">
            <span v-if="run.unpriced">{{ t('thread.unpriced') }}</span><span v-else :title="costTitle(run.costCents)">{{ cost(run.costCents) }}</span>
          </dd>
        </div>
      </dl>
      <p v-if="fix" class="w-full text-ui">
        <NuxtLink :to="fix.to" class="link">
          {{ t(fix.labelKey) }}
        </NuxtLink>
      </p>
    </header>

    <section class="min-w-0 px-4 py-3 lg:px-6" :aria-labelledby="`run-${run.id}-messages`">
      <h2 :id="`run-${run.id}-messages`" class="pb-2 text-ui font-semibold">
        {{ t('run.messages') }}
      </h2>
      <p v-if="!view.items.length" class="text-ui text-base-content/70">
        {{ t('run.no_messages') }}
      </p>
      <ChatTranscript v-else viewer="agent" :items="view.items" :agent-name="agent.name" :you-name="t('try.you')" read-only />
    </section>
  </div>
</template>
