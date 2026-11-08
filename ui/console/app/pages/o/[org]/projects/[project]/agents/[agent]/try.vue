<script setup lang="ts">
import AppButton from '#base/components/ui/AppButton.vue';
import ErrorText from '#base/components/ui/ErrorText.vue';
import LoadError from '#base/components/ui/LoadError.vue';
import StatusLabel from '#base/components/ui/StatusLabel.vue';
import { useErrorMessage } from '#base/composables/useErrorMessage';
import { publishBlockers } from '~/components/agent/configure/runtime';
import { useAgentDraft } from '~/components/agent/configure/useAgentDraft';
import { useAgentPage } from '~/components/agent/useAgentPage';
import ChatTranscript from '~/components/try/ChatTranscript.vue';
import { phaseLabel } from '~/components/try/phase';
import TryRunPane from '~/components/try/TryRunPane.vue';
import { tryApi, useTryChat } from '~/components/try/useTryChat';
import { useUnits } from '~/composables/useUnits';

definePageMeta({ bleed: true });

const { t } = useI18n();
const { count, cost } = useUnits();
const { agent, providers, servers, base } = await useAgentPage();
const draft = useAgentDraft(agent.value.id, { provider: id => providers.items.find(p => p.id === id)?.name ?? '—', server: id => servers.items.find(s => s.id === id)?.name ?? id });
await draft.load();

const uid = useId();
const tab = ref<'draft' | 'live'>('draft');
const text = ref('');
const current = computed(() => draft.state.value?.publishedVersion ?? 0);
const blockers = computed(() => publishBlockers(draft.config.value, { providers: providers.items, servers: servers.items }));

const { $threadClient } = useNuxtApp();
const errorMessage = useErrorMessage();
const memoryKey = `oagent_try_thread:${agent.value.id}`;
const memory = {
  get: () => {
    try {
      return localStorage.getItem(memoryKey);
    } catch {
      return null;
    }
  },
  set: (id: string) => {
    try {
      localStorage.setItem(memoryKey, id);
    } catch {}
  },
};
const chat = useTryChat(tryApi(useRuntimeConfig().app.baseURL, agent.value.id, async runId => (await $threadClient.getRun({ runId })).run), undefined, memory);
onMounted(() => void chat.restore());
const { items, phase, tokens, costCents, busy, canStop, toolCalls } = chat;
const status = computed(() => phaseLabel(phase.value));
const shownError = computed(() => (chat.error.value ? errorMessage(chat.error.value, 'try.run_refused') : null));

function send() {
  const message = text.value;
  text.value = '';
  void chat.send(message);
}

function onKey(event: KeyboardEvent) {
  if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) {
    event.preventDefault();
    send();
  }
}

useHead({ title: () => `${agent.value?.name ?? ''} · ${t('try.title')} · OAgent` });
</script>

<template>
  <div class="flex min-h-full min-w-0 flex-col">
    <div class="flex flex-wrap items-center justify-between gap-3 border-b border-base-300 px-4 py-3 lg:px-6">
      <div class="flex min-w-0 flex-col gap-1">
        <div class="flex flex-wrap items-center gap-2">
          <h1 class="text-xl font-semibold">
            {{ t('try.title') }}
          </h1>
        </div>
        <p class="text-ui text-base-content/70">
          {{ t('try.subtitle', { name: agent.name }) }}
        </p>
      </div>
      <div class="flex flex-wrap items-center gap-3">
        <div class="flex flex-col items-end gap-1">
          <div class="tabs tabs-box tabs-sm" role="radiogroup" :aria-label="t('try.version')">
            <label class="tab" :class="{ 'tab-active': tab === 'draft' }">
              <input v-model="tab" type="radio" :name="`${uid}-tab`" value="draft">
              {{ t('try.draft') }}
            </label>
            <label class="tab tab-disabled">
              <input v-model="tab" type="radio" :name="`${uid}-tab`" value="live" disabled :aria-describedby="`${uid}-live`">
              {{ current ? t('try.live_legacy', { n: current }) : t('try.live_none') }}
            </label>
          </div>
          <p :id="`${uid}-live`" class="text-xs text-base-content/70">
            {{ current ? t('try.live_legacy_hint', { n: current }) : t('try.live_none_hint') }}
          </p>
        </div>
        <AppButton variant="plain" size="sm" :disabled="!items.length" @click="chat.newChat()">
          <Icon name="lucide:message-square-plus" size="16" aria-hidden="true" />
          {{ t('try.new_chat') }}
        </AppButton>
      </div>
    </div>

    <div class="px-4 lg:px-6">
      <LoadError :error="draft.loadError.value" :busy="false" :retry-label="t('common.retry')" @retry="draft.load()" />
    </div>

    <div v-if="blockers.length" class="p-4 lg:p-6">
      <div role="alert" class="alert alert-warning alert-soft items-start text-base-content text-ui">
        <Icon name="lucide:triangle-alert" size="16" class="mt-0.5 shrink-0" aria-hidden="true" />
        <div class="flex flex-col gap-1">
          <p class="font-semibold">
            {{ t('try.cant_run') }}
          </p>
          <ul class="flex flex-col gap-0.5">
            <li v-for="b in blockers" :key="b.key + (b.params?.server ?? '')">
              {{ t(b.key, b.params ?? {}) }}
            </li>
          </ul>
          <NuxtLink :to="base" class="link">
            {{ t('try.open_configure') }}
          </NuxtLink>
        </div>
      </div>
    </div>

    <div v-else class="grid min-h-0 min-w-0 flex-1 grid-cols-[minmax(0,1fr)] xl:grid-cols-[minmax(0,1fr)_minmax(20rem,24rem)]">
      <div class="flex min-h-[calc(100dvh-10rem)] min-w-0 flex-col xl:min-h-0">
        <details class="collapse-arrow collapse rounded-none border-b border-base-300 bg-base-200 xl:hidden">
          <summary class="collapse-title flex flex-wrap items-center gap-x-3 gap-y-1 py-2 text-ui">
            <span class="font-semibold">{{ t('try.this_run') }}</span>
            <span role="status"><StatusLabel :tone="status.tone" :label="t(status.key)" :detail="`${count(tokens)} · ${cost(costCents)}`" /></span>
          </summary>
          <div class="collapse-content">
            <TryRunPane :tokens="count(tokens)" :cost="cost(costCents)" :model="draft.config.value.model || '—'" :tool-calls="toolCalls" />
          </div>
        </details>
        <div class="flex-1 p-4 lg:p-6">
          <div v-if="!items.length" class="flex flex-col items-start gap-3 py-8">
            <p class="text-ui text-base-content/70">
              {{ t('try.empty') }}
            </p>
          </div>
          <ChatTranscript
            v-else
            viewer="person" :items="items" :agent-name="agent.name" :you-name="t('try.you')" :busy="busy"
            :run-base="`${base}/runs`" @retry="chat.retry()" @reload="chat.reload()"
          />
        </div>

        <form class="sticky bottom-0 flex flex-col gap-2 border-t border-base-300 bg-base-100 p-4 lg:px-6" @submit.prevent="send()">
          <ErrorText :error="shownError" />
          <div class="flex items-end gap-2">
            <label class="min-w-0 flex-1">
              <span class="sr-only">{{ t('try.message') }}</span>
              <textarea
                v-model="text" rows="1" class="textarea textarea-sm field-sizing-content max-h-40 min-h-0 w-full text-ui"
                :placeholder="busy ? t('try.replying') : t('try.placeholder')" :disabled="busy" :aria-describedby="`${uid}-keys`"
                @keydown="onKey"
              />
            </label>
            <AppButton v-if="busy" variant="outline" size="sm" :disabled="!canStop" @click="chat.stop()">
              <Icon name="lucide:square" size="14" aria-hidden="true" />
              {{ phase === 'stopping' ? t('try.phase_stopping') : t('try.stop') }}
            </AppButton>
            <AppButton v-else type="submit" size="sm" :disabled="!text.trim()">
              <Icon name="lucide:send" size="16" aria-hidden="true" />
              {{ t('try.send') }}
            </AppButton>
          </div>
          <p :id="`${uid}-keys`" class="flex flex-wrap items-center gap-1 text-xs text-base-content/70">
            <kbd class="kbd kbd-xs">Enter</kbd> {{ t('try.keys_send') }}
            <kbd class="kbd kbd-xs">Shift</kbd>+<kbd class="kbd kbd-xs">Enter</kbd> {{ t('try.keys_newline') }}
          </p>
          <p v-if="items.length" class="flex items-center gap-1.5 text-xs text-base-content/70" data-testid="draft-note">
            <Icon name="lucide:flask-conical" size="12" class="shrink-0" aria-hidden="true" />
            {{ t('try.empty') }}
          </p>
        </form>
      </div>

      <aside class="min-w-0 border-s border-base-300 bg-base-200 p-4 max-xl:hidden" :aria-label="t('try.run_pane')">
        <div class="flex flex-col gap-4 xl:sticky xl:top-0">
          <div class="flex flex-col gap-1" role="status">
            <h2 class="text-ui font-semibold">
              {{ t('try.this_run') }}
            </h2>
            <StatusLabel :tone="status.tone" :label="t(status.key)" />
          </div>
          <TryRunPane :tokens="count(tokens)" :cost="cost(costCents)" :model="draft.config.value.model || '—'" :tool-calls="toolCalls" />
        </div>
      </aside>
    </div>
  </div>
</template>
